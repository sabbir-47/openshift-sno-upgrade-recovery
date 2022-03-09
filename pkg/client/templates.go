package client

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
                image: 2620-52-0-1302--1db3.sslip.io:5000/olm/openshift-ai-image-backup:latest
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
