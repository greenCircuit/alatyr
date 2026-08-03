package calico

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"graph/internal/k8s"
	"graph/internal/models"

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/lib/numorstring"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// buildTestPolicy builds a globalPolicy with a workload-selector matcher that
// always matches the given label predicate. `nets` empty on a calicoRule means
// catch-all; supply "0.0.0.0/0" explicitly to test the bucket-derivation path.
func buildTestPolicy(name string, doesEgress, doesIngress bool, egress, ingress []calicoRule) globalPolicy {
	return globalPolicy{
		name:            name,
		order:           100,
		tier:            "default",
		ingress:         ingress,
		egress:          egress,
		doesEgress:      doesEgress,
		doesIngress:     doesIngress,
		selectorMatch:   func(map[string]string) bool { return true },
		nsSelectorMatch: nil,
		ref: models.PolicyRef{
			Source:    sourceName,
			Name:      name,
			Namespace: "",
		},
	}
}

func newTestResult() *models.EvaluationResult {
	return &models.EvaluationResult{
		AllowByNs:      map[string][]models.Rule{},
		DenyByNs:       map[string][]models.Rule{},
		PolicyStatuses: map[string]models.PolicyStatus{},
		NodePolicies:   map[string][]models.PolicyRef{},
		NodeRules:      map[string]models.NodeRules{},
		Nodes:          map[string]models.WorkloadNode{},
	}
}

// testCache mirrors what Evaluate builds per-ns. buildTestPolicy always
// leaves nsSelectorMatch=nil so every policy survives the ns filter.
func testCache(policies []globalPolicy) nsSelection {
	return buildNsSelection(policies, nil)
}

func testWorkload() models.WorkloadNode {
	return models.WorkloadNode{
		ID:        "workload-1",
		Namespace: "ns-a",
		Labels:    map[string]string{"app": "web"},
		Type:      models.NodeTypeDeployment,
	}
}

// Empty nets on a Calico rule = catch-all destination. Allow verdict for the
// catch-all bucket must stamp CoverageAllowAll on the emitted models.Rule so
// reach detection reads it as a direction-wide allow.
func TestResolveWorkload_CoverageAllowAll(t *testing.T) {
	policy := buildTestPolicy("allow-all-egress", true, false,
		[]calicoRule{{action: models.ActionAllow, nets: nil}},
		nil,
	)
	result := newTestResult()

	(&source{}).resolveWorkload(testCache([]globalPolicy{policy}), testWorkload(), "ns-a", result)

	nodeRules, ok := result.NodeRules["workload-1"]
	if !ok {
		t.Fatal("expected NodeRules entry for workload-1")
	}
	if len(nodeRules.Egress.Allow) != 1 {
		t.Fatalf("expected 1 egress allow rule, got %d", len(nodeRules.Egress.Allow))
	}
	got := nodeRules.Egress.Allow[0]
	if got.Coverage != models.CoverageAllowAll {
		t.Errorf("Coverage: got %q, want %q", got.Coverage, models.CoverageAllowAll)
	}
	if got.Action != models.ActionAllow {
		t.Errorf("Action: got %v, want ActionAllow", got.Action)
	}
	if got.Direction != models.DirectionEgress {
		t.Errorf("Direction: got %v, want DirectionEgress", got.Direction)
	}
	if got.Contributor.Name != "allow-all-egress" {
		t.Errorf("Contributor.Name: got %q, want allow-all-egress", got.Contributor.Name)
	}
	// egress blanket: workload is SrcID, peer side left empty (matches k8s pattern)
	if got.SrcID != "workload-1" {
		t.Errorf("SrcID: got %q, want workload-1", got.SrcID)
	}
	if got.DstID != "" {
		t.Errorf("DstID: got %q, want empty (blanket rule has no peer endpoint)", got.DstID)
	}
	// no phantom cidr:0.0.0.0/0 node — Coverage marker replaces it
	if _, ok := result.Nodes[models.CIDRIDPrefix+catchAllCIDR]; ok {
		t.Error("catchAll bucket must not synthesize a CIDR node; Coverage marker carries semantic")
	}
}

