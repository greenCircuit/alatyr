package calico

import "alatyr/internal/models"

// globalPolicy — normalized projectcalico.org/v3 GlobalNetworkPolicy. CRD
// decoded into this shape once at the fetch boundary so resolve/status walks
// never touch vendored types. selectorMatch/nsSelectorMatch parsed once (memo).
type globalPolicy struct {
	name        string
	order       float64 // math.MaxFloat64 when Spec.Order unset (sorts last)
	tier        string
	ingress     []calicoRule
	egress      []calicoRule
	doesIngress bool // Spec.Types includes Ingress
	doesEgress  bool // Spec.Types includes Egress

	selectorMatch   workloadMatcher
	nsSelectorMatch workloadMatcher // nil = match all ns
	// selectsAllWorkloads — Spec.Selector is empty or all(), so the policy targets
	// every workload in each selected namespace by construction. Distinct from "a
	// label selector that happens to match every pod present right now".
	selectsAllWorkloads bool

	ref models.PolicyRef
}

// calicoRule — one ingress/egress entry. Array order load-bearing: first-match
// runs per rule in sequence, not as a union. Empty ports = all-ports match;
// otherwise the rule only matches buckets whose (port, protocol) tuple is
// listed. notPorts intentionally omitted for now.
type calicoRule struct {
	action  models.RuleAction
	isPass  bool // Pass — hands off to next tier
	nets    []string
	notNets []string
	ports   []models.Port
}

// nsSelection — ns-level precomputed cache. policies survives nsSelector
// filter; buckets and candidate lists are shared across every workload in
// the ns so per-workload cost is only workload-selector match, not CIDR/port
// arithmetic.
type nsSelection struct {
	policies          []globalPolicy
	egressBuckets     []bucketKey
	ingressBuckets    []bucketKey
	egressCandidates  map[bucketKey][]bucketCandidate
	ingressCandidates map[bucketKey][]bucketCandidate
}

// peerKind — which matching algebra a destination probe point uses. CIDR today
// (address-space containment). selector/serviceAccount arms land when Calico
// non-CIDR peers are wired; the tag keeps the resolution unit from being fused
// to a raw CIDR string so those slot in as new arms, not a rewrite.
type peerKind int

const (
	peerCIDR peerKind = iota
)

// peer — one destination probe point for first-match resolution. CIDR is the
// only realized kind; cidr is meaningful only when kind==peerCIDR.
type peer struct {
	kind peerKind
	cidr string
}

// bucketKey — resolution unit. (peer, port) tuple. Two rules opening different
// ports on the same peer produce two buckets → both can win. Zero-value port =
// "all ports" sentinel for the direction-wide class.
type bucketKey struct {
	peer peer
	port models.Port
}

// bucketCandidate — one policy's contribution to a bucket, already resolved
// through CIDR/port match. Per-workload resolution walks candidates in
// precedence order and picks the first whose workloadMatch closure returns
// true. netsCover / portsCover run ONCE per bucket at ns-cache build; per
// workload only pays selector match.
type bucketCandidate struct {
	winner        models.PolicyRef
	action        models.RuleAction
	workloadMatch workloadMatcher
	selectsAll    bool // winner's selector is catch-all (see globalPolicy)
}

type workloadMatcher func(labels map[string]string) bool
