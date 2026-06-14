package istio

import (
	"fmt"
	"sort"

	"graph/internal/models"
	"graph/internal/utils"

	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

// Predefined issue codes surfaced in MtlsState.Issues. Format: "<code>: <detail>"
// when detail adds context, bare "<code>" otherwise. Stable across versions —
// grep/jq-friendly.
const (
	IssueRootSelectorIgnored = "root selector ignored"
	IssueDuplicateAtScope    = "duplicate at scope"
	IssueAllUnset            = "all peer authentications are set to Unset"
	IssuePortWithoutSelector = "port level without selector"
)

// resolveMtls walks Istio's PeerAuthentication precedence (workload → namespace
// → mesh) and returns the effective MtlsState for the workload. Oldest PA wins
// at each scope (Istio's tie-break by creationTimestamp). UNSET falls through
// to the next scope; if every scope is UNSET (or empty), defaults to PERMISSIVE.
func resolveMtls(workload models.WorkloadNode, nsPAs []*istiosec.PeerAuthentication, rootPAs []*istiosec.PeerAuthentication) *models.MtlsState {
	state := &models.MtlsState{}
	var issues []string

	// Oldest-first so first match in each bucket = winner.
	sortByCreation(nsPAs)
	sortByCreation(rootPAs)

	// Bucket matching PAs by precedence rank.
	var workloadPAs, nsScopedPAs, meshPAs []*istiosec.PeerAuthentication

	for _, pa := range nsPAs {
		if pa.Spec.Selector == nil && len(pa.Spec.PortLevelMtls) > 0 {
			issues = append(issues, fmt.Sprintf("%s: %s/%s has port rules but no selector, so the rules apply to every workload in %s. Add a selector to target specific workloads, or move the port rules into a workload-specific policy.",
				IssuePortWithoutSelector, pa.Namespace, pa.Name, pa.Namespace))
		}
		if pa.Spec.Selector != nil {
			if utils.LabelsMatch(pa.Spec.Selector.MatchLabels, workload.Labels) {
				workloadPAs = append(workloadPAs, pa)
			}
		} else {
			nsScopedPAs = append(nsScopedPAs, pa)
		}
	}

	for _, pa := range rootPAs {
		if pa.Spec.Selector != nil {
			// Root-ns selectorful PA is namespace-local to root only.
			issues = append(issues, fmt.Sprintf("%s: %s/%s uses a selector but lives in %s. Istio only matches workloads inside %s when a selector is set on a root-ns policy. To target workloads in other namespaces, move this policy into the workload's namespace.",
				IssueRootSelectorIgnored, pa.Namespace, pa.Name, RootNamespace, RootNamespace))
			if workload.Namespace == RootNamespace &&
				utils.LabelsMatch(pa.Spec.Selector.MatchLabels, workload.Labels) {
				workloadPAs = append(workloadPAs, pa)
			}
			continue
		}
		meshPAs = append(meshPAs, pa)
	}

	// Same-scope conflicts: oldest wins, rest are silently ignored by Istio.
	if loser := dupDetail("workload", workloadPAs); loser != "" {
		issues = append(issues, loser)
	}
	if loser := dupDetail("namespace", nsScopedPAs); loser != "" {
		issues = append(issues, loser)
	}
	if loser := dupDetail("mesh", meshPAs); loser != "" {
		issues = append(issues, loser)
	}

	// Sources: precedence order, oldest-first within each bucket.
	for _, pa := range workloadPAs {
		state.Sources = append(state.Sources, convert(pa))
	}
	for _, pa := range nsScopedPAs {
		state.Sources = append(state.Sources, convert(pa))
	}
	for _, pa := range meshPAs {
		state.Sources = append(state.Sources, convert(pa))
	}

	// Workload-level verdict: first non-UNSET in precedence order.
	verdict, effective := pickVerdict(workloadPAs, nsScopedPAs, meshPAs)
	if effective != nil {
		state.EffectiveSource = models.MtlsPolicyApplied{
			Namespace: effective.Namespace,
			Name:      effective.Name,
		}
		state.Verdict = verdict
	} else {
		state.Verdict = models.MeshPermissive
		if len(state.Sources) > 0 {
			issues = append(issues, IssueAllUnset+": every matching policy is set to UNSET, so Istio falls back to the install default (permissive). Set an explicit mode (STRICT, PERMISSIVE, or DISABLE) on at least one policy if you wanted specific behavior.")
		}
	}

	// Per-port overrides: same precedence walk per referenced port.
	state.PortOverrides = resolvePortOverrides(workloadPAs, nsScopedPAs, meshPAs)

	state.Issues = issues
	return state
}

// sortByCreation orders PAs oldest-first using metadata.creationTimestamp.
// Ties break by name (lexicographic) for determinism.
func sortByCreation(pas []*istiosec.PeerAuthentication) {
	sort.SliceStable(pas, func(i, j int) bool {
		ti := pas[i].CreationTimestamp.Time
		tj := pas[j].CreationTimestamp.Time
		if ti.Equal(tj) {
			return pas[i].Name < pas[j].Name
		}
		return ti.Before(tj)
	})
}

// pickVerdict walks workload → ns → mesh and returns the first non-UNSET mode
// along with the PA that produced it. Returns (MeshUnset, nil) if nothing wins.
func pickVerdict(workloadPAs, nsScopedPAs, meshPAs []*istiosec.PeerAuthentication) (models.MeshScope, *istiosec.PeerAuthentication) {
	for _, bucket := range [][]*istiosec.PeerAuthentication{workloadPAs, nsScopedPAs, meshPAs} {
		for _, pa := range bucket {
			scope := mtlsModeToScope(pa.Spec.Mtls.GetMode())
			if scope != models.MeshUnset {
				return scope, pa
			}
		}
	}
	return models.MeshUnset, nil
}

// resolvePortOverrides collects every port referenced by any matching PA and
// runs the precedence walk for each. UNSET at a port falls through; ports
// nowhere referenced are absent from the map (caller treats absent = Verdict).
func resolvePortOverrides(workloadPAs, nsScopedPAs, meshPAs []*istiosec.PeerAuthentication) map[uint32]models.MeshScope {
	portsSeen := map[uint32]struct{}{}
	for _, bucket := range [][]*istiosec.PeerAuthentication{workloadPAs, nsScopedPAs, meshPAs} {
		for _, pa := range bucket {
			for port := range pa.Spec.PortLevelMtls {
				portsSeen[port] = struct{}{}
			}
		}
	}
	if len(portsSeen) == 0 {
		return nil
	}
	out := make(map[uint32]models.MeshScope, len(portsSeen))
	for port := range portsSeen {
		for _, bucket := range [][]*istiosec.PeerAuthentication{workloadPAs, nsScopedPAs, meshPAs} {
			scope := pickPortScope(bucket, port)
			if scope != models.MeshUnset {
				out[port] = scope
				break
			}
		}
	}
	return out
}

// pickPortScope returns the first non-UNSET portLevelMtls mode for port within
// a single precedence bucket (PAs already oldest-first).
func pickPortScope(bucket []*istiosec.PeerAuthentication, port uint32) models.MeshScope {
	for _, pa := range bucket {
		portMtls, ok := pa.Spec.PortLevelMtls[port]
		if !ok {
			continue
		}
		scope := mtlsModeToScope(portMtls.GetMode())
		if scope != models.MeshUnset {
			return scope
		}
	}
	return models.MeshUnset
}

// dupDetail returns a formatted IssueDuplicateAtScope message when bucket has
// more than one PA at the same scope. Empty string when no conflict.
func dupDetail(scope string, bucket []*istiosec.PeerAuthentication) string {
	if len(bucket) < 2 {
		return ""
	}
	winner := bucket[0]
	losers := make([]string, 0, len(bucket)-1)
	for _, pa := range bucket[1:] {
		losers = append(losers, fmt.Sprintf("%s/%s", pa.Namespace, pa.Name))
	}
	return fmt.Sprintf("%s: two policies at the %s level — %s/%s is older and is the one Istio uses. These are silently ignored: %v. Delete the unused ones or merge their settings into the winning policy.",
		IssueDuplicateAtScope, scope, winner.Namespace, winner.Name, losers)
}
