/*
Copyright 2021.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

const policyBackupLiveImageTemplate = `
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: policy-backup-live-image
  namespace: open-cluster-management
  annotations:
    policy.open-cluster-management.io/standards: CM-2 Baseline Configuration
    policy.open-cluster-management.io/categories: NIST SP 800-53
    policy.open-cluster-management.io/controls: CM Configuration Management
spec:
  remediationAction: inform
  disabled: false
  policy-templates:
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: backup-live-image
        spec:
          remediationAction: inform
          severity: low
          namespaceSelector:
            exclude:
              - kube-*
            include:
              - {{ .SpokeName }}
          object-templates:
            - complianceType: musthave
              objectDefinition:
                apiVersion: batch/v1
                kind: Job
                metadata:
                  name: backup-live-image
                  namespace: {{ .SpokeName }}
                spec:
                  concurrencyPolicy: Forbid
                  jobTemplate:
                    spec:
                      backoffLimit: 0
                      template:
                        spec:						  
                            containers:
                              - name: backup-live-image
                                image: {{ .LiveImageBinaryImageName }}
                                command:
                                - /bin/openshift-ai-image-backup
                                - backupLiveImage
                                - -u
                                - {{ .LiveImageURL }}
                                - -p
                                - {{ .RecoveryPartitionPath }}
                            securityContext:
                              privileged: true
                            terminationMessagePath: /dev/termination-log
                            terminationMessagePolicy: FallbackToLogsOnError
                            nodeSelector:
                              node-role.kubernetes.io/master: ''
                            restartPolicy: Never
                            tolerations:
                              - effect: NoSchedule
                                operator: Exists
                              - effect: NoExecute
                                operator: Exists
                startingDeadlineSeconds: 200
                suspend: false
`

const policySpokePlacementRule = `
apiVersion: apps.open-cluster-management.io/v1
kind: PlacementRule
metadata:
  name: placement-policy-backup-spoke
spec:
  clusterConditions:
  - status: "True"
    type: ManagedClusterConditionAvailable
  clusterSelector:
    matchExpressions:
      - {key: name, operator: In, values: ["{{ .SpokeName }} "]}	  
`

const policyBackupLiveImagePlacementBinding = `
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: placement-binding-live-spoke
  placementRef:
    name: placement-policy-backup-spoke
    kind: PlacementRule
  apiGroup: apps.open-cluster-management.io
subjects:
- name: policy-backup-live-image
  kind: Policy
  apiGroup: policy.open-cluster-management.io
`
