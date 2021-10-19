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
  name: {{ .PolicyName }}
  namespace: open-cluster-management
  annotations:
    policy.open-cluster-management.io/standards: CM-2 Baseline Configuration
    policy.open-cluster-management.io/categories: NIST SP 800-53
    policy.open-cluster-management.io/controls: CM Configuration Management
spec:
  remediationAction: enforce
  disabled: false
  policy-templates:
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: backup-live-image
        spec:
          remediationAction: enforce
          severity: low
          namespaceSelector:
            exclude:
              - kube-*
            include:
              - {{ .SpokeName }}
          object-templates:
            - complianceType: musthave
              objectDefinition:
                apiVersion: v1
                kind: ServiceAccount
                metadata:
                  name: disaster-recovery-creator
            - complianceType: musthave
              objectDefinition:
                apiVersion: rbac.authorization.k8s.io/v1
                kind: ClusterRoleBinding
                metadata:
                  name: disaster-recovery-crb
                roleRef:
                  name: cluster-admin
                  apiGroup: rbac.authorization.k8s.io
                  kind: ClusterRole
                subjects:
                  - name: disaster-recovery-creator
                    namespace: "{{ .SpokeName }}"
                    kind: ServiceAccount                  
            - complianceType: musthave
              objectDefinition:
                apiVersion: batch/v1
                kind: Job
                metadata:
                  name: backup-live-image-{{ .RandomId }}
                spec:
                  backoffLimit: 5
                  ttlSecondsAfterFinished: 100
                  template:
                    metadata:
                      name: backup-live-image
                    spec:						  
                      containers:
                      - name: backup-live-image
                        image: {{ .ImageBinaryImageName }}
                        args: ["backupLiveImage", "--RootFSURL", "{{ .ImageURL }}", "--BackupPath", "{{ .RecoveryPartitionPath }}/liveImage"]
                        securityContext:
                          privileged: true
                        volumeMounts:
                        - name: live-image
                          mountPath: "{{ .RecoveryPartitionPath }}"
                      nodeSelector:
                        node-role.kubernetes.io/master: ''
                      restartPolicy: OnFailure
                      serviceAccount: disaster-recovery-creator
                      serviceAccountName: disaster-recovery-creator
                      volumes:
                      - name: live-image
                        hostPath:
                          path: "{{ .RecoveryPartitionPath }}"
                          type: Directory
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: check-live-image-job-status
        spec:
          remediationAction: inform
          severity: low
          object-templates:
            - complianceType: musthave
              objectDefinition:
                apiVersion: batch/v1
                kind: Job
                metadata:
                  name: backup-live-image-{{ .RandomId }}
                  namespace: {{ .SpokeName }}
                status:
                  conditions:
                    - type: Complete                  
`

const policySpokePlacementRuleTemplate = `
apiVersion: apps.open-cluster-management.io/v1
kind: PlacementRule
metadata:
  name: placement-policy-backup-spoke
  namespace: open-cluster-management
spec:
  clusterConditions:
  - status: "True"
    type: ManagedClusterConditionAvailable
  clusterSelector:
    matchExpressions:
      - {key: name, operator: In, values: ["{{ .SpokeName }}"]}	  
`

const policyBackupPlacementBindingTemplate = `
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: {{ .PlacementName }}
  namespace: open-cluster-management
placementRef:
  name: placement-policy-backup-spoke
  kind: PlacementRule
  apiGroup: apps.open-cluster-management.io
subjects:
- name: {{ .PolicyName }}
  kind: Policy
  apiGroup: policy.open-cluster-management.io
`

const policyBackupReleaseImageTemplate = `
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: {{ .PolicyName }}
  namespace: open-cluster-management
  annotations:
    policy.open-cluster-management.io/standards: CM-2 Baseline Configuration
    policy.open-cluster-management.io/categories: NIST SP 800-53
    policy.open-cluster-management.io/controls: CM Configuration Management
spec:
  remediationAction: enforce
  disabled: false
  policy-templates:
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: backup-release-image
        spec:
          remediationAction: enforce
          severity: low
          namespaceSelector:
            exclude:
              - kube-*
            include:
              - {{ .SpokeName }}
          object-templates:
            - complianceType: musthave
              objectDefinition:
                apiVersion: v1
                kind: ServiceAccount
                metadata:
                  name: disaster-recovery-creator
            - complianceType: musthave
              objectDefinition:
                apiVersion: rbac.authorization.k8s.io/v1
                kind: ClusterRoleBinding
                metadata:
                  name: disaster-recovery-crb
                roleRef:
                  name: cluster-admin
                  apiGroup: rbac.authorization.k8s.io
                  kind: ClusterRole
                subjects:
                  - name: disaster-recovery-creator
                    namespace: "{{ .SpokeName }}"
                    kind: ServiceAccount                  
            - complianceType: musthave
              objectDefinition:
                apiVersion: batch/v1
                kind: Job
                metadata:
                  name: backup-release-image-{{ .RandomId }}
                spec:
                  backoffLimit: 5
                  ttlSecondsAfterFinished: 100
                  template:
                    metadata:
                      name: backup-release-image
                    spec:						  
                      containers:
                      - name: backup-release-image
                        image: {{ .ImageBinaryImageName }}
                        args: ["backupReleaseImage", "--ReleaseImageURL", "{{ .ImageURL }}", "--BackupPath", "{{ .RecoveryPartitionPath }}/releaseImage"]
                        securityContext:
                          privileged: true
                        volumeMounts:
                        - name: release-image
                          mountPath: "{{ .RecoveryPartitionPath }}"
                      nodeSelector:
                        node-role.kubernetes.io/master: ''
                      restartPolicy: OnFailure
                      serviceAccount: disaster-recovery-creator
                      serviceAccountName: disaster-recovery-creator
                      volumes:
                      - name: release-image
                        hostPath:
                          path: "{{ .RecoveryPartitionPath }}"
                          type: Directory
    - objectDefinition:
        apiVersion: policy.open-cluster-management.io/v1
        kind: ConfigurationPolicy
        metadata:
          name: check-release-image-job-status
        spec:
          remediationAction: inform
          severity: low
          object-templates:
            - complianceType: musthave
              objectDefinition:
                apiVersion: batch/v1
                kind: Job
                metadata:
                  name: backup-release-image-{{ .RandomId }}
                  namespace: {{ .SpokeName }}
                status:
                  conditions:
                    - type: Complete                  
`
