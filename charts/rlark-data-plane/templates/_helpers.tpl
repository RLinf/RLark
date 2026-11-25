{{/*
Expand the name of the chart.
*/}}
{{- define "rlark-data-plane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "rlark-data-plane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "rlark-data-plane.labels" -}}
helm.sh/chart: {{ include "rlark-data-plane.chart" . }}
{{ include "rlark-data-plane.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "rlark-data-plane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "rlark-data-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Return the image pull policy
*/}}
{{- define "rlark-data-plane.imagePullPolicy" -}}
{{- default "Always" .Values.global.imagePullPolicy }}
{{- end }}

{{/*
Return image pull secrets
*/}}
{{- define "rlark-data-plane.imagePullSecrets" -}}
{{- with .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Return the common environment variables.
*/}}
{{- define "rlark-data-plane.commonEnvs" -}}
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
Resolve the SSH server address.
When sshAddress is set, use it directly; otherwise derive from controlPlaneAddress.
*/}}
{{- define "rlark-data-plane.sshServerAddress" -}}
{{- if .Values.sshAddress }}
{{- .Values.sshAddress }}
{{- else }}
{{- $host := .Values.controlPlaneAddress | replace "https://" "" | replace "http://" "" }}
{{- $host = $host | splitList ":" | first }}
{{- printf "client@%s:2222" $host }}
{{- end }}
{{- end }}
