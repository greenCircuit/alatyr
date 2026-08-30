package istio

import (
	"reflect"
	"testing"

	"graph/internal/models"
	"graph/internal/utils"

	istioapi "istio.io/api/security/v1beta1"
	istioapitype "istio.io/api/type/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// buildFixtureNSIndex mirrors graph.buildWorkloadIndex without a graph→istio
// cycle. Picks up the namespace node from the fixture if present.
func buildFixtureNSIndex(nodes []models.WorkloadNode) models.NSIndex {
	labelIndex := map[string][]*models.WorkloadNode{}
	var nsNode *models.WorkloadNode
	for position := range nodes {
		node := &nodes[position]
		if node.Type == models.NodeTypeNamespace {
			nsNode = node
		}
		for key, value := range node.Labels {
			entry := utils.MakeLabelIndexKey(key, value)
			labelIndex[entry] = append(labelIndex[entry], node)
		}
	}
	return models.NSIndex{
		Workloads:  nodes,
		LabelIndex: labelIndex,
		NSNode:     nsNode,
	}
}

func makePolicy(name, namespace string, action istioapi.AuthorizationPolicy_Action, selector map[string]string, rules []*istioapi.Rule) *istiosec.AuthorizationPolicy {
	var workloadSelector *istioapitype.WorkloadSelector
	if selector != nil {
		workloadSelector = &istioapitype.WorkloadSelector{MatchLabels: selector}
	}
	return &istiosec.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: istioapi.AuthorizationPolicy{
			Selector: workloadSelector,
			Action:   action,
			Rules:    rules,
		},
	}
}

// buildPolicyStatus

// Covers: ALLOW from own namespace with L7 to-operation. Verifies the
// short-circuit-free accumulator path sets InnerNsIngress + IngressLocked
// AND that L7 detection runs inside the same accumulator pass.
func TestBuildPolicyStatus_AllowNsWithL7(t *testing.T) {
	authzPolicy := makePolicy("allow-l7", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-a"}}}},
			To:   []*istioapi.Rule_To{{Operation: &istioapi.Operation{Methods: []string{"GET"}, Paths: []string{"/api/*"}}}},
		}},
	)

	status := buildPolicyStatus([]*istiosec.AuthorizationPolicy{authzPolicy})

	if !status.IngressLocked {
		t.Errorf("IngressLocked = false, want true")
	}
	if !status.InnerNsIngress {
		t.Errorf("InnerNsIngress = false, want true")
	}
	if status.CrossNS {
		t.Errorf("CrossNS = true, want false (source is own ns)")
	}
	if !status.HasL7 {
		t.Errorf("HasL7 = false, want true (L7 matchers present)")
	}
}

// Covers: deny short-circuit exits early but still propagates HasL7 if the
// same DENY policy carried L7 matchers on the way to the wildcard rule.
func TestBuildPolicyStatus_DenyAllShortCircuitKeepsL7(t *testing.T) {
	authzPolicy := makePolicy("deny-all", "ns-a", istioapi.AuthorizationPolicy_DENY,
		nil,
		[]*istioapi.Rule{
			{To: []*istioapi.Rule_To{{Operation: &istioapi.Operation{Paths: []string{"/admin"}}}}}, // L7 rule processed first
			{From: nil},                                                                            // wildcard source → short-circuit fires
		},
	)

	status := buildPolicyStatus([]*istiosec.AuthorizationPolicy{authzPolicy})

	if !status.IngressLocked {
		t.Errorf("IngressLocked = false, want true (deny short-circuit)")
	}
	if !status.HasL7 {
		t.Errorf("HasL7 = false, want true (L7 detected in same loop as short-circuit)")
	}
}