// Catch-all bucket with Deny verdict must stamp CoverageDenyAll — the
// direction-wide-drop marker downstream reach detection promotes to a
// blanket deny reason.
func TestResolveWorkload_CoverageDenyAll(t *testing.T) {
	policy := buildTestPolicy("deny-all-ingress", false, true,
		nil,
		[]calicoRule{{action: models.ActionDeny, nets: nil}},
	)
	result := newTestResult()

	(&source{}).resolveWorkload(testCache([]globalPolicy{policy}), testWorkload(), "ns-a", result)

	nodeRules := result.NodeRules["workload-1"]
	if len(nodeRules.Ingress.Deny) != 1 {
		t.Fatalf("expected 1 ingress deny rule, got %d", len(nodeRules.Ingress.Deny))
	}
	got := nodeRules.Ingress.Deny[0]
	if got.Coverage != models.CoverageDenyAll {
		t.Errorf("Coverage: got %q, want %q", got.Coverage, models.CoverageDenyAll)
	}
	if got.Direction != models.DirectionIngress {
		t.Errorf("Direction: got %v, want DirectionIngress", got.Direction)
	}
	// ingress blanket: workload is DstID, peer side left empty (matches k8s pattern)
	if got.DstID != "workload-1" {
		t.Errorf("DstID: got %q, want workload-1", got.DstID)
	}
	if got.SrcID != "" {
		t.Errorf("SrcID: got %q, want empty (blanket rule has no peer endpoint)", got.SrcID)
	}
	if _, ok := result.Nodes[models.CIDRIDPrefix+catchAllCIDR]; ok {
		t.Error("catchAll bucket must not synthesize a CIDR node; Coverage marker carries semantic")
	}
}

// Narrower CIDR bucket (not the catch-all) must stamp CoverageRestricted —
// signals "per-CIDR rule, not direction-wide" to reach detection.
func TestResolveWorkload_CoverageRestricted(t *testing.T) {
	policy := buildTestPolicy("allow-lan", true, false,
		[]calicoRule{{action: models.ActionAllow, nets: []string{"10.0.0.0/8"}}},
		nil,
	)
	result := newTestResult()

	(&source{}).resolveWorkload(testCache([]globalPolicy{policy}), testWorkload(), "ns-a", result)

	nodeRules := result.NodeRules["workload-1"]
	if len(nodeRules.Egress.Allow) != 1 {
		t.Fatalf("expected 1 egress allow rule, got %d", len(nodeRules.Egress.Allow))
	}
	got := nodeRules.Egress.Allow[0]
	if got.Coverage != models.CoverageRestricted {
		t.Errorf("Coverage: got %q, want %q", got.Coverage, models.CoverageRestricted)
	}
	if got.DstID != models.CIDRIDPrefix+"10.0.0.0/8" {
		t.Errorf("DstID: got %q, want %q", got.DstID, models.CIDRIDPrefix+"10.0.0.0/8")
	}
	// CIDR endpoint must be added to result.Nodes so graph rendering can draw it.
	if _, ok := result.Nodes[models.CIDRIDPrefix+"10.0.0.0/8"]; !ok {
		t.Error("expected CIDR endpoint 10.0.0.0/8 in result.Nodes")
	}
}

