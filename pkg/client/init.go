package client

import (
	"bytes"
	"context"
	"fmt"

	"text/template"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	KubeconfigPath   string
	Spoke            string
	BinaryImage      string
	BackupPath       string
	KubernetesClient dynamic.Interface
}

type BackupLiveImageSpoke struct {
	SpokeName                string
	LiveImageBinaryImageName string
	LiveImageURL             string
	RecoveryPartitionPath    string
}

func New(KubeconfigPath string, Spoke string, BinaryImage string, BackupPath string) (Client, error) {
	c := Client{KubeconfigPath, Spoke, BinaryImage, BackupPath, nil}

	// establish kubernetes connection
	config, err := clientcmd.BuildConfigFromFlags("", KubeconfigPath)
	if err != nil {
		log.Error(err)
		return c, err
	}

	// now try to connect to cluster
	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Error(err)
		return c, err
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
		fmt.Println(release)

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

// launch the backup for the spoke cluster, for the specific image
func (c Client) LaunchLiveImageBackup(liveImg string) error {
	// create policy from template

	var backupPolicy bytes.Buffer
	tmpl := template.New("policyBackupLiveImageTemplate")
	tmpl.Parse(policyBackupLiveImageTemplate)

	// create a new object for live image
	b := BackupLiveImageSpoke{c.Spoke, c.BinaryImage, liveImg, fmt.Sprintf("%s/%s", c.BackupPath, "liveImage")}
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

	_, err = c.KubernetesClient.Resource(gvr).Namespace("open-cluster-management").Create(context.Background(), finalPolicy, v1.CreateOptions{})
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}