// Covers: cross-ns ingress signal accumulation without L7 — pure L3 path.
// Also asserts HasL7 stays false when only L4 fields are present.
func TestBuildPolicyStatus_CrossNsNoL7(t *testing.T) {
	authzPolicy := makePolicy("allow-cross", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-b"}}}},
			To:   []*istioapi.Rule_To{{Operation: &istioapi.Operation{Ports: []string{"8080"}}}}, // ports only — L4
		}},
	)

	status := buildPolicyStatus([]*istiosec.AuthorizationPolicy{authzPolicy})

	if !status.CrossNS {
		t.Errorf("CrossNS = false, want true")
	}
	if status.InnerNsIngress {
		t.Errorf("InnerNsIngress = true, want false (source is different ns)")
	}
	if status.HasL7 {
		t.Errorf("HasL7 = true, want false (ports only is L4)")
	}
}

// buildRules

// Covers: rule index threading + L7 attachment to per-port rules. A single
// policy with two rules (rule 0 = L4 ports, rule 1 = L7 paths) should
// produce models.Rule entries with the right RuleIndex and L7Match populated
// only on rule-1's outputs.
func TestBuildRules_RuleIndexAndL7Attachment(t *testing.T) {
	authzPolicy := makePolicy("two-rules", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{
			{
				From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-a"}}}},
				To:   []*istioapi.Rule_To{{Operation: &istioapi.Operation{Ports: []string{"8080"}}}},
			},
			{
				From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-a"}}}},
				To:   []*istioapi.Rule_To{{Operation: &istioapi.Operation{Paths: []string{"/v1"}}}},
			},
		},
	)

	target := models.WorkloadNode{ID: "api-wid", Labels: map[string]string{"app": "api"}, Namespace: "ns-a"}
	nsNode := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace, Namespace: "ns-a", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-a"}}
	index := map[string]models.NSIndex{"ns-a": buildFixtureNSIndex([]models.WorkloadNode{target, nsNode})}

	rulesByNs := buildRulesByNs(index, map[string][]*istiosec.AuthorizationPolicy{"ns-a": {authzPolicy}}, map[string]*models.NodeRules{})
	var rules []models.Rule
	for _, nsRules := range rulesByNs {
		rules = append(rules, nsRules...)
	}

	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}

	// Split by L7-ness rather than by source rule ordinal — the L4 block and
	// the L7 block are the two distinct outputs the expansion must produce.
	var l4Rules, l7Rules []models.Rule
	for _, rule := range rules {
		if rule.Contributor.Name == "" {
			t.Fatalf("rule missing contributor: %+v", rule)
		}
		if rule.L7Match == nil {
			l4Rules = append(l4Rules, rule)
			continue
		}
		l7Rules = append(l7Rules, rule)
	}

	if len(l4Rules) != 1 || len(l7Rules) != 1 {
		t.Fatalf("got %d L4 / %d L7 rules, want 1 each: %+v", len(l4Rules), len(l7Rules), rules)
	}
	if len(l4Rules[0].Ports) != 1 || l4Rules[0].Ports[0].Port != 8080 {
		t.Errorf("L4 rule Ports = %+v, want [{Port: 8080}]", l4Rules[0].Ports)
	}
	if !reflect.DeepEqual(l7Rules[0].L7Match.Paths, []string{"/v1"}) {
		t.Errorf("L7 rule L7Match = %+v, want Paths=[/v1]", l7Rules[0].L7Match)
	}
}