// Buckets exist because SOME policy named a net; the winner may be a different
// policy that named none. A catch-all Allow wins every bucket it is first-match
// for, and re-emitting it once per net draws arrows to CIDRs that policy never
// mentions — the graph would claim scope the manifest does not have. Those
// restatements must be dropped, while a bucket whose winner genuinely differs
// from the direction-wide one still draws.
func TestResolveWorkload_CatchAllWinnerDoesNotRestateNamedNets(t *testing.T) {
	clusterNets := []string{"10.42.0.0/16", "10.43.0.0/16", "192.168.8.0/24"}
	// Real shape: one GNP allows the cluster ranges then drops everything else.
	// It contributes the buckets whether or not it wins any of them.
	defaultDeny := buildTestPolicy("egress-default-deny", true, false,
		[]calicoRule{
			{action: models.ActionAllow, nets: clusterNets},
			{action: models.ActionDeny},
		},
		nil,
	)
	enroll := buildTestPolicy("egress-enroll-namespaces", true, false,
		[]calicoRule{{action: models.ActionAllow}}, // no nets = catch-all
		nil,
	)

	// enroll first = lower order, so it wins every bucket including the named ones.
	result := newTestResult()
	(&source{}).resolveWorkload(testCache([]globalPolicy{enroll, defaultDeny}), testWorkload(), "ns-a", result)

	allows := result.NodeRules["workload-1"].Egress.Allow
	if len(allows) != 1 {
		for _, rule := range allows {
			t.Logf("emitted: dst=%q coverage=%q by=%s", rule.DstID, rule.Coverage, rule.Contributor.Name)
		}
		t.Fatalf("catch-all winner: want 1 direction-wide allow, got %d", len(allows))
	}
	if allows[0].Coverage != models.CoverageAllowAll || allows[0].DstID != "" {
		t.Errorf("surviving rule: want the blanket marker, got coverage %q dst %q", allows[0].Coverage, allows[0].DstID)
	}
	for _, cidr := range clusterNets {
		if _, ok := result.Nodes[models.CIDRIDPrefix+cidr]; ok {
			t.Errorf("%s: winner never named this range — no CIDR node may be synthesized", cidr)
		}
	}

	// Reverse the order and the named buckets have a different winner (and action)
	// than the direction-wide bucket, so every one of them is real and stays.
	ordered := newTestResult()
	(&source{}).resolveWorkload(testCache([]globalPolicy{defaultDeny, enroll}), testWorkload(), "ns-a", ordered)

	orderedRules := ordered.NodeRules["workload-1"].Egress
	if len(orderedRules.Allow) != len(clusterNets) {
		t.Fatalf("narrow winner: want %d per-net allows, got %d", len(clusterNets), len(orderedRules.Allow))
	}
	if len(orderedRules.Deny) != 1 || orderedRules.Deny[0].Coverage != models.CoverageDenyAll {
		t.Fatalf("narrow winner: want 1 blanket deny, got %+v", orderedRules.Deny)
	}
}

// Policy selects the workload but doesEgress=false — calico has no opinion
// on egress. Reach detection needs an explicit CoverageUnenforced marker so
// the direction gets read as "no restriction" rather than silent deny.
func TestResolveWorkload_UnenforcedEgressWhenIngressOnly(t *testing.T) {
	policy := buildTestPolicy("ingress-only", false, true,
		nil,
		[]calicoRule{{action: models.ActionAllow, nets: nil}},
	)
	result := newTestResult()

	(&source{}).resolveWorkload(testCache([]globalPolicy{policy}), testWorkload(), "ns-a", result)

	nodeRules := result.NodeRules["workload-1"]
	if len(nodeRules.Egress.Allow) != 1 {
		t.Fatalf("expected 1 synthetic egress rule, got %d", len(nodeRules.Egress.Allow))
	}
	synthetic := nodeRules.Egress.Allow[0]
	if synthetic.Coverage != models.CoverageUnenforced {
		t.Errorf("Coverage: got %q, want %q", synthetic.Coverage, models.CoverageUnenforced)
	}
	if synthetic.Contributor.Name != "ingress-only" {
		t.Errorf("Contributor.Name: got %q, want ingress-only", synthetic.Contributor.Name)
	}
	// No ingress unenforced synthetic — the policy governs ingress
	for _, rule := range nodeRules.Ingress.Allow {
		if rule.Coverage == models.CoverageUnenforced {
			t.Errorf("unexpected ingress Unenforced synthetic when policy governs ingress")
		}
	}
}

