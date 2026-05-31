"""Reachability endpoint (`/api/reachable`) — multi-engine src→dst verdict.

Backend logic lives in `internal/store/buildStore.go::IsNodesReachable`. These
tests exercise the verdict an operator actually reads from the UI: `allow`,
`deny`, plus the per-engine `not enforced` state. Internal shape (matched-rule
counts, reason strings, lock booleans) is intentionally not asserted — those
are covered by Go unit tests in `internal/store/buildStore_test.go`. Keep these
robust against refactors.
"""

from istio import scenarios as istio_scenarios
from k8s import scenarios as k8s_scenarios
from libraries.constants import NS_A, NS_B
from libraries.graph_api import find_node


def _ids(graph: dict) -> tuple[str, str]:
    """Resolve frontend and backend workload IDs from a graph response."""
    frontend = find_node(graph, predicate_label="frontend", predicate_namespace=NS_A)
    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_A)
    return frontend["id"], backend["id"]


def test_no_policies_verdict_allow_engines_not_enforced(get_graph, get_reachability):
    # Baseline: empty policy set → every engine is transparent → verdict allow,
    # both engines report `not enforced`.
    graph = get_graph(NS_A)
    src_id, dst_id = _ids(graph)

    result = get_reachability(src_id, NS_A, dst_id, NS_A)

    assert result["verdict"] == "allow"
    assert result["engines"]["k8s"]["status"] == "not enforced"
    assert result["engines"]["istio"]["status"] == "not enforced"


def test_k8s_allow_rule_grants_reachability(get_graph, get_reachability):
    # k8s ingress allow fe → be, no istio policy. k8s permits; istio transparent.
    k8s_scenarios.apply("allow_fe_to_be")
    graph = get_graph(NS_A)
    src_id, dst_id = _ids(graph)

    result = get_reachability(src_id, NS_A, dst_id, NS_A)

    assert result["verdict"] == "allow"
    assert result["engines"]["k8s"]["status"] == "allow"
    assert result["engines"]["istio"]["status"] == "not enforced"


def test_k8s_default_deny_blocks_when_locked_without_allow(get_graph, get_reachability):
    # Locked ingress + egress with no allow rule → k8s default-deny → overall deny.
    k8s_scenarios.apply("deny_all_backend")
    graph = get_graph(NS_A)
    src_id, dst_id = _ids(graph)

    result = get_reachability(src_id, NS_A, dst_id, NS_A)

    assert result["verdict"] == "deny"
    assert result["engines"]["k8s"]["status"] == "deny"
    assert "k8s" in result["reason"]


def test_istio_explicit_deny_blocks_path(get_graph, get_reachability):
    # Istio DENY policy targeting backend from NS_B → blocks the ns-b → backend path.
    # k8s has no opinion (not enforced); Istio's deny is enough to make verdict deny.
    istio_scenarios.apply("deny_from_other_ns")
    graph = get_graph(NS_A, NS_B)

    # Source is the NS_B namespace node — Istio rules use ns-node IDs for
    # `from.source.namespaces`. find_node by label="graph-test-b" wouldn't work,
    # so reach for the namespace node directly.
    ns_b_id = f"ns-{NS_B}"
    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_A)

    result = get_reachability(ns_b_id, NS_B, backend["id"], NS_A)

    assert result["verdict"] == "deny"
    assert result["engines"]["istio"]["status"] == "deny"
    assert "istio" in result["reason"]


def test_multi_engine_and_one_engine_blocks_overall(get_graph, get_reachability):
    # k8s allows fe → be; Istio adds an ingress lock on backend (deny-all)
    # that no allow rule matches. k8s says yes, Istio says no → overall deny.
    # Tests the AND-across-engines invariant: any blocking engine wins.
    k8s_scenarios.apply("allow_fe_to_be")
    istio_scenarios.apply("deny_all_backend")
    graph = get_graph(NS_A)
    src_id, dst_id = _ids(graph)

    result = get_reachability(src_id, NS_A, dst_id, NS_A)

    assert result["verdict"] == "deny"
    assert result["engines"]["k8s"]["status"] == "allow"
    assert result["engines"]["istio"]["status"] == "deny"
