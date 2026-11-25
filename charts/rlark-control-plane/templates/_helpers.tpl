{{/*
Expand the name of the chart.
*/}}
{{- define "rlark-control-plane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "rlark-control-plane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "rlark-control-plane.labels" -}}
helm.sh/chart: {{ include "rlark-control-plane.chart" . }}
{{ include "rlark-control-plane.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "rlark-control-plane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "rlark-control-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels
*/}}
{{- define "rlark-control-plane.componentLabels" -}}
app: {{ .componentName }}
{{- end }}

{{/*
Resolve the effective storage config for a component.
Merges component-level storage over global storage.
Usage: include "rlark-control-plane.storage" (dict "root" . "component" .Values.kcp)
*/}}
{{- define "rlark-control-plane.storage" -}}
{{- $global := .root.Values.global.storage }}
{{- $component := .component.storage }}
{{- if $component.type }}
type: {{ $component.type }}
hostPath: {{ default "" $component.hostPath }}
storageClass: {{ default "" $component.storageClass }}
size: {{ default "10Gi" $component.size }}
nodeSelector: {{ toYaml (default dict $component.nodeSelector) }}
{{- else }}
type: {{ default "emptyDir" $global.type }}
hostPath: {{ default "" $global.hostPath }}
storageClass: {{ default "" $global.storageClass }}
size: {{ default "10Gi" $global.size }}
nodeSelector: {{ toYaml (default dict $global.nodeSelector) }}
{{- end }}
{{- end }}

{{/*
Return the image pull policy
*/}}
{{- define "rlark-control-plane.imagePullPolicy" -}}
{{- default "Always" .Values.global.imagePullPolicy }}
{{- end }}

{{/*
Return image pull secrets
*/}}
{{- define "rlark-control-plane.imagePullSecrets" -}}
{{- with .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Return the common environment variables injected into most component containers.
*/}}
{{- define "rlark-control-plane.commonEnvs" -}}
- name: POD_NAME
  valueFrom:
    fieldRef:
      fieldPath: metadata.name
- name: POD_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
- name: NODE_NAME
  valueFrom:
    fieldRef:
      fieldPath: spec.nodeName
- name: POD_IP
  valueFrom:
    fieldRef:
      fieldPath: status.podIP
- name: DISABLE_HTTP2
  value: "true"
{{- end }}

{{/*
Return the args for connecting to the management API (--in-cluster or --kubeconfig).
Usage: include "rlark-control-plane.managementArgs" .
*/}}
{{- define "rlark-control-plane.managementArgs" -}}
{{- if eq .Values.managementApi "kubernetes" }}
- --in-cluster
- --kube-namespace={{ .Release.Namespace }}
{{- else }}
- --kubeconfig
- /etc/kcp/admin.kubeconfig
- --kube-namespace=default
{{- end }}
{{- end }}

{{/*
Return volumes for management API connection.
Usage: include "rlark-control-plane.managementVolumes" .
*/}}
{{- define "rlark-control-plane.managementVolumes" -}}
{{- if ne .Values.managementApi "kubernetes" }}
- name: kcp-kubeconfig
  configMap:
    name: rlark-kcp-kubeconfig
{{- end }}
{{- end }}

{{/*
Return volume mounts for management API connection.
Usage: include "rlark-control-plane.managementVolumeMounts" .
*/}}
{{- define "rlark-control-plane.managementVolumeMounts" -}}
{{- if ne .Values.managementApi "kubernetes" }}
- name: kcp-kubeconfig
  mountPath: /etc/kcp/admin.kubeconfig
  subPath: admin.kubeconfig
  readOnly: true
{{- end }}
{{- end }}

{{/*
Return the management API RBAC rules (ClusterRole rules).
*/}}
{{- define "rlark-control-plane.managementRBACRules" -}}
- apiGroups: ["rlinf.io"]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: [""]
  resources: ["configmaps", "events", "namespaces", "secrets", "serviceaccounts"]
  verbs: ["*"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles", "rolebindings", "clusterroles", "clusterrolebindings"]
  verbs: ["*"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["*"]
{{- end }}