// No selecting policy = calico is entirely silent about the workload. Empty
// NodeRules and empty PolicyStatuses entries would inflate the intersect
// step (see policy.IntersectPolicyStatus filter). Skip the workload cleanly.
func TestResolveWorkload_NoSelectingPoliciesLeavesResultUntouched(t *testing.T) {
	nonSelecting := globalPolicy{
		name:            "matches-nothing",
		selectorMatch:   func(map[string]string) bool { return false },
		nsSelectorMatch: nil,
		ref:             models.PolicyRef{Source: sourceName, Name: "matches-nothing"},
	}
	result := newTestResult()

	(&source{}).resolveWorkload(testCache([]globalPolicy{nonSelecting}), testWorkload(), "ns-a", result)

	if _, ok := result.NodeRules["workload-1"]; ok {
		t.Error("expected NodeRules to have no entry when no policy selects workload")
	}
	if _, ok := result.PolicyStatuses["workload-1"]; ok {
		t.Error("expected PolicyStatuses to have no entry when no policy selects workload")
	}
	if _, ok := result.NodePolicies["workload-1"]; ok {
		t.Error("expected NodePolicies to have no entry when no policy selects workload")
	}
}

// Two selecting policies, each scoped to a different port on 0.0.0.0/0. Both
// must contribute — port dimension participates in bucketing so distinct
// (cidr, port) tuples resolve independently.
func TestResolveWorkload_PortScopedPoliciesBothContribute(t *testing.T) {
	port80 := models.Port{Port: 80, Protocol: "TCP"}
	port53 := models.Port{Port: 53, Protocol: "UDP"}
	policyHTTP := buildTestPolicy("allow-http", true, false,
		[]calicoRule{{action: models.ActionAllow, ports: []models.Port{port80}}},
		nil,
	)
	policyDNS := buildTestPolicy("allow-dns", true, false,
		[]calicoRule{{action: models.ActionAllow, ports: []models.Port{port53}}},
		nil,
	)
	result := newTestResult()

	(&source{}).resolveWorkload(testCache([]globalPolicy{policyHTTP, policyDNS}), testWorkload(), "ns-a", result)

	nodeRules := result.NodeRules["workload-1"]
	if len(nodeRules.Egress.Allow) != 2 {
		t.Fatalf("expected 2 egress allow rules (one per port), got %d", len(nodeRules.Egress.Allow))
	}
	contributors := map[string]models.Port{}
	for _, rule := range nodeRules.Egress.Allow {
		if len(rule.Ports) != 1 {
			t.Fatalf("expected exactly 1 Port stamped per rule, got %d", len(rule.Ports))
		}
		if rule.AllPorts {
			t.Errorf("port-scoped rule must not set AllPorts=true (contributor=%s)", rule.Contributor.Name)
		}
		if rule.Coverage != models.CoverageRestricted {
			t.Errorf("port-scoped catchAllCIDR rule must be Restricted, got %q", rule.Coverage)
		}
		contributors[rule.Contributor.Name] = rule.Ports[0]
	}
	if got := contributors["allow-http"]; got != port80 {
		t.Errorf("allow-http port: got %+v, want %+v", got, port80)
	}
	if got := contributors["allow-dns"]; got != port53 {
		t.Errorf("allow-dns port: got %+v, want %+v", got, port53)
	}
}

// Rule with empty ports = matches every port bucket. When a broad all-ports
// rule is first-match precedence, it also wins any port-scoped sibling bucket
// generated by other rules — emitting one AllPorts=true rule per bucket the
// broad rule covers. This is the honest model output; downstream renderer
// coalesces same-contributor entries.
func TestResolveWorkload_AllPortsRuleWinsSiblingPortBuckets(t *testing.T) {
	port80 := models.Port{Port: 80, Protocol: "TCP"}
	broad := buildTestPolicy("allow-all", true, false,
		[]calicoRule{{action: models.ActionAllow}}, // empty ports = all
		nil,
	)
	narrow := buildTestPolicy("allow-http", true, false,
		[]calicoRule{{action: models.ActionAllow, ports: []models.Port{port80}}},
		nil,
	)
	result := newTestResult()

	(&source{}).resolveWorkload(testCache([]globalPolicy{broad, narrow}), testWorkload(), "ns-a", result)

	nodeRules := result.NodeRules["workload-1"]
	if len(nodeRules.Egress.Allow) != 2 {
		t.Fatalf("expected 2 egress allow rules (broad rule wins both allPorts and port80 buckets), got %d", len(nodeRules.Egress.Allow))
	}
	for _, rule := range nodeRules.Egress.Allow {
		if rule.Contributor.Name != "allow-all" {
			t.Errorf("Contributor: got %q, want allow-all (narrow rule never wins any bucket)", rule.Contributor.Name)
		}
	}
}

