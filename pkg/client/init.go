package client

import (
	"context"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	KubeconfigPath   string
	Spoke            string
	KubernetesClient dynamic.Interface
}

func New(KubeconfigPath string, Spoke string) (Client, error) {
	c := Client{KubeconfigPath, Spoke, nil}

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
	// retrieve agentserviceconfig for that spoke
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
