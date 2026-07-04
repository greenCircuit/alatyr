package istio

import (
	"reflect"
	"testing"

	"graph/internal/models"

	istioapi "istio.io/api/security/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

// Coverage classification tests. Assert desired semantics from
// docs/backlog/policy-coverage-classification.md § "Test cases to add".
// Several cases are TDD-style — they fail against today's expandRules
// (the doc calls them "the regression the feature fixes"). Each such
// test has an inline note explaining what wiring will make it pass.
//
// Ingress-only. Semantics under test:
//   deny-all ALLOW = action:ALLOW + rules:[]       — matches nothing → allows nothing
//   deny-all DENY  = action:DENY  + rules:[{}]     — catch-all → denies everything
//   unenforced DENY= action:DENY  + rules:[]       — matches nothing → no-op
//   allow-all      = rule with no from             — any source, ALLOW
//   allow-all-ns   = from.source.namespaces:[one]  — whole ns
//   restricted     = L7-only / specific selectors  — anything narrower than a whole ns

// rulesFromPolicy runs the policy through buildRulesByNs (matches how Evaluate
// calls it) and flattens the per-ns map into a single slice. Tests care about
// content and Coverage; per-ns bucketing is a separate concern.
func rulesFromPolicy(t *testing.T, authzPolicy *istiosec.AuthorizationPolicy, index map[string]models.NSIndex) []models.Rule {
	t.Helper()
	rulesByNs := buildRulesByNs(index, map[string][]*istiosec.AuthorizationPolicy{
		authzPolicy.Namespace: {authzPolicy},
	})
	var out []models.Rule
	for _, nsRules := range rulesByNs {
		out = append(out, nsRules...)
	}
	return out
}

// coverageFixture builds a two-namespace index: ns-a with a `target` workload
// (destination that policies select) and ns-b as a distinct source ns for
// allow-all-ns / cross-ns cases.
func coverageFixture() map[string]models.NSIndex {
	target := models.WorkloadNode{ID: "api-wid", Labels: map[string]string{"app": "api"}, Namespace: "ns-a"}
	nsA := models.WorkloadNode{ID: "ns-ns-a", Type: models.NodeTypeNamespace, Namespace: "ns-a", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-a"}}
	nsB := models.WorkloadNode{ID: "ns-ns-b", Type: models.NodeTypeNamespace, Namespace: "ns-b", Labels: map[string]string{"kubernetes.io/metadata.name": "ns-b"}}
	return map[string]models.NSIndex{
		"ns-a": buildFixtureNSIndex([]models.WorkloadNode{target, nsA}),
		"ns-b": buildFixtureNSIndex([]models.WorkloadNode{nsB}),
	}
}

// Canonical Istio deny-all: action:ALLOW with no rules matches nothing → allows
// nothing. Currently emits 0 rules (the rules loop never runs). The feature
// must synthesize one marker per selected workload so the "denies everything"
// case doesn't silently disappear from the graph.
func TestExpandRules_Coverage_AllowEmptyRulesIsDenyAll(t *testing.T) {
	index := coverageFixture()
	policy := makePolicy("empty-allow", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"}, nil)

	rules := rulesFromPolicy(t, policy, index)
	if len(rules) == 0 {
		t.Fatal("got 0 rules — empty ALLOW must surface as deny-all marker (currently invisible)")
	}
	for _, r := range rules {
		if r.Coverage != models.CoverageDenyAll {
			t.Errorf("Coverage=%q, want %q", r.Coverage, models.CoverageDenyAll)
		}
		if r.Action != models.ActionAllow {
			t.Errorf("Action=%d, want ActionAllow (marker keeps spec.Action; frontend pairs with Coverage)", r.Action)
		}
		if r.DstID != "api-wid" {
			t.Errorf("DstID=%q, want api-wid (marker must attribute to selected workload)", r.DstID)
		}
	}
}

// DENY + empty rules is a no-op — the policy exists but denies nothing. Doc
// fix (§ "The gap in the current Istio rule layer"): treat as unenforced so
// the policy stays visible without lying about enforcement. Distinct from
// deny-all under DENY (which is rules:[{}], catch-all).
func TestExpandRules_Coverage_DenyEmptyRulesIsUnenforced(t *testing.T) {
	index := coverageFixture()
	policy := makePolicy("empty-deny", "ns-a", istioapi.AuthorizationPolicy_DENY,
		map[string]string{"app": "api"}, nil)

	rules := rulesFromPolicy(t, policy, index)
	if len(rules) == 0 {
		t.Fatal("got 0 rules — empty DENY must surface as unenforced marker")
	}
	for _, r := range rules {
		if r.Coverage != models.CoverageUnenforced {
			t.Errorf("Coverage=%q, want %q (empty DENY denies nothing = no-op)", r.Coverage, models.CoverageUnenforced)
		}
		if r.Action != models.ActionDeny {
			t.Errorf("Action=%d, want ActionDeny", r.Action)
		}
	}
}

// DENY + rules:[{}] is the canonical Istio deny-everything: the empty rule
// matches any source, DENY blocks all of it. Same shape (empty rule) as
// AllowEmptyRules but opposite action → opposite intent → still deny-all.
func TestExpandRules_Coverage_DenyCatchAllIsDenyAll(t *testing.T) {
	index := coverageFixture()
	policy := makePolicy("deny-all", "ns-a", istioapi.AuthorizationPolicy_DENY,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{}}, // one empty rule = catch-all
	)

	rules := rulesFromPolicy(t, policy, index)
	if len(rules) == 0 {
		t.Fatal("got 0 rules — catch-all DENY must surface as deny-all marker")
	}
	for _, r := range rules {
		if r.Coverage != models.CoverageDenyAll {
			t.Errorf("Coverage=%q, want %q", r.Coverage, models.CoverageDenyAll)
		}
		if r.Action != models.ActionDeny {
			t.Errorf("Action=%d, want ActionDeny", r.Action)
		}
	}
}
// Allow + rules:[{}] allows everything
func TestExpandRules_Coverage_AllowCatchAllIsUnenforced(t *testing.T) {
	index := coverageFixture()
	policy := makePolicy("deny-all", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{}}, // one empty rule = catch-all
	)

	rules := rulesFromPolicy(t, policy, index)
	if len(rules) == 0 {
		t.Fatal("got 0 rules — catch-all must surface a rule")
	}
	for _, r := range rules {
		if r.Coverage != models.CoverageUnenforced {
			t.Errorf("Coverage=%q, want %q", r.Coverage, models.CoverageUnenforced)
		}
		if r.Action != models.ActionAllow {
			t.Errorf("Action=%d, want ActionDeny", r.Action)
		}
	}
}

// ALLOW rule with no from = any source. No L4/L7 narrowing → unbounded.
// Currently emits 0 rules because matchWorkloads stays empty when from is
// absent (buildRules.go:69 loop no-ops on nil from). Feature must synthesize
// a wildcard-source marker with AllPorts + AllL7.
func TestExpandRules_Coverage_AllowNoFromIsAllowAll(t *testing.T) {
	index := coverageFixture()
	policy := makePolicy("open", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			To: []*istioapi.Rule_To{{Operation: &istioapi.Operation{}}}, // no ports/L7
		}},
	)

	rules := rulesFromPolicy(t, policy, index)
	if len(rules) == 0 {
		t.Fatal("got 0 rules — allow-all (any source) must emit a marker (currently invisible)")
	}
	for _, r := range rules {
		if r.Coverage != models.CoverageAllowAll {
			t.Errorf("Coverage=%q, want %q", r.Coverage, models.CoverageAllowAll)
		}
		if !r.AllPorts {
			t.Errorf("AllPorts=false, want true (no port narrowing)")
		}
		if !r.AllL7 {
			t.Errorf("AllL7=false, want true (no L7 narrowing)")
		}
	}
}