// Evaluate against a cluster with no GlobalNetworkPolicies yields an empty-but-
// well-formed EvaluationResult (initialized maps, no entries). Engine is live
// (fetch + decode), so this drives the real getPolicies path with zero input.
func TestEvaluate_NoPoliciesYieldsEmptyResult(t *testing.T) {
	client, err := k8s.NewDemoClient(fstest.MapFS{}, ".")
	if err != nil {
		t.Fatalf("demo client: %v", err)
	}
	result, err := New(client, nil).Evaluate(context.Background(), []string{"ns-a"}, map[string]models.NSIndex{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NodeRules == nil || result.PolicyStatuses == nil || result.Nodes == nil {
		t.Fatal("expected initialized maps on empty result")
	}
	if len(result.NodeRules) != 0 {
		t.Errorf("expected empty NodeRules, got %d entries", len(result.NodeRules))
	}
}

// End-to-end from CRD structs: decode the two real policies (enroll + default-
// deny) through toGlobalPolicy — exercising the vendored selector parser on the
// `in {…}` namespaceSelector — then resolve. Enrolled ns → full egress (enroll
// wins), non-enrolled ns → east-west allow + DenyAll, internet closed.
func TestDecodeAndEvaluate_EnrollScenario(t *testing.T) {
	order500, order2000 := 500.0, 2000.0
	enroll := &calicov3.GlobalNetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "egress-enroll-namespaces"},
		Spec: calicov3.GlobalNetworkPolicySpec{
			Order:             &order500,
			NamespaceSelector: `kubernetes.io/metadata.name in {"observability","gitlab","frigate","security","flux-system","kube-system"}`,
			Selector:          "all()",
			Types:             []calicov3.PolicyType{calicov3.PolicyTypeEgress},
			Egress:            []calicov3.Rule{{Action: calicov3.Allow}},
		},
	}
	defaultDeny := &calicov3.GlobalNetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "egress-default-deny"},
		Spec: calicov3.GlobalNetworkPolicySpec{
			Order:    &order2000,
			Selector: "all()",
			Types:    []calicov3.PolicyType{calicov3.PolicyTypeEgress},
			Egress: []calicov3.Rule{
				{Action: calicov3.Allow, Destination: calicov3.EntityRule{Nets: []string{"10.42.0.0/16", "10.43.0.0/16", "192.168.8.0/24"}}},
				{Action: calicov3.Deny},
			},
		},
	}

	var policies []globalPolicy
	for _, gnp := range []*calicov3.GlobalNetworkPolicy{defaultDeny, enroll} {
		gp, err := toGlobalPolicy(gnp)
		if err != nil {
			t.Fatalf("decode %s: %v", gnp.Name, err)
		}
		policies = append(policies, gp)
	}
	sortByPrecedence(policies)

	web := models.WorkloadNode{ID: "web", Labels: map[string]string{"app": "web"}, Type: models.NodeTypeDeployment}

	// enrolled: real parser must match kube-system → enroll survives → full egress
	enrolledCache := buildNsSelection(policies, map[string]string{"kubernetes.io/metadata.name": "kube-system"})
	if len(enrolledCache.policies) != 2 {
		t.Fatalf("enrolled: expected both policies, got %d (parser mismatch on namespaceSelector?)", len(enrolledCache.policies))
	}
	enrolledResult := newTestResult()
	(&source{}).resolveWorkload(enrolledCache, web, "kube-system", enrolledResult)
	if findCoverage(enrolledResult.NodeRules["web"].Egress.Allow, models.CoverageAllowAll) == nil {
		t.Error("enrolled: expected CoverageAllowAll")
	}
	if !enrolledResult.PolicyStatuses["web"].InternetEgress {
		t.Error("enrolled: expected InternetEgress")
	}

	// non-enrolled: real parser must NOT match default → only default-deny governs
	plainCache := buildNsSelection(policies, map[string]string{"kubernetes.io/metadata.name": "default"})
	if len(plainCache.policies) != 1 {
		t.Fatalf("non-enrolled: expected only default-deny, got %d", len(plainCache.policies))
	}
	plainResult := newTestResult()
	(&source{}).resolveWorkload(plainCache, web, "default", plainResult)
	if findCoverage(plainResult.NodeRules["web"].Egress.Deny, models.CoverageDenyAll) == nil {
		t.Error("non-enrolled: expected CoverageDenyAll")
	}
	if plainResult.PolicyStatuses["web"].InternetEgress {
		t.Error("non-enrolled: internet must stay closed")
	}

	// Both policies use all() → every emitted rule must assert namespace-wide
	// scope, so the graph layer may collapse the per-workload fan-out.
	for _, rule := range plainResult.NodeRules["web"].Egress.Allow {
		if !rule.NamespaceWide {
			t.Errorf("all() selector must stamp NamespaceWide, got false on %+v", rule.Contributor)
		}
	}

	// A label selector that could match every pod present is NOT namespace-wide:
	// intent, not observed coverage. Same shape, selector swapped.
	labelled := defaultDeny.DeepCopy()
	labelled.Spec.Selector = "app == 'web'"
	labelledPolicy, err := toGlobalPolicy(labelled)
	if err != nil {
		t.Fatalf("decode labelled: %v", err)
	}
	if labelledPolicy.selectsAllWorkloads {
		t.Error("label selector must not be treated as namespace-wide")
	}
	labelledResult := newTestResult()
	(&source{}).resolveWorkload(buildNsSelection([]globalPolicy{labelledPolicy}, nil), web, "default", labelledResult)
	for _, rule := range labelledResult.NodeRules["web"].Egress.Deny {
		if rule.NamespaceWide {
			t.Errorf("label-selected rule wrongly claims namespace scope: %+v", rule.Contributor)
		}
	}
}

