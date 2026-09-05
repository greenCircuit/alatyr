package istio

import (
	istioapi "istio.io/api/security/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"

	"alatyr/internal/models"
)

// isIstioComponent reports whether a workload is an Istio control-plane or
// gateway pod (istiod, ztunnel, istio-cni, ingress gateway). These carry
// istio.io/dataplane-mode: none because they are the mesh proxies, not ambient
// data-plane members — the ambient opt-out must not exclude them from mesh
// membership. Both the identifying label and the namespace must match: the
// ingress gateway lives in IngressNamespace, everything else in RootNamespace,
// so an app in a user namespace reusing a label (app=istiod) is not mistaken
// for the control plane.
func isIstioComponent(workload models.WorkloadNode) bool {
	switch workload.Namespace {
	case IngressNamespace:
		return workload.Labels[istioSelector] == gatewayVal
	case RootNamespace:
		if workload.Labels[istioSelector] == istiodVal || workload.Labels[istioSelector] == ztunelVal {
			return true
		}
		return workload.Labels[istioCniKey] == istioCniVal
	}
	return false
}

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

// participatesInMesh reports whether a workload is reachable over HBONE — either
// label-enrolled, or a system component in the root / ingress namespace, whose
// ztunnel/gateway pods always speak HBONE even without the enrollment label.
// Mirrors the namespace rule in Membership so the 15008 gate and membership agree.
func participatesInMesh(workload models.WorkloadNode, nsLabels map[string]string) bool {
	if workload.Namespace == RootNamespace || workload.Namespace == IngressNamespace {
		return true
	}
	return inAmbientMesh(workload.Labels, nsLabels)
}