// Allow-all narrowed by a port carries the port; Coverage stays allow-all
// (any source still allowed on that port). Mirrors k8s convention: portless
// empty peer = allow-all, ported empty peer = allow-all on that port.
func TestExpandRules_Coverage_AllowNoFromWithPortsCarriesPorts(t *testing.T) {
	index := coverageFixture()
	policy := makePolicy("open-port", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			To: []*istioapi.Rule_To{{Operation: &istioapi.Operation{Ports: []string{"8080"}}}},
		}},
	)

	rules := rulesFromPolicy(t, policy, index)
	if len(rules) == 0 {
		t.Fatal("got 0 rules — allow-all-with-port must emit a rule")
	}
	for _, r := range rules {
		if r.Coverage != models.CoverageAllowAll {
			t.Errorf("Coverage=%q, want %q (any source + port = still allow-all shape)", r.Coverage, models.CoverageAllowAll)
		}
		if r.AllPorts {
			t.Errorf("AllPorts=true, want false (port narrowing present)")
		}
		if len(r.Ports) != 1 || r.Ports[0].Port != 8080 {
			t.Errorf("Ports=%+v, want [{Port:8080}]", r.Ports)
		}
	}
}

// from.source.namespaces:[one ns] = allow-all-ns: everything from that ns.
// Currently classified as CoverageRestricted (:139 falls through). Feature
// must detect the whole-ns shape and set CoverageAllowAllNs. Guards the
// specific-selector-is-restricted test below from co-drifting.
func TestExpandRules_Coverage_FromSingleNamespaceIsAllowAllNs(t *testing.T) {
	index := coverageFixture()
	policy := makePolicy("from-ns-b", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			From: []*istioapi.Rule_From{{Source: &istioapi.Source{Namespaces: []string{"ns-b"}}}},
		}},
	)

	rules := rulesFromPolicy(t, policy, index)
	var nsSourced []models.Rule
	for _, r := range rules {
		if r.SrcID == "ns-ns-b" {
			nsSourced = append(nsSourced, r)
		}
	}
	if len(nsSourced) == 0 {
		t.Fatalf("got 0 rules sourced from ns-ns-b, all rules=%+v", rules)
	}
	for _, r := range nsSourced {
		if r.Coverage != models.CoverageAllowAll {
			t.Errorf("Coverage=%q, want %q (whole ns source = restricted)", r.Coverage, models.CoverageAllowAll)
		}
		if r.Action != models.ActionAllow {
			t.Errorf("Action=%d, want ActionAllow", r.Action)
		}
	}
}

