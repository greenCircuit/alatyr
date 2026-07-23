"""Smoke tests for /api/mesh-status — per-workload mesh membership hydration.

Endpoint returns {nodes: {nodeId: MeshMembership}} straight from the cache
/api/graph populates. Mtls is intentionally omitted from this payload —
resolved on demand by /api/node-info instead.
"""

from libraries.constants import NS_A, NS_MESH
from libraries.graph_api import find_node


def test_mesh_status_reports_ambient_workloads(get_graph, get_mesh_status):
    # NS_MESH is ambient-enrolled via ns label; every workload there must
    # report inMesh + provider/mode. Mtls stays nil in this payload.
    graph = get_graph(NS_MESH)
    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_MESH)
    frontend = find_node(graph, predicate_label="frontend", predicate_namespace=NS_MESH)

    status = get_mesh_status()
    nodes = status.get("nodes") or {}

    for node in (backend, frontend):
        entry = nodes.get(node["id"])
        assert entry is not None, f"{node['id']} missing from mesh-status; keys={list(nodes)[:10]}"
        assert entry.get("inMesh") is True, entry
        assert entry.get("provider") == "istio", entry
        assert entry.get("mode") == "ambient", entry
        # Endpoint returns cache.MeshMembership as-is, which includes the
        # resolved MtlsState. With no PA applied the verdict falls back to the
        # install default (permissive) and effectiveSource is empty.
        mtls = entry.get("mtls") or {}
        assert mtls.get("verdict") == "permissive", entry


def test_mesh_status_marks_unenrolled_workloads(get_graph, get_mesh_status):
    # NS_A has no ambient label → workloads report inMesh=false. Provider/mode
    # may be empty strings depending on omitempty; only inMesh is contractual.
    graph = get_graph(NS_A, NS_MESH)
    unenrolled = find_node(graph, predicate_label="backend", predicate_namespace=NS_A)

    status = get_mesh_status()
    entry = (status.get("nodes") or {}).get(unenrolled["id"])
    assert entry is not None, f"{unenrolled['id']} missing from mesh-status"
    assert entry.get("inMesh") is False, entry