// Demonstrates the multi-to[] last-wins bug at buildRules.go:66-70:
// rulePorts/ruleL7 are reassigned on each to[] iteration, so only the LAST
// to[] block survives. Fails today; passes once expandRules emits one Rule
// per to[] block (per the allowance-list design — one source-spec block per
// Rule, L7Match stays scalar pointer).
func TestExpandRules_MultipleToBlocksPreserved(t *testing.T) {
	authzPolicy := makePolicy("multi-to", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-a"}}}},
			To: []*istioapi.Rule_To{
				{Operation: &istioapi.Operation{Ports: []string{"80"}, Hosts: []string{"api.example.com"}}},
				{Operation: &istioapi.Operation{Ports: []string{"443"}, Hosts: []string{"admin.example.com"}}},
			},
		}},
	)
	target := models.WorkloadNode{ID: "api-wid", Labels: map[string]string{"app": "api"}, Namespace: "ns-a"}
	nsNode := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace, Namespace: "ns-a", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-a"}}
	index := map[string]models.NSIndex{"ns-a": buildFixtureNSIndex([]models.WorkloadNode{target, nsNode})}

	rulesByNs := buildRulesByNs(index, map[string][]*istiosec.AuthorizationPolicy{"ns-a": {authzPolicy}}, map[string]*models.NodeRules{})
	var rules []models.Rule
	for _, nsRules := range rulesByNs {
		rules = append(rules, nsRules...)
	}

	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 (one per to[] block)", len(rules))
	}

	// Index by port so the assertion isn't ordering-dependent and the
	// failure message is clear when the bug fires (only one survives).
	byPort := map[int]models.Rule{}
	for _, rule := range rules {
		if len(rule.Ports) != 1 {
			t.Fatalf("rule has %d ports, want exactly 1: %+v", len(rule.Ports), rule)
		}
		byPort[rule.Ports[0].Port] = rule
	}

	port80, has80 := byPort[80]
	if !has80 {
		t.Fatalf("missing rule for port 80 (multi-to[] last-wins bug drops the first block)")
	}
	if port80.L7Match == nil || !reflect.DeepEqual(port80.L7Match.Hosts, []string{"api.example.com"}) {
		t.Errorf("port 80 rule L7Match = %+v, want Hosts=[api.example.com] paired with its own to[] block", port80.L7Match)
	}

	port443, has443 := byPort[443]
	if !has443 {
		t.Fatalf("missing rule for port 443")
	}
	if port443.L7Match == nil || !reflect.DeepEqual(port443.L7Match.Hosts, []string{"admin.example.com"}) {
		t.Errorf("port 443 rule L7Match = %+v, want Hosts=[admin.example.com] paired with its own to[] block", port443.L7Match)
	}

	// Pairing guard: with the bug, both rules would carry the LAST block's
	// L7 (admin) attached to BOTH ports — the matrix lie. Explicit check so
	// a future regression that "merges" L7s independently can't pass.
	if port80.L7Match != nil && port443.L7Match != nil &&
		reflect.DeepEqual(port80.L7Match.Hosts, port443.L7Match.Hosts) {
		t.Errorf("both rules share the same L7 hosts %+v — pairing lost", port80.L7Match.Hosts)
	}

	for _, rule := range rules {
		if rule.SrcID != "ns-ns-a" || rule.DstID != "api-wid" {
			t.Errorf("rule SrcID/DstID = %s/%s, want ns-ns-a/api-wid", rule.SrcID, rule.DstID)
		}
	}
}

