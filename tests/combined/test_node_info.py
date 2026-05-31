"""Node-info endpoint (`/api/node-info`) — per-engine rules + selecting
policies for a single workload.

Backend logic: `internal/store/buildStore.go::GetNodeData`. The response is
a `{engineName: {rules, policies}}` map; an engine is omitted entirely when
it has neither a matching rule nor a selecting policy for the node — that
"silence is meaningful" contract is what these tests guard.

Two viewpoints per policy:
  - the workload the policy selects (`policies` populated, `rules` may be empty)
  - the workload that appears as a rule's source (`rules` populated, `policies`
    only when a separate policy also selects it)

`GetNodeData` returns only outbound-style rules (`rule.SrcID == nodeId`).
Inbound matches are deliberately excluded — assertions below assume that.
"""

from istio import scenarios as istio_scenarios
from k8s import scenarios as k8s_scenarios
from libraries.constants import NS_A, NS_B
from libraries.graph_api import find_node


def _ids(graph: dict) -> tuple[str, str]:
    frontend = find_node(graph, predicate_label="frontend", predicate_namespace=NS_A)
    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_A)
    return frontend["id"], backend["id"]


def test_node_with_no_policies_returns_empty_map(get_graph, get_node_info):
    # Empty cluster → no engine has data for the workload → response is `{}`.
    graph = get_graph(NS_A)
    _, backend_id = _ids(graph)

    info = get_node_info(backend_id, NS_A)

    assert info == {}


def test_selecting_policy_appears_under_its_engine(get_graph, get_node_info):
    # k8s `allow-fe-be` selects backend. Backend's node-info should list it
    # under the k8s engine; istio has no data → istio key absent.
    k8s_scenarios.apply("allow_fe_to_be")
    graph = get_graph(NS_A)
    _, backend_id = _ids(graph)

    info = get_node_info(backend_id, NS_A)

    assert "k8s" in info
    policy_names = [policy["name"] for policy in info["k8s"]["policies"]]
    assert "allow-fe-be" in policy_names
    # istio sees nothing — engine entry must be omitted, not present-but-empty
    assert "istio" not in info


def test_outbound_rule_source_lists_its_rule(get_graph, get_node_info):
    # `allow-fe-be` produces a rule with SrcID=frontend, DstID=backend.
    # Querying frontend's node-info returns that rule under `rules`.
    # Frontend isn't selected by any policy, so `policies` is empty —
    # but the engine entry still appears because `rules` is non-empty.
    k8s_scenarios.apply("allow_fe_to_be")
    graph = get_graph(NS_A)
    frontend_id, _ = _ids(graph)

    info = get_node_info(frontend_id, NS_A)

    assert "k8s" in info
    assert len(info["k8s"]["rules"]) >= 1


def test_istio_policy_visible_on_selected_workload(get_graph, get_node_info):
    # Istio AuthorizationPolicy selecting backend → backend's node-info has an
    # istio entry. k8s remains silent because no k8s policy is applied.
    istio_scenarios.apply("allow_b_to_backend_in_a")
    graph = get_graph(NS_A, NS_B)
    _, backend_id = _ids(graph)

    info = get_node_info(backend_id, NS_A)

    assert "istio" in info
    assert info["istio"]["policies"], "istio policies list should not be empty"
    assert "k8s" not in info


def test_both_engines_listed_when_each_has_data(get_graph, get_node_info):
    # k8s + istio policies both select backend → both engine keys present,
    # each with its own policies list.
    k8s_scenarios.apply("allow_fe_to_be")
    istio_scenarios.apply("allow_b_to_backend_in_a")
    graph = get_graph(NS_A, NS_B)
    _, backend_id = _ids(graph)

    info = get_node_info(backend_id, NS_A)

    assert "k8s" in info and "istio" in info
    assert info["k8s"]["policies"], "k8s policies missing"
    assert info["istio"]["policies"], "istio policies missing"
