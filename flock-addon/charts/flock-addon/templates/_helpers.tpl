{{- /*
-----------------------------------------------------------------------------
flock-addon helpers
-----------------------------------------------------------------------------
Named templates shared across the chart. Each block below documents the
contract it owns; changing the contract usually requires touching either
the Makefile dispatcher (`make enable-addon`) or the AddOnTemplate that
references the rendered value.
-----------------------------------------------------------------------------
*/ -}}

{{- /*
-----------------------------------------------------------------------------
flock-addon.flockAllianceImage
-----------------------------------------------------------------------------
Renders the value injected into each AddOnDeploymentConfig's
FLOCK_ALLIANCE_IMAGE customizedVariable. The AddOnTemplate references that
variable verbatim as the container `image` field, so any change to the
image rendering format MUST stay compatible with `repository:tag` shape —
the template does not parse it further.
-----------------------------------------------------------------------------
*/ -}}
{{- define "flock-addon.flockAllianceImage" -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}

{{- /*
-----------------------------------------------------------------------------
flock-addon.templateName / flock-addon.gpuTemplateName
-----------------------------------------------------------------------------
Owns the AddOnTemplate naming convention. The CPU template name is also
referenced as the default in ClusterManagementAddOn.spec.supportedConfigs
and by the GPU/CPU dispatch in `make enable-addon`, so both names must
stay stable across releases.

The "-gpu" suffix is hard-coded here (not a value) because the dispatcher
in the Makefile encodes the same suffix; keeping the convention in one
place would require teaching the Makefile how to read Helm values, which
is not worth the indirection.
-----------------------------------------------------------------------------
*/ -}}
{{- define "flock-addon.templateName" -}}
{{- .Values.addon.name -}}
{{- end -}}

{{- define "flock-addon.gpuTemplateName" -}}
{{- printf "%s-gpu" .Values.addon.name -}}
{{- end -}}