// Demonstrates the multi-from[] last-wins bug at buildRules.go:63-65:
// matchWorkloads is reassigned on each from[] iteration, so only the LAST
// from[] block's sources survive. Fails today; passes once expandRules
// accumulates sources across from[] blocks (they're OR'd per Istio spec).
func TestExpandRules_MultipleFromBlocksPreserved(t *testing.T) {
	authzPolicy := makePolicy("multi-from", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{
				{Source: &istioapi.Source{Namespaces: []string{"ns-a"}}},
				{Source: &istioapi.Source{Namespaces: []string{"ns-b"}}},
			},
			To: []*istioapi.Rule_To{{Operation: &istioapi.Operation{Ports: []string{"80"}}}},
		}},
	)
	target := models.WorkloadNode{ID: "api-wid", Labels: map[string]string{"app": "api"}, Namespace: "ns-a"}
	nsAObj := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace, Namespace: "ns-a", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-a"}}
	nsBObj := models.WorkloadNode{ID: "ns-ns-b", Type: models.NodeTypeNamespace, Namespace: "ns-b", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-b"}}
	index := map[string]models.NSIndex{
		"ns-a": buildFixtureNSIndex([]models.WorkloadNode{target, nsAObj}),
		"ns-b": buildFixtureNSIndex([]models.WorkloadNode{nsBObj}),
	}

	rulesByNs := buildRulesByNs(index, map[string][]*istiosec.AuthorizationPolicy{"ns-a": {authzPolicy}}, map[string]*models.NodeRules{})
	var rules []models.Rule
	for _, nsRules := range rulesByNs {
		rules = append(rules, nsRules...)
	}

	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 (one per from[] source)", len(rules))
	}

	bySrc := map[string]models.Rule{}
	for _, rule := range rules {
		bySrc[rule.SrcID] = rule
	}

	if _, ok := bySrc["ns-ns-a"]; !ok {
		t.Errorf("missing rule sourced from ns-ns-a (multi-from[] last-wins bug drops the first block)")
	}
	if _, ok := bySrc["ns-ns-b"]; !ok {
		t.Errorf("missing rule sourced from ns-ns-b")
	}

	for srcID, rule := range bySrc {
		if rule.DstID != "api-wid" {
			t.Errorf("rule from %s has DstID=%s, want api-wid", srcID, rule.DstID)
		}
		if len(rule.Ports) != 1 || rule.Ports[0].Port != 80 {
			t.Errorf("rule from %s has Ports=%+v, want [{Port:80}] (every from[] source pairs with the single to[] block)", srcID, rule.Ports)
		}
	}
}

// Demonstrates that a rule with from[] but no to[] block emits zero rules
// today. Every "DENY from ns-X" policy silently produces no edges. Fails
// today; passes once expandRules handles len(rule.To) == 0 by emitting one
// Rule per workload with AllPorts=true, AllL7=true.
func TestExpandRules_NoToBlockStillEmitsRule(t *testing.T) {
	authzPolicy := makePolicy("from-only", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-a"}}}},
		}},
	)
	target := models.WorkloadNode{ID: "api-wid", Labels: map[string]string{"app": "api"}, Namespace: "ns-a"}
	nsNode := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace, Namespace: "ns-a", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-a"}}
	index := map[string]models.NSIndex{"ns-a": buildFixtureNSIndex([]models.WorkloadNode{target, nsNode})}

	rulesByNs := buildRulesByNs(index, map[string][]*istiosec.AuthorizationPolicy{"ns-a": {authzPolicy}}, map[string]*models.NodeRules{})
	var rules []models.Rule
	for _, nsRules := range rulesByNs {
		rules = append(rules, nsRules...)
	}

	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1 (no-to[] should emit one unrestricted Rule per workload)", len(rules))
	}
	if !rules[0].AllPorts || !rules[0].AllL7 {
		t.Errorf("AllPorts=%v AllL7=%v, want both true (no to[] → unrestricted on both axes)", rules[0].AllPorts, rules[0].AllL7)
	}
}

// Demonstrates that AllL7 is not set on Rules emitted from to-blocks that
// have ports but no L7 predicates. L7Match comes out nil (correct) but
// AllL7 stays false (zero-value) — violates the invariant "L7Match == nil
// ⇒ AllL7 == true." Frontend would read this as "L7 restricted" when
// reality is unrestricted. Fails today; passes once both to-block Rule
// emits set AllL7: ruleL7.IsEmpty().
func TestExpandRules_AllL7SetWhenNoL7Predicates(t *testing.T) {
	authzPolicy := makePolicy("ports-only", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-a"}}}},
			To:   []*istioapi.Rule_To{{Operation: &istioapi.Operation{Ports: []string{"80"}}}}, // ports, no L7
		}},
	)
	target := models.WorkloadNode{ID: "api-wid", Labels: map[string]string{"app": "api"}, Namespace: "ns-a"}
	nsNode := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace, Namespace: "ns-a", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-a"}}
	index := map[string]models.NSIndex{"ns-a": buildFixtureNSIndex([]models.WorkloadNode{target, nsNode})}

	rulesByNs := buildRulesByNs(index, map[string][]*istiosec.AuthorizationPolicy{"ns-a": {authzPolicy}}, map[string]*models.NodeRules{})
	var rules []models.Rule
	for _, nsRules := range rulesByNs {
		rules = append(rules, nsRules...)
	}

	if len(rules) == 0 {
		t.Fatal("got 0 rules, want at least 1")
	}
	for _, rule := range rules {
		if rule.L7Match != nil {
			t.Errorf("rule L7Match = %+v, want nil (operation had no L7 fields)", rule.L7Match)
		}
		if !rule.AllL7 {
			t.Errorf("rule AllL7 = false, want true (no L7 predicates → no L7 restriction)")
		}
	}
}

