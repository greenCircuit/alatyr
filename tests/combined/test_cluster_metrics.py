"""Smoke tests for /api/cluster-metrics — cluster totals + mesh posture counters.

MeshMetrics is populated by BuildMeshMembership during PopulateCache; totals
(NsTotal/WorkloadTotal) read from cache sizes at request time. Every test
warms /api/graph first so the cache is fresh.
"""

from istio import mesh_scenarios
from libraries.constants import NS_MESH, POLICY_NAMESPACES


def _metrics(get_graph, get_cluster_metrics) -> dict:
    """Warm the graph then fetch cluster metrics."""
    get_graph(*POLICY_NAMESPACES)
    return get_cluster_metrics()


def test_cluster_metrics_reports_topology_totals(get_graph, get_cluster_metrics):
    # Baseline: topology has NS_MESH + NS_A/B/C, two pods each. Cluster may hold
    # additional namespaces from prior state, so lower-bound the assertion.
    metrics = _metrics(get_graph, get_cluster_metrics)

    assert metrics.get("nsTotal", 0) >= 4, metrics
    assert metrics.get("workloadTotal", 0) >= 8, metrics


def test_mesh_metrics_marks_enrolled_ns_and_workloads(get_graph, get_cluster_metrics):
    # NS_MESH is ambient-enrolled with 2 workloads → NsEnrolled + WorkloadsEnrolled
    # must count them.
    metrics = _metrics(get_graph, get_cluster_metrics)
    mesh = metrics.get("meshMetrics") or {}

    assert mesh.get("nsEnrolled", 0) >= 1, mesh
    assert mesh.get("workloadsEnrolled", 0) >= 2, mesh


def test_mesh_metrics_shifts_to_strict_when_ns_pa_applied(get_graph, get_cluster_metrics):
    # ns-scoped STRICT PA covers both NS_MESH workloads → MtlsStrict jumps by 2.
    baseline = _metrics(get_graph, get_cluster_metrics)
    baseline_strict = (baseline.get("meshMetrics") or {}).get("mtlsStrict", 0)

    mesh_scenarios.apply("ns_strict")
    after = _metrics(get_graph, get_cluster_metrics)
    after_strict = (after.get("meshMetrics") or {}).get("mtlsStrict", 0)

    assert after_strict - baseline_strict >= 2, (baseline, after)


def test_mesh_metrics_shifts_to_disabled_when_pa_disables(get_graph, get_cluster_metrics):
    baseline = _metrics(get_graph, get_cluster_metrics)
    baseline_disabled = (baseline.get("meshMetrics") or {}).get("mtlsDisabled", 0)

    mesh_scenarios.apply("ns_disable")
    after = _metrics(get_graph, get_cluster_metrics)
    after_disabled = (after.get("meshMetrics") or {}).get("mtlsDisabled", 0)

    assert after_disabled - baseline_disabled >= 2, (baseline, after)
