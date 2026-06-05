{{/*
Expand the name of the chart.
*/}}
{{- define "nantian-gw.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "nantian-gw.fullname" -}}
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
{{- define "nantian-gw.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nantian-gw.labels" -}}
helm.sh/chart: {{ include "nantian-gw.chart" . }}
{{ include "nantian-gw.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: nantian-gw
{{- with .Values.global.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "nantian-gw.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nantian-gw.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Controlplane labels
*/}}
{{- define "nantian-gw.controlplane.labels" -}}
{{ include "nantian-gw.labels" . }}
app.kubernetes.io/component: controlplane
{{- end }}

{{/*
Controlplane selector labels
*/}}
{{- define "nantian-gw.controlplane.selectorLabels" -}}
{{ include "nantian-gw.selectorLabels" . }}
app: {{ include "nantian-gw.name" . }}-controlplane
{{- end }}

{{/*
Dataplane labels
*/}}
{{- define "nantian-gw.dataplane.labels" -}}
{{ include "nantian-gw.labels" . }}
app.kubernetes.io/component: dataplane
{{- end }}

{{/*
Dataplane selector labels
*/}}
{{- define "nantian-gw.dataplane.selectorLabels" -}}
{{ include "nantian-gw.selectorLabels" . }}
app: {{ include "nantian-gw.name" . }}-dataplane
{{- end }}

{{/*
Dashboard labels
*/}}
{{- define "nantian-gw.dashboard.labels" -}}
{{ include "nantian-gw.labels" . }}
app.kubernetes.io/component: dashboard
{{- end }}

{{/*
Dashboard selector labels
*/}}
{{- define "nantian-gw.dashboard.selectorLabels" -}}
{{ include "nantian-gw.selectorLabels" . }}
app: {{ include "nantian-gw.name" . }}-dashboard
{{- end }}

{{/*
Controlplane image
*/}}
{{- define "nantian-gw.controlplane.image" -}}
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
{{- define "nantian-gw.dataplane.image" -}}
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
{{- define "nantian-gw.dashboard.image" -}}
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
{{- define "nantian-gw.namespace" -}}
{{- if .Values.namespace.create }}
{{- .Values.namespace.name }}
{{- else }}
{{- .Release.Namespace }}
{{- end }}
{{- end }}

{{/*
Image pull secrets
*/}}
{{- define "nantian-gw.imagePullSecrets" -}}
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
{{- define "nantian-gw.podSecurityContext" -}}
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
{{- define "nantian-gw.containerSecurityContext" -}}
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
{{- define "nantian-gw.controlplane.configYaml" -}}
{{- toYaml .Values.controlplane.config }}
{{- end }}

{{/*
Dataplane config as YAML
*/}}
{{- define "nantian-gw.dataplane.configYaml" -}}
{{- $cfg := .Values.dataplane.config -}}
{{- if $cfg.controlPlaneAddr -}}
{{- $dpConfig := omit $cfg "controlPlaneAddr" -}}
controlPlaneAddr: {{ $cfg.controlPlaneAddr | quote }}
{{ toYaml $dpConfig }}
{{- else -}}
{{- $dpConfig := omit $cfg "controlPlaneAddr" -}}
controlPlaneAddr: "http://{{ include "nantian-gw.name" . }}-controlplane-grpc.{{ include "nantian-gw.namespace" . }}.svc.cluster.local:18080"
{{ toYaml $dpConfig }}
{{- end }}
{{- end }}