// Demonstrates that authzPolicy.Spec.Action is never read into the emitted
// Rules — DENY policies produce Rules with Action=ActionAllow (zero value),
// silently rendering as green ALLOW edges. Fails today; passes once
// expandRules stamps Action from spec.Action on every emit.
func TestExpandRules_DenyActionPropagated(t *testing.T) {
	authzPolicy := makePolicy("deny-from-b", "ns-a", istioapi.AuthorizationPolicy_DENY,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-b"}}}},
			To:   []*istioapi.Rule_To{{Operation: &istioapi.Operation{Ports: []string{"80"}}}},
		}},
	)
	target := models.WorkloadNode{ID: "api-wid", Labels: map[string]string{"app": "api"}, Namespace: "ns-a"}
	nsAObj := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace, Namespace: "ns-a", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-a"}}
	nsBObj := models.WorkloadNode{ID: "ns-ns-b", Type: models.NodeTypeNamespace, Namespace: "ns-b", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-b"}}
	index := map[string]models.NSIndex{
		"ns-a": buildFixtureNSIndex([]models.WorkloadNode{target, nsAObj}),
		"ns-b": buildFixtureNSIndex([]models.WorkloadNode{nsBObj}),
	}

	rulesByNs := buildRulesByNs(index, map[string][]*istiosec.AuthorizationPolicy{"ns-a": {authzPolicy}}, map[string]*models.NodeRules{})
	var rules []models.Rule
	for _, nsRules := range rulesByNs {
		rules = append(rules, nsRules...)
	}

	if len(rules) == 0 {
		t.Fatal("got 0 rules, want at least 1")
	}
	for _, rule := range rules {
		if rule.Action != models.ActionDeny {
			t.Errorf("rule Action = %d, want %d (ActionDeny — spec.Action not propagated)", rule.Action, models.ActionDeny)
		}
	}
}

// expandToOperation

// Covers: every L7 dimension (Hosts/Methods/Paths) AND Ports round-trip
// through expandToOperation. Guards against a prior copy-paste bug where
// the Methods accumulator was appending to l7Match.Hosts instead of Methods,
// which TestBuildRules' L4+L7 split couldn't catch (no methods exercised).
// Also confirms an empty Operation yields an empty L7Match (IsEmpty true).
func TestExpandToOperation_AllL7FieldsAndPorts(t *testing.T) {
	operation := &istioapi.Operation{
		Ports:   []string{"8080", "9090"},
		Hosts:   []string{"example.com", "api.example.com"},
		Methods: []string{"GET", "POST"},
		Paths:   []string{"/v1", "/v2"},
	}

	ports, l7 := expandToOperation(operation)

	wantPorts := []models.Port{
		{Port: 8080, Protocol: "TCP"},
		{Port: 9090, Protocol: "TCP"},
	}
	if !reflect.DeepEqual(ports, wantPorts) {
		t.Errorf("ports = %+v, want %+v", ports, wantPorts)
	}
	if !reflect.DeepEqual(l7.Hosts, []string{"example.com", "api.example.com"}) {
		t.Errorf("Hosts = %+v, want [example.com api.example.com]", l7.Hosts)
	}
	if !reflect.DeepEqual(l7.Methods, []string{"GET", "POST"}) {
		t.Errorf("Methods = %+v, want [GET POST] (regression guard for Hosts-into-Methods bug)", l7.Methods)
	}
	if !reflect.DeepEqual(l7.Paths, []string{"/v1", "/v2"}) {
		t.Errorf("Paths = %+v, want [/v1 /v2]", l7.Paths)
	}

	// Empty Operation → IsEmpty true so callers nil out the L7Match pointer.
	_, emptyL7 := expandToOperation(&istioapi.Operation{})
	if !emptyL7.IsEmpty() {
		t.Errorf("empty operation produced non-empty L7Match: %+v", emptyL7)
	}
}

