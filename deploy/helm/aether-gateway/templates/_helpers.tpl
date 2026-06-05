{{/*
Expand the name of the chart.
*/}}
{{- define "aether-gateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "aether-gateway.fullname" -}}
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
{{- define "aether-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "aether-gateway.labels" -}}
helm.sh/chart: {{ include "aether-gateway.chart" . }}
{{ include "aether-gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: aether-gateway
{{- with .Values.global.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "aether-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aether-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Controlplane labels
*/}}
{{- define "aether-gateway.controlplane.labels" -}}
{{ include "aether-gateway.labels" . }}
app.kubernetes.io/component: controlplane
{{- end }}

{{/*
Controlplane selector labels
*/}}
{{- define "aether-gateway.controlplane.selectorLabels" -}}
{{ include "aether-gateway.selectorLabels" . }}
app: {{ include "aether-gateway.name" . }}-controlplane
{{- end }}

{{/*
Dataplane labels
*/}}
{{- define "aether-gateway.dataplane.labels" -}}
{{ include "aether-gateway.labels" . }}
app.kubernetes.io/component: dataplane
{{- end }}

{{/*
Dataplane selector labels
*/}}
{{- define "aether-gateway.dataplane.selectorLabels" -}}
{{ include "aether-gateway.selectorLabels" . }}
app: {{ include "aether-gateway.name" . }}-dataplane
{{- end }}

{{/*
Dashboard labels
*/}}
{{- define "aether-gateway.dashboard.labels" -}}
{{ include "aether-gateway.labels" . }}
app.kubernetes.io/component: dashboard
{{- end }}

{{/*
Dashboard selector labels
*/}}
{{- define "aether-gateway.dashboard.selectorLabels" -}}
{{ include "aether-gateway.selectorLabels" . }}
app: {{ include "aether-gateway.name" . }}-dashboard
{{- end }}

{{/*
Controlplane image
*/}}
{{- define "aether-gateway.controlplane.image" -}}
{{- $registry := .Values.controlplane.image.registry | default .Values.global.imageRegistry -}}
{{- $repository := .Values.controlplane.image.repository -}}
{{- $tag := .Values.controlplane.image.tag | default .Chart.AppVersion -}}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
Dataplane image
*/}}
{{- define "aether-gateway.dataplane.image" -}}
{{- $registry := .Values.dataplane.image.registry | default .Values.global.imageRegistry -}}
{{- $repository := .Values.dataplane.image.repository -}}
{{- $tag := .Values.dataplane.image.tag | default .Chart.AppVersion -}}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
Dashboard image
*/}}
{{- define "aether-gateway.dashboard.image" -}}
{{- $registry := .Values.dashboard.image.registry | default .Values.global.imageRegistry -}}
{{- $repository := .Values.dashboard.image.repository -}}
{{- $tag := .Values.dashboard.image.tag | default .Chart.AppVersion -}}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
Release namespace
*/}}
{{- define "aether-gateway.namespace" -}}
{{- if .Values.namespace.create }}
{{- .Values.namespace.name }}
{{- else }}
{{- .Release.Namespace }}
{{- end }}
{{- end }}

{{/*
Image pull secrets
*/}}
{{- define "aether-gateway.imagePullSecrets" -}}
{{- $secrets := list -}}
{{- range .Values.global.imagePullSecrets }}
{{- $secrets = append $secrets (dict "name" .) }}
{{- end }}
{{- if $secrets }}
imagePullSecrets:
{{ toYaml $secrets }}
{{- end }}
{{- end }}

{{/*
Security context defaults
*/}}
{{- define "aether-gateway.podSecurityContext" -}}
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  fsGroup: 65532
  seccompProfile:
    type: RuntimeDefault
{{- end }}

{{/*
Container security context
*/}}
{{- define "aether-gateway.containerSecurityContext" -}}
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
{{- end }}

{{/*
Controlplane config as YAML
*/}}
{{- define "aether-gateway.controlplane.configYaml" -}}
{{- toYaml .Values.controlplane.config }}
{{- end }}

{{/*
Dataplane config as YAML
*/}}
{{- define "aether-gateway.dataplane.configYaml" -}}
{{- $cfg := .Values.dataplane.config -}}
{{- if $cfg.controlPlaneAddr -}}
{{- $dpConfig := omit $cfg "controlPlaneAddr" -}}
controlPlaneAddr: {{ $cfg.controlPlaneAddr | quote }}
{{ toYaml $dpConfig }}
{{- else -}}
{{- $dpConfig := omit $cfg "controlPlaneAddr" -}}
controlPlaneAddr: "http://{{ include "aether-gateway.name" . }}-controlplane-grpc.{{ include "aether-gateway.namespace" . }}.svc.cluster.local:18080"
{{ toYaml $dpConfig }}
{{- end }}
{{- end }}