// findCoverage returns the first rule with the given coverage, or nil.
func findCoverage(rules []models.Rule, coverage models.Coverage) *models.Rule {
	for index := range rules {
		if rules[index].Coverage == coverage {
			return &rules[index]
		}
	}
	return nil
}

// enrollScenarioPolicies builds the two-policy egress model from the audit: a
// low-order namespace-enrollment Allow-all (order 500) and a high-order
// default-deny (order 2000; allow pod/svc/LAN east-west, deny the rest).
// Returned pre-sorted by precedence, as Evaluate does. Selector closures are
// injected directly — real libcalico parsing is orthogonal to resolution.
func enrollScenarioPolicies() []globalPolicy {
	enrolled := map[string]bool{
		"observability": true, "gitlab": true, "frigate": true,
		"security": true, "flux-system": true, "kube-system": true,
	}
	enroll := globalPolicy{
		name:          "egress-enroll-namespaces",
		order:         500,
		tier:          "default",
		doesEgress:    true,
		egress:        []calicoRule{{action: models.ActionAllow}}, // catch-all allow
		selectorMatch: func(map[string]string) bool { return true },
		nsSelectorMatch: func(labels map[string]string) bool {
			return enrolled[labels["kubernetes.io/metadata.name"]]
		},
		ref: models.PolicyRef{Source: sourceName, Name: "egress-enroll-namespaces"},
	}
	defaultDeny := globalPolicy{
		name:       "egress-default-deny",
		order:      2000,
		tier:       "default",
		doesEgress: true,
		egress: []calicoRule{
			{action: models.ActionAllow, nets: []string{"10.42.0.0/16", "10.43.0.0/16", "192.168.8.0/24"}},
			{action: models.ActionDeny}, // catch-all deny
		},
		selectorMatch:   func(map[string]string) bool { return true },
		nsSelectorMatch: nil, // absent namespaceSelector = all namespaces
		ref:             models.PolicyRef{Source: sourceName, Name: "egress-default-deny"},
	}
	policies := []globalPolicy{defaultDeny, enroll} // unsorted on purpose
	sortByPrecedence(policies)
	return policies
}

