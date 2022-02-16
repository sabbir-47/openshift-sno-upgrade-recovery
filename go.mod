module github.com/redhat-ztp/openshift-ai-trigger-backup

go 1.16

require (
	github.com/mitchellh/go-homedir v1.1.0
	github.com/openshift/build-machinery-go v0.0.0-20200512074546-3744767c4131
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/viper v1.9.0
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.9.3-0.20210709165254-650ea59f19cc
)

replace k8s.io/client-go => k8s.io/client-go v0.21.3
