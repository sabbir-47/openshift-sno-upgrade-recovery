package client

import (
	"context"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/dynamic"
	log "github.com/sirupsen/logrus"
)

type Client struct {
	KubeconfigPath string
	Spoke string
	KubernetesClient dynamic.Interface
}

func New(KubeconfigPath string, Spoke string) ( Client, error ) {
	c := Client {KubeconfigPath, Spoke, nil}

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
		Group: "cluster.open-cluster-management.io",
		Version: "v1",
		Resource: "managedclusters",
	}

	foundSpokeCluster, err := c.KubernetesClient.Resource(gvr).Get(context.Background(), c.Spoke, v1.GetOptions{})
	
	if err != nil {
		log.Error(err)
		return false
	}

	// transform to typed
	if (foundSpokeCluster != nil) {
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