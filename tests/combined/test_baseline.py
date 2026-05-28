"""Engine-agnostic graph behavior: cluster-state shape, empty-graph baseline,
and namespace filter. No policies applied — every test runs against the
suite-scoped pod topology alone.
"""

from libraries.constants import NS_A, NS_B, NS_C
from libraries.graph_api import (
    available_namespaces, edge_count, namespace_node_ids,
    policy_sources, status_keys, workload_nodes,
)


def test_cluster_state_exposes_engines_and_namespaces(get_cluster_state):
    state = get_cluster_state()
    sources = policy_sources(state)
    assert "k8s" in sources
    assert "istio" in sources
    for namespace in (NS_A, NS_B, NS_C):
        assert namespace in available_namespaces(state)
    assert status_keys(state), "status key catalog should not be empty"


def test_graph_without_policies_has_workloads_and_no_edges(get_graph):
    graph = get_graph(NS_A, NS_B, NS_C)
    assert len(namespace_node_ids(graph)) == 3
    assert len(workload_nodes(graph)) == 6
    assert edge_count(graph) == 0


def test_graph_filtered_by_namespace_returns_subset(get_graph):
    graph = get_graph(NS_A)
    assert len(namespace_node_ids(graph)) == 1
    assert len(workload_nodes(graph)) == 2