// rules[to][operations]:
	// methods: ["GET"]
	// paths: ["/api/*"]
func TestExpandRules_Coverage_L7OnlyIsRestricted(t *testing.T) {
	index := coverageFixture()
	policy := makePolicy("l7-only", "ns-a", istioapi.AuthorizationPolicy_ALLOW,
		map[string]string{"app": "api"},
		[]*istioapi.Rule{{
			To: []*istioapi.Rule_To{{Operation: &istioapi.Operation{
				Methods: []string{"GET"},
				Paths:   []string{"/api/*"},
			}}},
		}},
	)

	rules := rulesFromPolicy(t, policy, index)
	if len(rules) == 0 {
		t.Fatal("got 0 rules — L7-only rule must still emit a marker (currently invisible)")
	}
	for _, r := range rules {
		if r.Coverage != models.CoverageRestricted {
			t.Errorf("Coverage=%q, want %q (L7 predicates restrict — not allow-all)", r.Coverage, models.CoverageRestricted)
		}
		if r.L7Match == nil {
			t.Fatalf("L7Match=nil, want populated")
		}
		if !reflect.DeepEqual(r.L7Match.Methods, []string{"GET"}) {
			t.Errorf("L7Match.Methods=%+v, want [GET]", r.L7Match.Methods)
		}
		if !reflect.DeepEqual(r.L7Match.Paths, []string{"/api/*"}) {
			t.Errorf("L7Match.Paths=%+v, want [/api/*]", r.L7Match.Paths)
		}
		if r.AllL7 {
			t.Errorf("AllL7=true, want false (L7 predicates present)")
		}
	}
}

// Contributor attribution guard: marker rules (deny-all / allow-all /
// unenforced) must carry the policy name + namespace so the detail-panel
// card can render "policy X denies everything." A marker without a
// contributor is a ghost — visible on the graph, unattributable in the UI.
func TestExpandRules_Coverage_MarkersCarryContributor(t *testing.T) {
	index := coverageFixture()
	cases := []struct {
		name   string
		policy *istiosec.AuthorizationPolicy
	}{
		{"deny-all ALLOW empty rules",
			makePolicy("mk-deny-all", "ns-a", istioapi.AuthorizationPolicy_ALLOW, map[string]string{"app": "api"}, nil)},
		{"unenforced DENY empty rules",
			makePolicy("mk-unenforced", "ns-a", istioapi.AuthorizationPolicy_DENY, map[string]string{"app": "api"}, nil)},
		{"allow-all no-from",
			makePolicy("mk-allow-all", "ns-a", istioapi.AuthorizationPolicy_ALLOW, map[string]string{"app": "api"},
				[]*istioapi.Rule{{To: []*istioapi.Rule_To{{Operation: &istioapi.Operation{}}}}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := rulesFromPolicy(t, tc.policy, index)
			if len(rules) == 0 {
				t.Fatal("got 0 rules — coverage marker feature not wired")
			}
			for _, r := range rules {
				if r.Contributor.Name != tc.policy.Name {
					t.Errorf("Contributor.Name=%q, want %q (marker orphaned from its policy)", r.Contributor.Name, tc.policy.Name)
				}
				if r.Contributor.Namespace != tc.policy.Namespace {
					t.Errorf("Contributor.Namespace=%q, want %q", r.Contributor.Namespace, tc.policy.Namespace)
				}
			}
		})
	}
}
