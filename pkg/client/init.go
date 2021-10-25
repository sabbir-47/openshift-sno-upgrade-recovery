package client

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"text/template"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var LIVE_POLICY string = "policy-backup-live-image"
var RELEASE_POLICY string = "policy-backup-release-image"
var NAMESPACE = "open-cluster-management"
var RETRY_PERIOD_SECONDS = 10
var TIMEOUT_MINUTES = 15

type Client struct {
	KubeconfigPath        string
	Spoke                 string
	BinaryImage           string
	BackupPath            string
	KubernetesClient      dynamic.Interface
	CurrentLiveVersion    string
	CurrentReleaseVersion string
}

type BackupImageSpoke struct {
	SpokeName             string
	PolicyName            string
	ImageBinaryImageName  string
	ImageURL              string
	RecoveryPartitionPath string
	RandomId              string
}

type PlacementBinding struct {
	PlacementName string
	PolicyName    string
}

func RandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func New(KubeconfigPath string, Spoke string, BinaryImage string, BackupPath string) (Client, error) {
	rand.Seed(time.Now().UnixNano())
	c := Client{KubeconfigPath, Spoke, BinaryImage, BackupPath, nil, "", ""}

	var clientset dynamic.Interface

	if KubeconfigPath != "" {
		// generate config from file
		config, err := clientcmd.BuildConfigFromFlags("", KubeconfigPath)
		if err != nil {
			log.Error(err)
			return c, err
		}
		// now try to connect to cluster
		clientset, err = dynamic.NewForConfig(config)
		if err != nil {
			log.Error(err)
			return c, err
		}

	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			log.Error(err)
			return c, err
		}

		// now try to connect to cluster
		clientset, err = dynamic.NewForConfig(config)
		if err != nil {
			log.Error(err)
			return c, err
		}
	}

	c.KubernetesClient = clientset

	return c, nil
}

func (c Client) SpokeClusterExists() bool {
	// using client, get if spoke cluster with given name exists
	gvr := schema.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}

	foundSpokeCluster, err := c.KubernetesClient.Resource(gvr).Get(context.Background(), c.Spoke, v1.GetOptions{})

	if err != nil {
		log.Error(err)
		return false
	}

	// transform to typed
	if foundSpokeCluster != nil {
		status, _, _ := unstructured.NestedMap(foundSpokeCluster.Object, "status")
		if status != nil {
			if conditions, ok := status["conditions"]; ok {
				// check for condition
				for _, v := range conditions.([]interface{}) {
					key := v.(map[string]interface{})["type"]
					if key == "ManagedClusterConditionAvailable" {
						val := v.(map[string]interface{})["status"]
						if val == "True" {
							// exists and is available
							return true
						}
					}
				}
			}
		}

	}
	return false
}

// given a version, retrieves the matching rootfs
func (c Client) GetRootFsFromVersion(version string) (string, error) {
	gvr := schema.GroupVersionResource{
		Group:    "agent-install.openshift.io",
		Version:  "v1beta1",
		Resource: "agentserviceconfigs",
	}

	foundConfig, err := c.KubernetesClient.Resource(gvr).Get(context.Background(), "agent", v1.GetOptions{})

	if err != nil {
		log.Error(err)
		return "", err
	}

	if foundConfig != nil {
		// retrieve the images section
		spec, _, _ := unstructured.NestedMap(foundConfig.Object, "spec")
		images := spec["osImages"]

		// iterate over images until we find the matching version
		for _, v := range images.([]interface{}) {
			key := v.(map[string]interface{})["openshiftVersion"]
			if key == version {
				val := v.(map[string]interface{})["rootFSUrl"].(string)
				return val, nil
			}
		}

	}
	return "", err
}

// function to retrieve the openshift version and retrieve rootfs
func (c Client) GetRootFSUrl() (string, error) {
	// retrieve clusterdeployment for that spoke
	gvr := schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "clusterdeployments",
	}

	foundSpokeCluster, err := c.KubernetesClient.Resource(gvr).Namespace(c.Spoke).Get(context.Background(), c.Spoke, v1.GetOptions{})

	if err != nil {
		log.Error(err)
		return "", err
	}

	// transform to typed and retrieve version
	version := ""
	if foundSpokeCluster != nil {
		metadata, _, _ := unstructured.NestedMap(foundSpokeCluster.Object, "metadata")
		labels := metadata["labels"].(map[string]interface{})

		// check the label that starts with matching pattern
		for k, v := range labels {
			if k == "hive.openshift.io/version-major-minor" {
				// we have the version
				version = v.(string)
				break
			}
		}

		if version != "" {
			// we have version, let's extract rootfs
			rootfs, err := c.GetRootFsFromVersion(version)

			if err != nil {
				return "", err
			}

			return rootfs, nil
		}

	}
	return "", nil

}

