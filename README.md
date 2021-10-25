
# openshift-ai-trigger-backup
Cli to orchestrate backup requests on an ACM hub. It will trigger the backup jobs from https://github.com/redhat-ztp/openshift-ai-image-backup

## How to build
A Makefile is provided in order to facilitate the building of the client. Just execute `Make build` and it will place the generate binary into `bin/backup` relative path.

## How to generate images
While this program can be executed from the command line, it is mainly designed to run from a Kubernetes cluster, using a container image. In order to build the image, please execute `Make build-image` . You can override the default parameters using the following environment vars:

 - CONTAINER_COMMAND : docker/podman
 - IMAGE: name of the quay image to produce. By default is `quay.io/redhat-ztp/openshift-ai-trigger-backup`
 - GIT_REVISION: commit of the git repository to use for the build. By default is pointing to `HEAD`
 - CONTAINER_BUILD_PARAMS: additional flags to pass to `docker build` or `podman build` command.

Once the image is successfully built, you can push it to your remote repository using `Make push-image`command . Then it's ready to be consumed

## How to use

### Running from command line
In order to run ai-trigger-backup from command line, you need to have a valid KUBECONFIG file with valid credentials to access the hub cluster. You can then launch the command, that will trigger the backup on the desired spoke cluster:
`./bin/backup backupInitialData -k /tmp/kubeconfig_karmalabs -s spoke-cluster`
This command will create two policies in the hub cluster, that will launch the backup jobs in the spoke:

    NAME                                                  REMEDIATION ACTION   COMPLIANCE STATE   AGE

    open-cluster-management.policy-backup-live-image      enforce              Compliant          9s

    open-cluster-management.policy-backup-release-image   enforce              Compliant          9s

### Running from a job
In order to run integrated from a hub cluster, triggered by other events, you need to use the `openshift-ai-trigger-backup` image that was created. As an example, you can trigger from a job:

    apiVersion: v1
    kind: ServiceAccount
    metadata:
      name: disaster-recovery-launcher
      namespace: spoke-cluster
	---
	apiVersion: rbac.authorization.k8s.io/v1
	kind: ClusterRoleBinding
	metadata:
	  name: disaster-recovery-launcher
	  namespace: spoke-cluster
	roleRef:
	  name: cluster-admin
	  apiGroup: rbac.authorization.k8s.io
	  kind: ClusterRole
	subjects:
	  - name: disaster-recovery-launcher
	    namespace: spoke-cluster
	    kind: ServiceAccount
	---
	apiVersion: batch/v1
	kind: Job
	metadata:
	  name: trigger-backup-image
	  namespace: spoke-cluster
	spec:
	  backoffLimit: 5
	  ttlSecondsAfterFinished: 100
	  template:
	    metadata:
	      name: trigger-backup-image
	      namespace: spoke-cluster
	    spec:						  
	      containers:
	      - name: trigger-backup-image
	        image: lab-installer.karmalabs.com:5000/olm/openshift-ai-trigger-backup
	        args: ["backupInitialData", "-s", "spoke-cluster"]
	      nodeSelector:
	        node-role.kubernetes.io/master: ''
	      restartPolicy: OnFailure
	      serviceAccount: disaster-recovery-launcher
	      serviceAccountName: disaster-recovery-launcher

See that you need to have a privileged account in order to run it, because it needs to access details, and manipulate content in the spoke cluster, that is only available to privileged accounts.
When executed, it will create the matching policies to trigger the backup in the specified spoke cluster.