// Enrolled namespace: both policies survive nsSelector, enroll's order-500
// catch-all Allow beats default-deny's order-2000 Deny → full egress. Namespace
// node mirrors the all()-selected verdict.
func TestEnrollScenario_EnrolledNamespaceFullEgress(t *testing.T) {
	policies := enrollScenarioPolicies()
	cache := buildNsSelection(policies, map[string]string{"kubernetes.io/metadata.name": "kube-system"})
	if len(cache.policies) != 2 {
		t.Fatalf("enrolled ns: expected both policies to survive nsSelector, got %d", len(cache.policies))
	}

	result := newTestResult()
	web := models.WorkloadNode{ID: "web", Namespace: "kube-system", Labels: map[string]string{"app": "web"}, Type: models.NodeTypeDeployment}
	(&source{}).resolveWorkload(cache, web, "kube-system", result)

	rules := result.NodeRules["web"]
	allowAll := findCoverage(rules.Egress.Allow, models.CoverageAllowAll)
	if allowAll == nil {
		t.Fatal("enrolled workload: expected a CoverageAllowAll egress rule")
	}
	if allowAll.Contributor.Name != "egress-enroll-namespaces" {
		t.Errorf("AllowAll contributor: got %q, want egress-enroll-namespaces (order 500 wins)", allowAll.Contributor.Name)
	}
	if findCoverage(rules.Egress.Deny, models.CoverageDenyAll) != nil {
		t.Error("enrolled workload must not carry a DenyAll — enroll allows everything")
	}
	status := result.PolicyStatuses["web"]
	if !status.EgressLocked {
		t.Error("expected EgressLocked")
	}
	if !status.InternetEgress {
		t.Error("enrolled workload should reach internet (0.0.0.0/0 allowed)")
	}

	// namespace node: all() selects it too → same full-egress verdict
	nsResult := newTestResult()
	nsNode := models.WorkloadNode{ID: "ns:kube-system", Type: models.NodeTypeNamespace}
	(&source{}).resolveWorkload(cache, nsNode, "kube-system", nsResult)
	if findCoverage(nsResult.NodeRules["ns:kube-system"].Egress.Allow, models.CoverageAllowAll) == nil {
		t.Error("namespace node: expected CoverageAllowAll parity with pods")
	}
}

// Non-enrolled namespace: enroll filtered by nsSelector, only default-deny
// governs → east-west allowed, everything external denied. The InternetEgress=
// false assertion is the exfil-channel-closed guarantee (config-independent:
// only a literal 0.0.0.0/0 allow flips it).
func TestEnrollScenario_NonEnrolledNamespaceEastWestOnly(t *testing.T) {
	policies := enrollScenarioPolicies()
	cache := buildNsSelection(policies, map[string]string{"kubernetes.io/metadata.name": "default"})
	if len(cache.policies) != 1 {
		t.Fatalf("non-enrolled ns: expected only default-deny to survive, got %d", len(cache.policies))
	}

	result := newTestResult()
	web := models.WorkloadNode{ID: "web", Namespace: "default", Labels: map[string]string{"app": "web"}, Type: models.NodeTypeDeployment}
	(&source{}).resolveWorkload(cache, web, "default", result)

	rules := result.NodeRules["web"]
	wantCIDRs := map[string]bool{
		models.CIDRIDPrefix + "10.42.0.0/16":   false,
		models.CIDRIDPrefix + "10.43.0.0/16":   false,
		models.CIDRIDPrefix + "192.168.8.0/24": false,
	}
	for _, rule := range rules.Egress.Allow {
		if rule.Coverage != models.CoverageRestricted {
			continue
		}
		if _, ok := wantCIDRs[rule.DstID]; ok {
			wantCIDRs[rule.DstID] = true
		}
		if _, ok := result.Nodes[rule.DstID]; !ok {
			t.Errorf("allow to %s but no CIDR node emitted", rule.DstID)
		}
	}
	for cidr, seen := range wantCIDRs {
		if !seen {
			t.Errorf("expected east-west allow edge to %s", cidr)
		}
	}

	denyAll := findCoverage(rules.Egress.Deny, models.CoverageDenyAll)
	if denyAll == nil {
		t.Fatal("non-enrolled workload: expected CoverageDenyAll for external egress")
	}
	if denyAll.Contributor.Name != "egress-default-deny" {
		t.Errorf("DenyAll contributor: got %q, want egress-default-deny", denyAll.Contributor.Name)
	}
	if denyAll.SrcID != "web" || denyAll.DstID != "" {
		t.Errorf("DenyAll blanket endpoint: got SrcID=%q DstID=%q, want SrcID=web DstID empty", denyAll.SrcID, denyAll.DstID)
	}

	status := result.PolicyStatuses["web"]
	if status.InternetEgress {
		t.Error("non-enrolled workload must NOT reach internet — exfil channel must stay closed")
	}
	if !status.EgressLocked {
		t.Error("expected EgressLocked")
	}
}

