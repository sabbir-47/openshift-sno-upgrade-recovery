package client

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"text/template"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	Spoke            string
	BackupPath       string
	KubeconfigPath   string
	KubernetesClient dynamic.Interface
}

type TemplateData struct {
	ResourceName string
	ClusterName  string
	RecoveryPath string
}

// ResourceTemplate define a resource template structure
type ResourceTemplate struct {
	// Must always correspond the Action or View resource name
	resourceName string
	template     string
}

var BackupCreateTemplates = []ResourceTemplate{
	{"backup-create-namespace", mngClusterActCreateNS},
	{"backup-create-serviceaccount", mngClusterActCreateSA},
	{"backup-create-rolebinding", mngClusterActCreateRB},
	{"backup-create-job", mngClusterActCreateJob},
	{"backup-create-clusterview", mngClusterViewJob},
}

var JobDeleteTemplates = []ResourceTemplate{
	{"backup-delete-ns", mngClusterActDeleteNS},
}

func New(Spoke string, BackupPath string, KubeconfigPath string) (Client, error) {
	rand.Seed(time.Now().UnixNano())
	c := Client{Spoke, BackupPath, KubeconfigPath, nil}

	var clientset dynamic.Interface

	if KubeconfigPath != "" {
		// generate config from file
		config, err := c.GetConfig()
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

	log.Info(fmt.Sprintf("Checking if the Spoke cluster: %s exist...", c.Spoke))
	foundSpokeCluster, err := c.KubernetesClient.Resource(gvr).Get(context.Background(), c.Spoke, v1.GetOptions{})

	if err != nil {
		log.Error(err)
		return false
	}

	// transform to typed
	if foundSpokeCluster != nil {
		status, _, err := unstructured.NestedMap(foundSpokeCluster.Object, "status")
		if err != nil {
			log.Error(err)
			return false
		}
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
	log.Info(fmt.Sprintf("VERIFIED: Spoke cluster: %s exists", c.Spoke))
	return false
}

func (c Client) GetConfig() (*rest.Config, error) {
	config, err := clientcmd.BuildConfigFromFlags("", c.KubeconfigPath)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return config, nil
}

func (c Client) LaunchKubernetesObjects(KubeconfigPath string, action string, cluster string) error {

	//	config := ctrl.GetConfigOrDie()
	//	dynamic := dynamic.NewForConfigOrDie(config)
	config, err := c.GetConfig()
	if err != nil {
		log.Error(err)
		return err
	}

	newdata := TemplateData{
		ResourceName: "",
		ClusterName:  c.Spoke,
		RecoveryPath: c.BackupPath,
	}

	var templates []ResourceTemplate

	if cluster == "spoke" {
		templates = JobDeleteTemplates
	} else {
		templates = BackupCreateTemplates
	}

	for _, item := range templates {
		obj := &unstructured.Unstructured{}
		newdata.ResourceName = item.resourceName

		log.Info("\n\n")
		log.Info(strings.Repeat("-", 60))
		log.Info(fmt.Printf("####### Creating kubernetes object: [ %s ]", item.resourceName))
		log.Info(strings.Repeat("-", 60))
		log.Info("\n\n")

		log.Info(fmt.Printf("rendering resource: %s, data passed: %s\n", item.resourceName, newdata))
		w, err := c.renderYamlTemplate(item.resourceName, item.template, newdata)
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
		log.Info("Retreiving GVK....")
		// decode YAML into unstructured.Unstructured
		dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		_, gvk, err := dec.Decode(w.Bytes(), nil, obj)
		if err != nil {
			return err
		}

		log.Info(fmt.Printf("Retrieved GVK: %s\n", gvk))

		log.Info("Mapping gvk to gvr with discovery client....")

		// Map GVK to GVR with discovery client
		discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
		if err != nil {
			return err
		}
		mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return err
		}

		log.Info("Mapping has been successfully done")
		// Build resource
		resource := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: mapping.Resource.Resource,
		}

		switch action {
		case "delete":
			log.Info(fmt.Printf("DELETING the resource: [%s] at namespace: [backupresource] of spoke: [%s] ....\n", item.resourceName, c.Spoke))
			if err := c.DeleteKubernetesObjects(resource, item.resourceName); err != nil {
				log.Errorf("Couldn't remove object: [%s], err: [%s]", item.resourceName, err)
				return nil
			}
		case "create":
			log.Info(fmt.Printf("CREATING the resource: [%s] at namespace: [backupresource] of spoke: [%s] ....\n", item.resourceName, c.Spoke))
			err = c.CreateKubernetesObjects(obj, resource)
			if err != nil {
				log.Error(err)
				return err
			}
			log.Info("\n\n")
			log.Info(strings.Repeat("-", 60))
			log.Info(fmt.Printf("################# Successfully created the resource: [%s] at namespace: backupresource of spoke: [%s] ....\n\n", item.resourceName, c.Spoke))
			log.Info(strings.Repeat("-", 60))
			log.Info("\n\n")
		default:
			log.Errorf("No condition matched")
			return err

		}

	}
	return nil
}

