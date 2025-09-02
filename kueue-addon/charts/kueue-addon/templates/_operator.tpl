{{/*
Operator Resources - Single Template
*/}}
{{- define "kueue-addon.operatorResources" -}}
{{- if and .Values.kueueOperator .Values.certManagerOperator .Values.operatorLifecycleManager }}
- apiVersion: rbac.authorization.k8s.io/v1
  kind: ClusterRoleBinding
  metadata:
    name: {{ .Values.operatorLifecycleManager.clusterRoleBindingName }}
  roleRef:
    apiGroup: rbac.authorization.k8s.io
    kind: ClusterRole
    name: {{ .Values.operatorLifecycleManager.clusterRoleName }}
  subjects:
  - kind: ServiceAccount
    name: klusterlet-work-sa
    namespace: open-cluster-management-agent
- apiVersion: v1
  kind: Namespace
  metadata:
    name: {{ .Values.kueueOperator.namespace }}
- apiVersion: operators.coreos.com/v1
  kind: OperatorGroup
  metadata:
    name: {{ .Values.kueueOperator.operatorGroupName }}
    namespace: {{ .Values.kueueOperator.namespace }}
  spec:
    upgradeStrategy: Default
- apiVersion: operators.coreos.com/v1alpha1
  kind: Subscription
  metadata:
    name: {{ .Values.kueueOperator.name }}
    namespace: {{ .Values.kueueOperator.namespace }}
  spec:
    channel: {{ .Values.kueueOperator.channel }}
    installPlanApproval: Automatic
    name: {{ .Values.kueueOperator.name }}
    source: {{ .Values.kueueOperator.source }}
    sourceNamespace: {{ .Values.kueueOperator.sourceNamespace }}
    startingCSV: {{ .Values.kueueOperator.startingCSV }}
- apiVersion: kueue.openshift.io/v1
  kind: Kueue
  metadata:
    name: cluster
    annotations:
      addon.open-cluster-management.io/deletion-orphan: ""
    labels:
      app.kubernetes.io/name: kueue-operator
  spec:
    {{- if .Values.kueueCR.spec }}
{{ toYaml .Values.kueueCR.spec | indent 4 }}
    {{- end }}
- apiVersion: v1
  kind: Namespace
  metadata:
    name: {{ .Values.certManagerOperator.namespace }}
- apiVersion: operators.coreos.com/v1
  kind: OperatorGroup
  metadata:
    name: {{ .Values.certManagerOperator.operatorGroupName }}
    namespace: {{ .Values.certManagerOperator.namespace }}
  spec:
    upgradeStrategy: Default
- apiVersion: operators.coreos.com/v1alpha1
  kind: Subscription
  metadata:
    name: {{ .Values.certManagerOperator.name }}
    namespace: {{ .Values.certManagerOperator.namespace }}
  spec:
    channel: {{ .Values.certManagerOperator.channel }}
    installPlanApproval: Automatic
    name: {{ .Values.certManagerOperator.name }}
    source: {{ .Values.certManagerOperator.source }}
    sourceNamespace: {{ .Values.certManagerOperator.sourceNamespace }}
    startingCSV: {{ .Values.certManagerOperator.startingCSV }}
{{- end }}
{{- end }}
