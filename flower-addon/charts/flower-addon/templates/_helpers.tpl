{{/*
Expand the name of the chart.
*/}}
{{- define "flower-addon.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "flower-addon.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "flower-addon.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "flower-addon.labels" -}}
helm.sh/chart: {{ include "flower-addon.chart" . }}
{{ include "flower-addon.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "flower-addon.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flower-addon.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
SuperLink labels
*/}}
{{- define "flower-addon.superlinkLabels" -}}
app.kubernetes.io/name: flower
app.kubernetes.io/component: superlink
{{- end }}

{{/*
SuperLink selector labels
*/}}
{{- define "flower-addon.superlinkSelectorLabels" -}}
app.kubernetes.io/name: flower
app.kubernetes.io/component: superlink
{{- end }}

{{/*
SuperNode image
*/}}
{{- define "flower-addon.supernodeImage" -}}
{{- printf "%s:%s" .Values.supernode.image.repository .Values.supernode.image.tag }}
{{- end }}