func (c Client) renderYamlTemplate(resourceName string, TemplateData string, data TemplateData) (*bytes.Buffer, error) {

	w := new(bytes.Buffer)

	log.Info(fmt.Printf("Parsing template: %s", resourceName))

	tmpl, err := template.New(resourceName).Parse(commonTemplates + TemplateData)
	if err != nil {
		return w, fmt.Errorf("failed to parse template %s: %v", resourceName, err)
	}
	data.ResourceName = resourceName
	err = tmpl.Execute(w, data)
	if err != nil {
		return w, fmt.Errorf("failed to render template %s: %v", resourceName, err)
	}
	log.Info(fmt.Printf("Successfully parsed template: %s", resourceName))
	return w, nil
}

func (c Client) CreateKubernetesObjects(obj *unstructured.Unstructured, resource schema.GroupVersionResource) error {

	_, err := c.KubernetesClient.Resource(resource).Namespace(c.Spoke).Create(context.Background(), obj, v1.CreateOptions{})
	if err != nil {
		log.Info(fmt.Printf("err is : %s", err))
		return err
	}
	return nil
}

func (c Client) ListMCAobjects() error {

	gvr := schema.GroupVersionResource{
		Group:    "action.open-cluster-management.io",
		Version:  "v1beta1",
		Resource: "managedclusteractions",
	}

	resource, err := c.KubernetesClient.Resource(gvr).Namespace(c.Spoke).List(context.Background(), v1.ListOptions{})
	if err != nil {
		log.Info(fmt.Printf("err is : %s", err))
		return nil
	}
	log.Info("\n\n")
	log.Info(strings.Repeat("-", 60))
	log.Info(fmt.Printf("List of mca \n %s", resource.Object))
	log.Info(strings.Repeat("-", 60))
	return nil
}

func (c Client) ManageView(action string) (*unstructured.Unstructured, error) {

	gvr := schema.GroupVersionResource{
		Group:    "view.open-cluster-management.io",
		Version:  "v1beta1",
		Resource: "managedclusterviews",
	}

	name := "backup-create-clusterview"
	var view *unstructured.Unstructured

	switch action {
	case "list":
		view, err := c.KubernetesClient.Resource(gvr).Namespace(c.Spoke).Get(context.Background(), name, v1.GetOptions{})
		if err != nil {
			log.Info(fmt.Printf("err is : %s", err))
			return view, err
		}
		return view, nil
	case "delete":
		err := c.KubernetesClient.Resource(gvr).Namespace(c.Spoke).Delete(context.Background(), name, v1.DeleteOptions{})
		if err != nil {
			log.Info(fmt.Printf("err is : %s", err))
			return nil, err
		}
		log.Info(fmt.Printf("----------------- Successfully deleted the managedclusterview resource named: [%s] ---------------", name))
	default:
		log.Errorf("No condition matched")
		return nil, fmt.Errorf("no condition matched")
	}
	return view, nil
}

func (c Client) CheckViewProcessing(viewConditions []interface{}) string {
	var status, message string
	for _, condition := range viewConditions {
		status = condition.(map[string]interface{})["status"].(string)
		message = condition.(map[string]interface{})["type"].(string)
		log.Info(fmt.Printf("job status from mcv status: [%s], message: [%s]", status, message))
	}
	return status
}

