// Package istio implements mesh.MeshSource for Istio (ambient mode).
//
// Both methods are per-workload, called from the detail endpoint:
//   - Membership: cheap label check (ns + workload labels) → InMesh + Provider
//   - ResolveMtls: fetches PAs for ns + root ns, walks precedence → MtlsState
//
// See docs/arch/0003-istio-mesh-membership-and-mtls.md.
package istio

import (
	"log/slog"
	//"context"

	//istioapi "istio.io/api/security/v1beta1"
	//istiosec "istio.io/client-go/pkg/apis/security/v1"

	"alatyr/internal/k8s"
	//"alatyr/internal/models"
)

const SourceName = "istio"

// Label keys + sentinel values used to detect mesh membership and waypoint
const (
	// LabelDataplaneMode marks ambient-mesh enrollment. Applied to a
	// Namespace for ns-wide opt-in, or to a pod template / workload for a
	// per-workload override.
	AmbientEnrollmentKey = "istio.io/dataplane-mode"

	// DataplaneModeAmbient = workload participates in ambient mesh (ztunnel
	// handles L4 + mTLS; optional waypoint for L7).
	AmbientEnrollmentValue = "ambient"
	AmbientSkipValue       = "none"

	// LabelUseWaypoint binds a workload / ServiceAccount / namespace to a
	// waypoint Gateway. Value is the waypoint name (or "<ns>/<name>" for
	// cross-ns). Special value WaypointNone opts out at the label's scope.
	WaypointLabelKey  = "istio.io/use-waypoint"
	WaypointSkipValue = "none"

	RootNamespace    = "istio-system"
	IngressNamespace = "istio-ingress"
	ZtunnelHBONEPort = 15008

	// istio components labels
	istioSelector = "app" // key for finding all istio resources
	gatewayVal    = "istio-ingress"
	istiodVal     = "istiod"
	ztunelVal     = "ztunnel"

	// istio cni selectors
	istioCniKey = "k8s-app"
	istioCniVal = "istio-cni"
)

type source struct {
	client k8s.KubernetesClient
	log    *slog.Logger
}

// ZtunnelHBONEPort is the inbound HBONE port ztunnel listens on for mesh
// traffic in ambient mode. NetworkPolicies restricting ingress must allow
// this port; otherwise mesh-routed traffic is silently dropped at the CNI.

func New(client k8s.KubernetesClient, logger *slog.Logger) *source {
	if logger == nil {
		logger = slog.Default()
	}
	return &source{client: client, log: logger}
}

func (s *source) Name() string {
	return SourceName
}
