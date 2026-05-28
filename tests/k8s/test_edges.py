"""k8s NetworkPolicy → graph edge assertions."""

from libraries.constants import NS_A
from libraries.graph_api import edges_by_source

from . import scenarios


def test_k8s_netpol_produces_k8s_edge(get_graph):
    scenarios.apply("allow_fe_to_be")
    graph = get_graph(NS_A)
    assert edges_by_source(graph, "k8s")
    assert not edges_by_source(graph, "istio")