// getSourceNodes

// Covers: all three selector branches in one fixture — catch-all (nil
// selector) returns every workload, label-match returns only matching
// workloads, and label-miss returns nothing. The fourth case (empty
// MatchLabels, treated as catch-all per isCatchAllSelector) shares its
// path with nil, so isn't called out separately.
func TestGetSourceNodes_SelectorBranches(t *testing.T) {
	api := models.WorkloadNode{ID: "api", Namespace: "ns-a", Labels: map[string]string{"app": "api"}}
	web := models.WorkloadNode{ID: "web", Namespace: "ns-a", Labels: map[string]string{"app": "web"}}
	nsNode := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace, Namespace: "ns-a"}
	index := map[string]models.NSIndex{"ns-a": buildFixtureNSIndex([]models.WorkloadNode{api, web, nsNode})}

	catchAll := makePolicy("p1", "ns-a", istioapi.AuthorizationPolicy_ALLOW, nil, nil)
	specific := makePolicy("p2", "ns-a", istioapi.AuthorizationPolicy_ALLOW, map[string]string{"app": "api"}, nil)
	miss := makePolicy("p3", "ns-a", istioapi.AuthorizationPolicy_ALLOW, map[string]string{"app": "nonexistent"}, nil)

	if got := getSourceNodes(catchAll, index); len(got) != 3 {
		t.Errorf("catch-all selector picked %d nodes, want 3 (all workloads + ns-node)", len(got))
	}
	specifics := getSourceNodes(specific, index)
	if len(specifics) != 1 || specifics[0].ID != "api" {
		t.Errorf("label-match selector = %+v, want [api]", specifics)
	}
	if got := getSourceNodes(miss, index); len(got) != 0 {
		t.Errorf("non-matching selector returned %d nodes, want 0", len(got))
	}
}

// getNodePolicies

// Covers: catch-all policy applies to BOTH workload nodes and the
// namespace node; specific-selector policy applies only to matching
// workload and skips the namespace node entirely (mirrors k8s engine
// rule from CLAUDE.md — ns-node only matches catch-all).
func TestGetNodePolicies_NsNodeOnlyMatchesCatchAll(t *testing.T) {
	api := models.WorkloadNode{ID: "api", Labels: map[string]string{"app": "api"}}
	nsNode := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace}

	catchAll := makePolicy("ca", "ns-a", istioapi.AuthorizationPolicy_ALLOW, nil, nil)
	specific := makePolicy("sp", "ns-a", istioapi.AuthorizationPolicy_ALLOW, map[string]string{"app": "api"}, nil)
	candidates := []*istiosec.AuthorizationPolicy{catchAll, specific}

	workloadMatches := getNodePolicies(api, candidates)
	if len(workloadMatches) != 2 {
		t.Errorf("workload matched %d policies, want 2 (catch-all + specific)", len(workloadMatches))
	}

	nsMatches := getNodePolicies(nsNode, candidates)
	if len(nsMatches) != 1 || nsMatches[0].Name != "ca" {
		t.Errorf("ns-node matched %+v, want [ca] only (specific selector must skip ns-node)", nsMatches)
	}
}

// buildPolicyStatus — CIDR + subtraction + wildcard

