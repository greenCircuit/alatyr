"""Cross-engine tests. Compose scenarios from k8s + istio engines and
assert both render their own edges without merging.
"""

from istio import scenarios as istio_scenarios
from k8s import scenarios as k8s_scenarios
from libraries.constants import NS_A, NS_B, NS_C
from libraries.graph_api import edges_by_source


def test_both_engines_produce_distinct_edges_side_by_side(get_graph):
    k8s_scenarios.apply("allow_fe_to_be")
    istio_scenarios.apply("allow_b_to_backend_in_a")
    graph = get_graph(NS_A, NS_B, NS_C)
    assert edges_by_source(graph, "k8s"), "no k8s edges produced"
    assert edges_by_source(graph, "istio"), "no istio edges produced"
