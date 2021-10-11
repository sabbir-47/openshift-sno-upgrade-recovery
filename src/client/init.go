package client

import (
	"fmt"
	"k8s.io/client-go/tools/clientcmd"
	log "github.com/sirupsen/logrus"
)

type Client struct {
	KubeconfigPath string
	Spoke string
}

func New(KubeconfigPath string, Spoke string) ( Client, error ) {
	c := Client {KubeconfigPath, Spoke}

	// establish kubernetes connection
	config, err := clientcmd.BuildConfigFromFlags("", KubeconfigPath)
	fmt.Println("%T", config)
	if err != nil {
		log.Error(err)
		return c, err
	}	

	return c, nil
}