"""Cross-engine status intersection test.

Applies one k8s and one Istio policy to the same workload and asserts
that the effective `statuses` field reflects the intersection, not either
engine's view alone. Guards against regressions in IntersectPolicyStatus.

Fixture:
  k8s   allow_internet_egress  → k8s per-engine: internet-full
                                  (egress locked + internet egress open; ingress unlocked)
  istio allow_from_other_ns    → istio per-engine: internet-egress, cross-namespace
                                  (ingress locked to NS_B only)

Intersection drops:
  - internet-full  (Istio locks ingress, so k8s's open ingress no longer counts)
  - cross-namespace (k8s doesn't explicitly grant CrossNS, AND zeros it out)

Effective result: internet-egress only — distinct from both per-engine views.
"""

from k8s import scenarios as k8s_scenarios
from istio import scenarios as istio_scenarios
from libraries.constants import NS_A
from libraries.graph_api import node_statuses, node_statuses_by_source


def test_effective_status_is_intersection_of_both_engines(get_graph):
    k8s_scenarios.apply("allow_internet_egress")
    istio_scenarios.apply("allow_from_other_ns")
    graph = get_graph(NS_A)

    k8s_view = set(node_statuses_by_source(graph, "backend", NS_A, "k8s"))
    istio_view = set(node_statuses_by_source(graph, "backend", NS_A, "istio"))
    effective = set(node_statuses(graph, "backend", NS_A))

    assert k8s_view == {"internet-full"}
    assert istio_view == {"internet-egress", "cross-namespace"}
    assert effective == {"internet-egress"}
    # both per-engine views differ from effective — proves intersection is running
    assert effective != k8s_view
    assert effective != istio_view


# Regression: only one engine has a policy on the workload, the other engine
# sees nothing. Effective must match the engine with the policy — the zero-
# status engine has "no opinion" and must not inflate the intersection via
# transparency (Lan/InnerNs/ApiServer must NOT fire). Before the fix in
# IntersectPolicyStatus, the istio side's transparency would flip every
# unlocked-egress dimension to true, over-claiming 3-4 extra badges.
def test_no_opinion_engine_does_not_over_claim(get_graph):
    # k8s ingress-only policy on backend (no internet/LAN peers — only a pod ref).
    # Istio: no policy applied → engine produces zero PolicyStatus for backend.
    k8s_scenarios.apply("allow_fe_to_be")
    graph = get_graph(NS_A)

    k8s_view = set(node_statuses_by_source(graph, "backend", NS_A, "k8s"))
    istio_view = set(node_statuses_by_source(graph, "backend", NS_A, "istio"))
    effective = set(node_statuses(graph, "backend", NS_A))

    assert k8s_view == {"internet-egress"}          # ingress locked, egress transparent → internet egress
    assert istio_view == {"internet-full"}           # zero status → transparent both ways
    assert effective == {"internet-egress"}          # matches k8s view, zero engine ignored
    # the over-claim regression: any of these would mean filter is broken
    assert "lan-egress" not in effective
    assert "api-server-egress" not in effective
    assert "ns-egress-access" not in effective