// Covers: CIDR classification populates both InternetIngress AND LanIngress
// when the policy lists CIDRs in both ranges. Same fixture exercises the
// status accumulator path (no short-circuit), confirming a single rule
// can light up multiple ingress dimensions.
func TestBuildPolicyStatus_CIDRDualClassification(t *testing.T) {
	authzPolicy := makePolicy("cidr", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{{Source: &istioapi.Source{
				IpBlocks: []string{"10.0.0.0/8", "0.0.0.0/0"}, // LAN + internet
			}}},
		}},
	)

	status := buildPolicyStatus([]*istiosec.AuthorizationPolicy{authzPolicy})

	if !status.IngressLocked {
		t.Error("IngressLocked = false, want true (any selecting policy locks ingress)")
	}
	if !status.LanIngress {
		t.Error("LanIngress = false, want true (10.0.0.0/8 is private)")
	}
	if !status.InternetIngress {
		t.Error("InternetIngress = false, want true (0.0.0.0/0 is internet)")
	}
}

// Covers: combinePolicySignals subtraction — when an ALLOW grants ns-b and
// a DENY blocks ns-b in the same policy set, the resulting status must
// NOT flag CrossNS. Pairs with the notNamespaces over-claim path: a wildcard
// notNamespaces entry flips InnerNsIngress + CrossNS regardless of explicit
// namespace entries. Two assertions to keep one rich fixture instead of two
// thin tests.
func TestBuildPolicyStatus_SubtractionAndWildcardOverclaim(t *testing.T) {
	// Phase 1: allow ns-b, deny ns-b → ns-b removed from survivors → no CrossNS.
	allowNsB := makePolicy("allow-b", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-b"}}}}}},
	)
	denyNsB := makePolicy("deny-b", "ns-a", istioapi.AuthorizationPolicy_DENY,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-b"}}}}}},
	)

	subtractedStatus := buildPolicyStatus([]*istiosec.AuthorizationPolicy{allowNsB, denyNsB})
	if subtractedStatus.CrossNS {
		t.Error("CrossNS = true, want false (allow ns-b - deny ns-b = no survivor)")
	}
	if !subtractedStatus.IngressLocked {
		t.Error("IngressLocked = false, want true (policies still select the workload)")
	}

	// Phase 2: notNamespaces wildcard alone → over-claim BOTH inner-ns + cross-ns.
	wildcardPolicy := makePolicy("not-ns", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{From: []*istioapi.Rule_From{{Source: &istioapi.Source{NotNamespaces: []string{"ns-evil"}}}}}},
	)

	wildcardStatus := buildPolicyStatus([]*istiosec.AuthorizationPolicy{wildcardPolicy})
	if !wildcardStatus.InnerNsIngress {
		t.Error("InnerNsIngress = false, want true (notNamespaces over-claims own ns)")
	}
	if !wildcardStatus.CrossNS {
		t.Error("CrossNS = false, want true (notNamespaces over-claims cross-ns)")
	}
}

// splitPoliciesByAction

// Covers: action enum partitioning, including dropping AUDIT and CUSTOM
// which don't shape L3 edges.
func TestSplitPoliciesByAction_AllActionTypes(t *testing.T) {
	allow := makePolicy("a", "ns", istioapi.AuthorizationPolicy_ALLOW, nil, nil)
	deny := makePolicy("d", "ns", istioapi.AuthorizationPolicy_DENY, nil, nil)
	audit := makePolicy("au", "ns", istioapi.AuthorizationPolicy_AUDIT, nil, nil)
	custom := makePolicy("c", "ns", istioapi.AuthorizationPolicy_CUSTOM, nil, nil)

	allowOut, denyOut := splitPoliciesByAction(map[string][]*istiosec.AuthorizationPolicy{
		"ns": {allow, deny, audit, custom},
	})

	if len(allowOut["ns"]) != 1 || allowOut["ns"][0].Name != "a" {
		t.Errorf("allow bucket = %+v, want [a]", allowOut["ns"])
	}
	if len(denyOut["ns"]) != 1 || denyOut["ns"][0].Name != "d" {
		t.Errorf("deny bucket = %+v, want [d]", denyOut["ns"])
	}
}