// function to query an imageset and return the image
func (c Client) GetImageFromImageSet(name string) (string, error) {
	gvr := schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "clusterimagesets",
	}

	foundImageset, err := c.KubernetesClient.Resource(gvr).Get(context.Background(), name, v1.GetOptions{})

	if err != nil {
		log.Error(err)
		return "", err
	}

	if foundImageset != nil {
		// retrieve the images section
		spec, _, _ := unstructured.NestedMap(foundImageset.Object, "spec")
		release := spec["releaseImage"]

		if release != nil {
			return release.(string), nil
		}

	}
	return "", err
}

// function to retrieve a Release Image of a given cluster
func (c Client) GetReleaseImage() (string, error) {
	// retrieve agentclusterinstall for that spoke
	gvr := schema.GroupVersionResource{
		Group:    "extensions.hive.openshift.io",
		Version:  "v1beta1",
		Resource: "agentclusterinstalls",
	}

	foundSpokeCluster, err := c.KubernetesClient.Resource(gvr).Namespace(c.Spoke).Get(context.Background(), c.Spoke, v1.GetOptions{})

	if err != nil {
		log.Error(err)
		return "", err
	}

	// transform to typed and retrieve version
	if foundSpokeCluster != nil {
		spec, _, _ := unstructured.NestedMap(foundSpokeCluster.Object, "spec")
		imageSetRef := spec["imageSetRef"].(map[string]interface{})

		if imageSetRef != nil {
			clusterImageSetName := imageSetRef["name"].(string)
			if clusterImageSetName != "" {
				// need to retrieve url from imageset
				releaseImage, err := c.GetImageFromImageSet(clusterImageSetName)

				if err != nil {
					log.Error(err)
					return "", err
				}

				return releaseImage, nil
			}
		}
	}

	return "", nil
}

