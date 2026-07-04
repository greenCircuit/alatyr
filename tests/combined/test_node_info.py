"""Node-info endpoint (`/api/node-info`) — per-engine policy *neighbors* for a
single workload, plus mesh membership + interop issues.

Backend logic: `internal/store/buildNodeNeighbor.go::BuildNodeNeighbor`
(neighbors) + `GetWorkloadMesh` (mesh) + `ValidateExternalRules` (issues).
Response shape:
    {neighbors: {engineName: {In, Out}}, mesh?: {...}, issues?: [...]}

`neighbors` is adjacency per engine, keyed by `PolicySource.Name()`:
  - `In`  = clicked node is the rule destination (a peer talks *to* it)
  - `Out` = clicked node is the rule source      (it talks *to* a peer)
Each entry is a `NeighborRef{Rule, Workload}` — the matched rule plus the
resolved peer workload. No json tags on the Go structs, so keys are PascalCase
(`In`/`Out`/`Rule`/`Workload`); nil slices serialize to `null`.

Unlike the old `policies` contract, an engine is *not* omitted when it has no
data — every registered engine gets a key with empty (`null`) In/Out. So
"silence" is an engine present with no neighbors, not an absent key. Walks every
namespace bucket, so cross-ns neighbors are included (the branch's fix).
"""

from istio import scenarios as istio_scenarios
from k8s import scenarios as k8s_scenarios
from libraries.constants import NS_A, NS_B
from libraries.graph_api import find_node


def _ids(graph: dict) -> tuple[str, str]:
    frontend = find_node(graph, predicate_label="frontend", predicate_namespace=NS_A)
    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_A)
    return frontend["id"], backend["id"]


def _neighbors(info: dict, engine: str) -> dict:
    # Tolerate absent engine key and null slices — both mean "no neighbors".
    entry = info["neighbors"].get(engine) or {}
    return {"in": entry.get("In") or [], "out": entry.get("Out") or []}


def _peer_labels(refs: list) -> list[str]:
    return [ref["Workload"]["label"] for ref in refs]


def _policy_names(refs: list) -> list[str]:
    return [ref["Rule"].get("contributor", {}).get("name") for ref in refs]


def _has_any_neighbor(info: dict) -> bool:
    return any(
        _neighbors(info, engine)["in"] or _neighbors(info, engine)["out"]
        for engine in info["neighbors"]
    )


def test_node_with_no_policies_has_no_neighbors(get_graph, get_node_info):
    # Empty cluster → no rule touches the workload → every engine present with
    # empty In/Out.
    graph = get_graph(NS_A)
    _, backend_id = _ids(graph)

    info = get_node_info(backend_id, NS_A)

    assert not _has_any_neighbor(info)


def test_inbound_neighbor_lists_source(get_graph, get_node_info):
    # k8s `allow-fe-be` (frontend→backend) → backend is the destination, so
    # frontend shows under backend's k8s `In`. istio has no rule → no neighbors.
    k8s_scenarios.apply("allow_fe_to_be")
    graph = get_graph(NS_A)
    _, backend_id = _ids(graph)

    info = get_node_info(backend_id, NS_A)
    k8s = _neighbors(info, "k8s")

    assert "frontend" in _peer_labels(k8s["in"])
    assert "allow-fe-be" in _policy_names(k8s["in"])
    # istio sees nothing — present but empty
    istio = _neighbors(info, "istio")
    assert not istio["in"] and not istio["out"]


def test_outbound_neighbor_lists_destination(get_graph, get_node_info):
    # Same `allow-fe-be` rule from frontend's viewpoint: frontend is the source,
    # so backend shows under frontend's k8s `Out`.
    k8s_scenarios.apply("allow_fe_to_be")
    graph = get_graph(NS_A)
    frontend_id, _ = _ids(graph)

    info = get_node_info(frontend_id, NS_A)
    k8s = _neighbors(info, "k8s")

    assert "backend" in _peer_labels(k8s["out"])


def test_istio_neighbor_visible_on_selected_workload(get_graph, get_node_info):
    # Istio AuthorizationPolicy allowing ns-b → backend (ingress) → backend is
    # the destination, so istio `In` is non-empty. k8s stays silent.
    istio_scenarios.apply("allow_b_to_backend_in_a")
    graph = get_graph(NS_A, NS_B)
    _, backend_id = _ids(graph)

    info = get_node_info(backend_id, NS_A)

    assert _neighbors(info, "istio")["in"], "istio inbound neighbors should not be empty"
    k8s = _neighbors(info, "k8s")
    assert not k8s["in"] and not k8s["out"]


def test_both_engines_have_neighbors_when_each_has_data(get_graph, get_node_info):
    # k8s + istio both target backend → both engines carry inbound neighbors.
    k8s_scenarios.apply("allow_fe_to_be")
    istio_scenarios.apply("allow_b_to_backend_in_a")
    graph = get_graph(NS_A, NS_B)
    _, backend_id = _ids(graph)

    info = get_node_info(backend_id, NS_A)

    assert _neighbors(info, "k8s")["in"], "k8s inbound missing"
    assert _neighbors(info, "istio")["in"], "istio inbound missing"
