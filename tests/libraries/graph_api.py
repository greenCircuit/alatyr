"""Python helpers for inspecting /api/graph and /api/cluster-state responses.

Kept thin — Robot orchestrates HTTP via RequestsLibrary. These helpers handle
response shape navigation (nested JSON, list filtering) that's awkward in
Robot's `&{dict}` / `@{list}` syntax.
"""

from typing import Any


def node_ids(graph: dict) -> list[str]:
    return [node["id"] for node in graph.get("nodes", [])]


def namespace_node_ids(graph: dict) -> list[str]:
    return [node["id"] for node in graph.get("nodes", []) if node.get("type") == "namespace"]


def workload_nodes(graph: dict) -> list[dict]:
    return [node for node in graph.get("nodes", []) if node.get("type") != "namespace"]


def find_node(graph: dict, predicate_label: str = None, predicate_namespace: str = None) -> dict:
    """Return the first node matching given label/namespace filters. Raises if
    not found — failing fast in tests is the goal.
    """
    for node in graph.get("nodes", []):
        if predicate_label is not None and node.get("label") != predicate_label:
            continue
        if predicate_namespace is not None and node.get("namespace") != predicate_namespace:
            continue
        return node
    raise AssertionError(
        f"No node matched label={predicate_label!r} namespace={predicate_namespace!r}; "
        f"available: {[(n.get('label'), n.get('namespace')) for n in graph.get('nodes', [])]}"
    )


def node_statuses(graph: dict, label: str, namespace: str) -> list[str]:
    return list(find_node(graph, label, namespace).get("statuses", []))


def node_statuses_by_source(graph: dict, label: str, namespace: str, source: str) -> list[str]:
    """Per-engine status keys for one workload. Bypasses cross-engine
    intersection so engine-specific tests assert against pure engine output.
    """
    node = find_node(graph, label, namespace)
    return list(node.get("statusesBySource", {}).get(source, []))


def edges_by_source(graph: dict, policy_source: str) -> list[dict]:
    return [edge for edge in graph.get("edges", []) if edge.get("policySource") == policy_source]


def edge_count(graph: dict) -> int:
    return len(graph.get("edges", []))


def node_count(graph: dict) -> int:
    return len(graph.get("nodes", []))


def policy_sources(cluster_state: dict) -> list[str]:
    return list(cluster_state.get("policySources", []))


def available_namespaces(cluster_state: dict) -> list[str]:
    return list(cluster_state.get("availableNs", []))


def status_keys(cluster_state: dict) -> list[str]:
    return list(cluster_state.get("statusKeys", []))


def find_edge(
    graph: dict,
    source_id: str,
    target_id: str,
    policy_source: str | None = None,
) -> dict:
    """Return the first edge matching source/target (and optionally policySource). Raises if not found."""
    for edge in graph.get("edges", []):
        if edge.get("source") != source_id or edge.get("target") != target_id:
            continue
        if policy_source is not None and edge.get("policySource") != policy_source:
            continue
        return edge
    raise AssertionError(
        f"No edge source={source_id!r} target={target_id!r} policySource={policy_source!r}; "
        f"edges: {[(e.get('source'), e.get('target'), e.get('policySource')) for e in graph.get('edges', [])]}"
    )


def mesh_membership(detail: dict, source: str = "istio") -> dict | None:
    """Membership from /api/node-info NodeDetail. MeshMembership is a flat
    struct (single provider); `source` filters by `provider` field."""
    mesh = detail.get("mesh")
    if not mesh:
        return None
    if source and mesh.get("provider") != source:
        return None
    return mesh


def mtls_state(detail: dict, source: str = "istio") -> dict | None:
    """Resolved MtlsState for one mesh source, or None when unenrolled/absent."""
    membership = mesh_membership(detail, source)
    return membership.get("mtls") if membership else None


def mtls_issues(detail: dict, source: str = "istio") -> list[str]:
    """PA-config issue messages from MtlsState (root selector ignored, duplicate, etc)."""
    state = mtls_state(detail, source)
    if not state:
        return []
    return [entry.get("message", "") for entry in state.get("issues", [])]


def interop_issues(all_issues: list[dict], node_id: str) -> list[str]:
    """Filter /api/issues to ambient-HBONE (`mesh transport blocked`) findings
    scoped to one workload; returns the issue messages."""
    matches: list[str] = []
    for issue in all_issues:
        if issue.get("type") != "mesh transport blocked":
            continue
        node = issue.get("node")
        if not node or node.get("id") != node_id:
            continue
        matches.append(issue.get("message", ""))
    return matches


def assert_contains_all(actual: list[Any], expected: list[Any], label: str = "values") -> None:
    missing = [item for item in expected if item not in actual]
    if missing:
        raise AssertionError(f"{label} missing {missing}; actual={actual}")