// create a generic placement binding
func (c Client) CreatePlacementBinding(PlacementBindingName string, PlacementRuleName string) error {
	var backupPolicy bytes.Buffer
	tmpl := template.New("policyBackupPlacementBindingTemplate")
	tmpl.Parse(policyBackupPlacementBindingTemplate)

	// create a new object for live image
	b := PlacementBinding{PlacementBindingName, PlacementRuleName}
	if err := tmpl.Execute(&backupPolicy, b); err != nil {
		log.Error(err)
		return err
	}

	// convert to unstructured
	finalPolicy := &unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode(backupPolicy.Bytes(), nil, finalPolicy)
	if err != nil {
		log.Error(err)
		return err
	}

	// once we have the policy, apply it
	gvr := schema.GroupVersionResource{
		Group:    "policy.open-cluster-management.io",
		Version:  "v1",
		Resource: "placementbindings",
	}

	_, err = c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Create(context.Background(), finalPolicy, v1.CreateOptions{})
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

// launch the backup for the spoke cluster, for the specific image
func (c Client) LaunchLiveImageBackup(liveImg string) error {
	// create placement binding in case it does not exist
	c.CreatePlacementBinding("placement-binding-backup-live-image", LIVE_POLICY)

	var backupPolicy bytes.Buffer
	tmpl := template.New("policyBackupLiveImageTemplate")
	tmpl.Parse(policyBackupLiveImageTemplate)

	// create a new object for live image
	b := BackupImageSpoke{c.Spoke, LIVE_POLICY, c.BinaryImage, liveImg, c.BackupPath, RandomString(4)}
	if err := tmpl.Execute(&backupPolicy, b); err != nil {
		log.Error(err)
		return err
	}

	// convert to unstructured
	finalPolicy := &unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode(backupPolicy.Bytes(), nil, finalPolicy)
	if err != nil {
		log.Error(err)
		return err
	}

	// once we have the policy, apply it
	gvr := schema.GroupVersionResource{
		Group:    "policy.open-cluster-management.io",
		Version:  "v1",
		Resource: "policies",
	}

	_, err = c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Create(context.Background(), finalPolicy, v1.CreateOptions{})
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

// creates a global placement rule for spoke
func (c Client) CreatePlacementRule() error {
	var backupPolicy bytes.Buffer
	tmpl := template.New("policySpokePlacementRuleTemplate")
	tmpl.Parse(policySpokePlacementRuleTemplate)

	// create a new object for spoke rule
	b := BackupImageSpoke{c.Spoke, "", "", "", "", ""}
	if err := tmpl.Execute(&backupPolicy, b); err != nil {
		log.Error(err)
		return err
	}

	// convert to unstructured
	finalPolicy := &unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode(backupPolicy.Bytes(), nil, finalPolicy)
	if err != nil {
		log.Error(err)
		return err
	}

	// once we have the rule, apply it
	gvr := schema.GroupVersionResource{
		Group:    "apps.open-cluster-management.io",
		Version:  "v1",
		Resource: "placementrules",
	}

	_, err = c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Create(context.Background(), finalPolicy, v1.CreateOptions{})
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

// removes all previously created resources
func (c Client) RemovePreviousResources() error {
	PoliciesList := []string{LIVE_POLICY, RELEASE_POLICY}

	for _, policy := range PoliciesList {
		// check if policy exists
		gvr := schema.GroupVersionResource{
			Group:    "policy.open-cluster-management.io",
			Version:  "v1",
			Resource: "policies",
		}

		resource, _ := c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Get(context.Background(), policy, v1.GetOptions{})

		if resource != nil {
			// remove policy
			log.Info(fmt.Sprintf("Policy %s still exists, removing it", policy))
			err := c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Delete(context.Background(), policy, v1.DeleteOptions{})
			if err != nil {
				return err
			}
		}

	}
	return nil

}

// launch the backup for the spoke cluster, for the specific release image
func (c Client) LaunchReleaseImageBackup(releaseImg string) error {
	// create placement binding in case it does not exist
	c.CreatePlacementBinding("placement-binding-backup-release-image", RELEASE_POLICY)

	var backupPolicy bytes.Buffer
	tmpl := template.New("policyBackupReleaseImageTemplate")
	tmpl.Parse(policyBackupReleaseImageTemplate)

	// create a new object for live image
	b := BackupImageSpoke{c.Spoke, RELEASE_POLICY, c.BinaryImage, releaseImg, c.BackupPath, RandomString(4)}
	if err := tmpl.Execute(&backupPolicy, b); err != nil {
		log.Error(err)
		return err
	}

	// convert to unstructured
	finalPolicy := &unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode(backupPolicy.Bytes(), nil, finalPolicy)
	if err != nil {
		log.Error(err)
		return err
	}

	// once we have the policy, apply it
	gvr := schema.GroupVersionResource{
		Group:    "policy.open-cluster-management.io",
		Version:  "v1",
		Resource: "policies",
	}

	_, err = c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Create(context.Background(), finalPolicy, v1.CreateOptions{})
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

// checks if the expected policies are already removed
func (c Client) NoPoliciesExist() bool {
	gvr := schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}

	live, _ := c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Get(context.Background(), LIVE_POLICY, v1.GetOptions{})
	release, _ := c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Get(context.Background(), RELEASE_POLICY, v1.GetOptions{})

	return (live != nil && release != nil)

}

// checks status of policies and deletes if completed
func (c *Client) MonitorPolicies() bool {
	gvr := schema.GroupVersionResource{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}

	live, _ := c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Get(context.Background(), LIVE_POLICY, v1.GetOptions{})
	release, _ := c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Get(context.Background(), RELEASE_POLICY, v1.GetOptions{})

	if live == nil && release == nil {
		// no policies there, we are fine to go
		log.Info("All policies have been reconciled")
		return true
	}

	// check resource version
	liveVersion, _, _ := unstructured.NestedMap(live.Object, "metadata")
	if c.CurrentLiveVersion == "" {
		c.CurrentLiveVersion = liveVersion["resourceVersion"].(string)
	}
	releaseVersion, _, _ := unstructured.NestedMap(release.Object, "metadata")
	if c.CurrentReleaseVersion == "" {
		c.CurrentReleaseVersion = releaseVersion["resourceVersion"].(string)
	}

	// check status of each policy and remove if compliant , and has been refreshed
	statusLive, _, _ := unstructured.NestedMap(live.Object, "status")
	if statusLive["compliant"] == "Compliant" && c.CurrentLiveVersion != liveVersion["resourceVersion"].(string) {
		log.Info(fmt.Sprintf("Policy %s has reconciled, deleting it", LIVE_POLICY))
		c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Delete(context.Background(), LIVE_POLICY, v1.DeleteOptions{})
	}
	statusRelease, _, _ := unstructured.NestedMap(release.Object, "status")
	if statusRelease["compliant"] == "Compliant" && c.CurrentReleaseVersion != releaseVersion["resourceVersion"].(string) {
		log.Info(fmt.Sprintf("Policy %s has reconciled, deleting it", RELEASE_POLICY))
		c.KubernetesClient.Resource(gvr).Namespace(NAMESPACE).Delete(context.Background(), RELEASE_POLICY, v1.DeleteOptions{})
	}

	return false
}

// waits until policies are completed, and clean resources
func (c Client) WaitForCompletion() error {
	ticker := time.NewTicker(time.Second * time.Duration(RETRY_PERIOD_SECONDS)).C
	timeout := time.After(time.Minute * time.Duration(TIMEOUT_MINUTES))

	for {
		select {
		case <-timeout:
			// exit with timeout
			log.Error("Exited with timeout")
			os.Exit(1)

		case <-ticker:
			if c.MonitorPolicies() {
				return nil
			}
		}
	}

}