// checkPeerSupported is the guard against label-scoped peers silently
// broadening to a catch-all CIDR bucket. Regression here = false allow edges
// to every CIDR — the exact "quietly lies" failure this project must not ship.
// Each case builds a real calicov3.Rule and pushes it through toGlobalPolicy
// so the whole decode path (including isCatchAllSelector) participates.
func TestToGlobalPolicy_RejectsUnsupportedPeers(t *testing.T) {
	port80 := numorstring.SinglePort(80)

	cases := []struct {
		name    string
		peer    calicov3.EntityRule
		wantSub string // substring the returned error must contain
	}{
		{
			name:    "label selector",
			peer:    calicov3.EntityRule{Selector: "role == 'db'"},
			wantSub: "selector",
		},
		{
			name:    "notSelector",
			peer:    calicov3.EntityRule{NotSelector: "env == 'prod'"},
			wantSub: "notSelector",
		},
		{
			name:    "namespaceSelector",
			peer:    calicov3.EntityRule{NamespaceSelector: "app == 'ns'"},
			wantSub: "namespaceSelector",
		},
		{
			name:    "notPorts",
			peer:    calicov3.EntityRule{NotPorts: []numorstring.Port{port80}},
			wantSub: "notPorts",
		},
	}

	for _, tc := range cases {
		t.Run("egress/"+tc.name, func(t *testing.T) {
			gnp := &calicov3.GlobalNetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "reject-" + tc.name},
				Spec: calicov3.GlobalNetworkPolicySpec{
					Selector: "all()",
					Types:    []calicov3.PolicyType{calicov3.PolicyTypeEgress},
					Egress:   []calicov3.Rule{{Action: calicov3.Allow, Destination: tc.peer}},
				},
			}
			_, err := toGlobalPolicy(gnp)
			if err == nil {
				t.Fatalf("expected error for egress peer %q; got nil (peer silently broadened to catch-all)", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("egress %q: expected error to mention %q; got %q", tc.name, tc.wantSub, err.Error())
			}
		})

		t.Run("ingress/"+tc.name, func(t *testing.T) {
			// Ingress rule → checkPeerSupported walks rule.Source, not rule.Destination.
			// Guard must reject on either side.
			gnp := &calicov3.GlobalNetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "reject-ingress-" + tc.name},
				Spec: calicov3.GlobalNetworkPolicySpec{
					Selector: "all()",
					Types:    []calicov3.PolicyType{calicov3.PolicyTypeIngress},
					Ingress:  []calicov3.Rule{{Action: calicov3.Allow, Source: tc.peer}},
				},
			}
			_, err := toGlobalPolicy(gnp)
			if err == nil {
				t.Fatalf("expected error for ingress peer %q; got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("ingress %q: expected error to mention %q; got %q", tc.name, tc.wantSub, err.Error())
			}
		})
	}
}
