package client

import (
	"k8s.io/client-go/tools/clientcmd"
	log "github.com/sirupsen/logrus"
)

type Client struct {
	KubeconfigPath string
	Spoke string
}

funct New(KubeconfigPath string, Spoke string) {
	c := Client {KubeconfigPath, Spoke}

	// establish kubernetes connection
	config, err := clientcmd.BuildConfigFromFlags("", *KubeconfigPath)
	if err != nil {
		log.Error(err)
		return err
	}	

	return c, nil
}