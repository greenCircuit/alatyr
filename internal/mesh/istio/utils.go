package istio

import (
	istioapi "istio.io/api/security/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"

	"graph/internal/models"
)

// convert maps a PeerAuthentication to MtlsSource. Scope is derived from
// where the PA lives + whether it has a selector.
func convert(pa *istiosec.PeerAuthentication) models.MtlsSource {
	result := models.MtlsSource{
		Namespace: pa.Namespace,
		Name:      pa.Name,
	}
	if pa.Namespace == RootNamespace {
		result.MeshSource = models.MeshGlobal
	} else if pa.Spec.Selector == nil {
		result.MeshSource = models.MeshNs
	} else {
		result.MeshSource = models.MeshWorkload
	}

	result.MeshScope = mtlsModeToScope(pa.Spec.Mtls.GetMode())

	if len(pa.Spec.PortLevelMtls) > 0 {
		result.PortModes = make(map[uint32]models.MeshScope, len(pa.Spec.PortLevelMtls))
		for portNumber, portMtls := range pa.Spec.PortLevelMtls {
			result.PortModes[portNumber] = mtlsModeToScope(portMtls.GetMode())
		}
	}

	return result
}

// mtlsModeToScope maps Istio MutualTLS_Mode to MeshScope. UNSET stays
// distinct from PERMISSIVE so the resolver can fall through.
func mtlsModeToScope(mode istioapi.PeerAuthentication_MutualTLS_Mode) models.MeshScope {
	switch mode {
	case istioapi.PeerAuthentication_MutualTLS_DISABLE:
		return models.MeshDisable
	case istioapi.PeerAuthentication_MutualTLS_PERMISSIVE:
		return models.MeshPermissive
	case istioapi.PeerAuthentication_MutualTLS_STRICT:
		return models.MeshStrict
	default:
		return models.MeshUnset
	}
}

// inAmbientMesh returns true when the workload is enrolled. Workload label
// wins over namespace label.
func inAmbientMesh(workloadLabels, nsLabels map[string]string) bool {
	if v, ok := workloadLabels[AmbientEnrollmentKey]; ok {
		return v == AmbientEnrollmentValue
	}

	return nsLabels[AmbientEnrollmentKey] == AmbientEnrollmentValue
}

// effectiveMode returns the per-port override if set, else Verdict.
// port=0 means no port specified.
func effectiveMode(state *models.MtlsState, port uint32) models.MeshScope {
	if port != 0 && state.PortOverrides != nil {
		if override, ok := state.PortOverrides[port]; ok {
			return override
		}
	}
	return state.Verdict
}
