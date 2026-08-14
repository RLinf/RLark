{{/*
Expand the chart name.
*/}}
{{- define "embodied-runtime.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully-qualified app name. Defaults to "<release>-<chart>" unless the release
already contains the chart name (e.g. --install embodied-runtime), in which
case it collapses to the release name — so the device-plugin resource names
match the original static manifests when installed as `embodied-runtime`.
*/}}
{{- define "embodied-runtime.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "embodied-runtime.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
device-plugin resource name (DaemonSet / SA / Role / RoleBinding).
*/}}
{{- define "embodied-runtime.devicePluginName" -}}
{{- printf "%s-device-plugin" (include "embodied-runtime.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
ServiceAccount name (== device-plugin name, matching the original RBAC).
*/}}
{{- define "embodied-runtime.serviceAccountName" -}}
{{- include "embodied-runtime.devicePluginName" . -}}
{{- end -}}

{{/*
ConfigMap name.
*/}}
{{- define "embodied-runtime.configMapName" -}}
{{- printf "%s-config" (include "embodied-runtime.devicePluginName" .) -}}
{{- end -}}

{{/*
MutatingWebhookConfiguration name (cluster-scoped) created by this chart when
the webhook is enabled. The device plugin's --webhook-mutating-config flag is
set to this so its caBundle sync targets the object this chart owns.
*/}}
{{- define "embodied-runtime.webhookConfigName" -}}
{{- printf "%s-webhook" (include "embodied-runtime.devicePluginName" .) | trunc 63 -}}
{{- end -}}

{{/*
Webhook Service name. The device plugin's --webhook-service-name flag is set to
this so the serving cert DNS SANs match the Service DNS the API server uses.
*/}}
{{- define "embodied-runtime.webhookServiceName" -}}
{{- printf "%s-webhook" (include "embodied-runtime.devicePluginName" .) | trunc 63 -}}
{{- end -}}

{{/*
Whether the webhook should be rendered at all: enabled AND host_macvlans is
non-empty (the webhook only injects when macvlans are configured). Emits the
non-empty string "true" only when both hold, so `{{ if include "..." . }}`
behaves as a real boolean (an emitted "false" string would be truthy).
*/}}
{{- define "embodied-runtime.webhookEnabled" -}}
{{- if and .Values.webhook.enabled (gt (len .Values.config.hostMacvlans) 0) -}}true{{- end -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "embodied-runtime.labels" -}}
app.kubernetes.io/name: {{ include "embodied-runtime.name" . }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels (must not change after install — immutable).
*/}}
{{- define "embodied-runtime.selectorLabels" -}}
app.kubernetes.io/name: {{ include "embodied-runtime.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