func (c Client) CheckStatus() error {

	// this is static for now, it should be parametrized.
	for i := 0; i < 10; i++ {

		time.Sleep(5 * time.Second)
		log.Info(".... checking if managedclusterview related to job is present .....")

		clusterView, err := c.ManageView("list")
		if err != nil {
			log.Errorf("Couldn't find managedclusterview from %s cluster; err: %s", c.Spoke, err)
			return err
		}
		log.Info("Found managedclusterview object")

		conditions, exists, err := unstructured.NestedSlice(clusterView.Object, "status", "conditions")
		if err != nil {
			log.Error(err)
			return err
		}

		if !exists {
			return fmt.Errorf("couldn't find the structure")
		}
		value := c.CheckViewProcessing(conditions)
		log.Info(fmt.Printf("value is %s", value))
		if value == "True" {
			break
		}

	}
	log.Info(".... out of the loop .....")
	return nil
}

func (c Client) DeleteKubernetesObjects(resource schema.GroupVersionResource, name string) error {

	// we dont need resource going forward, since it queries and do rest mapping from gvk to gvr, it creates cpu and memory load to the server.
	// we can loop through resource template and delte by resource type.
	err := c.KubernetesClient.Resource(resource).Namespace(c.Spoke).Delete(context.Background(), name, v1.DeleteOptions{})
	if err != nil {
		log.Info(fmt.Printf("err is : %s", err))
		//	return err
	}
	return nil
}

func (c Client) DeleteSpokeJob(resource schema.GroupVersionResource, name string) error {

	// we dont need resource going forward, since it queries and do rest mapping from gvk to gvr, it creates cpu and memory load to the server.
	// we can loop through resource template and delte by resource type.
	err := c.KubernetesClient.Resource(resource).Namespace(c.Spoke).Delete(context.Background(), name, v1.DeleteOptions{})
	if err != nil {
		log.Info(fmt.Printf("err is : %s", err))
		//	return err
	}
	return nil
}

const commonTemplates string = `
{{ define "actionGVK" }}
apiVersion: action.open-cluster-management.io/v1beta1
kind: ManagedClusterAction
{{ end }}
{{ define "viewGVK" }}
apiVersion: view.open-cluster-management.io/v1beta1
kind: ManagedClusterView
{{ end }}
{{ define "metadata"}}
metadata:
  name: {{ .ResourceName }}
  namespace: {{ .ClusterName }}
{{ end }}
`
const mngClusterActCreateNS = `
{{ template "actionGVK"}}
{{ template "metadata" . }}
spec: 
  actionType: Create
  kube: 
    resource: namespace
    template: 
      apiVersion: v1
      kind: Namespace
      metadata: 
        name: backupresource
`
const mngClusterActCreateSA = `
{{ template "actionGVK"}}
{{ template "metadata" . }}
spec:
  actionType: Create
  kube:
    resource: serviceaccount
    template:
      apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: backupresource
        namespace: backupresource
`
const mngClusterActCreateRB = `
{{ template "actionGVK"}}
{{ template "metadata" . }}
spec:
  actionType: Create
  kube:
    resource: clusterrolebinding
    template:
      apiVersion: rbac.authorization.k8s.io/v1
      kind: ClusterRoleBinding
      metadata:
        name: backupResource
      roleRef:
        apiGroup: rbac.authorization.k8s.io
        kind: ClusterRole
        name: cluster-admin
      subjects:
        - kind: ServiceAccount
          name: backupresource
          namespace: backupresource
`
const mngClusterActCreateJob string = `
{{ template "actionGVK"}}
{{ template "metadata" . }}
spec:
  actionType: Create
  kube:
    namespace: backupresource
    resource: job
    template:
      apiVersion: batch/v1
      kind: Job
      metadata:
        name: backupresource
      spec:
        backoffLimit: 0
        template:
          spec:
            containers:
              -
                args:
                  - launchBackup
                  - "--BackupPath"
                  - /var/recovery
                image: 2620-52-0-1302--b88f.sslip.io:5000/olm/openshift-ai-image-backup:latest
                name: container-image
                securityContext:
                  privileged: true
                  runAsUser: 0
                tty: true
                volumeMounts:
                  -
                    mountPath: /host
                    name: backup
            restartPolicy: Never
            hostNetwork: true
            serviceAccountName: backupresource
            volumes:
              -
                hostPath:
                  path: /
                  type: Directory
                name: backup
`
const mngClusterActDeleteNS string = `
{{ template "actionGVK"}}
{{ template "metadata" . }}
spec: 
  actionType: Delete
  kube: 
    name: backupresource
    resource: namespace
`
const mngClusterViewJob string = `
{{ template "viewGVK"}}
{{ template "metadata" . }}
spec:
  scope:
    resource: jobs
    name: backupresource
    namespace: backupresource
`
