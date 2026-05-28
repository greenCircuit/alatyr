"""Istio AuthorizationPolicy → graph edge assertions."""

from libraries.constants import NS_A, NS_B
from libraries.graph_api import edges_by_source, find_edge, find_node

from . import scenarios

_ACTION_ALLOW = 0
_ACTION_DENY = 1


def test_istio_authz_produces_istio_edge(get_graph):
    scenarios.apply("allow_b_to_backend_in_a")
    graph = get_graph(NS_A, NS_B)
    assert edges_by_source(graph, "istio")


def test_istio_edge_direction(get_graph):
    # Source of an Istio ingress rule is the namespace node of the FROM namespace.
    # Target is the selected workload. Verifies SrcID/DstID wiring in buildRules.go.
    scenarios.apply("allow_b_to_backend_in_a")
    graph = get_graph(NS_A, NS_B)

    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_A)
    ns_b_node_id = f"ns-{NS_B}"

    edge = find_edge(graph, source_id=ns_b_node_id, target_id=backend["id"], policy_source="istio")
    assert edge is not None


def test_istio_deny_edge_action(get_graph):
    # A DENY policy with an explicit From source must produce an edge with action=1 (Deny).
    scenarios.apply("deny_from_other_ns")
    graph = get_graph(NS_A, NS_B)

    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_A)
    ns_b_node_id = f"ns-{NS_B}"

    edge = find_edge(graph, source_id=ns_b_node_id, target_id=backend["id"], policy_source="istio")
    assert edge["action"] == _ACTION_DENY
