"""Smoke tests for /api/issues — whole-cluster policy conflict scan.

Endpoint reads from the cache /api/graph populates, so every case fetches
the graph first. Base + one conflict edge case.
"""

from kubernetes.client import (
    V1LabelSelector,
    V1NetworkPolicy,
    V1NetworkPolicyIngressRule,
    V1NetworkPolicyPeer,
    V1NetworkPolicySpec,
    V1ObjectMeta,
)

from istio import mesh_scenarios
from k8s import scenarios as k8s_scenarios
from istio import scenarios as istio_scenarios
from libraries import cluster
from libraries.constants import NS_A, NS_MESH


def test_issues_empty_when_no_policies(get_graph, get_issues):
    get_graph(NS_A)

    issues = get_issues()

    conflicts = [issue for issue in issues if issue.get("type") == "policy conflict"]
    assert conflicts == []


# k8s ALLOW fe→be inside NS_A + istio DENY-all on backend → k8s permits, istio
# blocks. PolicyIssues must emit one "policy conflict" attributed to istio,
# both endpoints populated so the UI can open reachability for the pair.
def test_policy_conflict_flagged_when_engines_disagree(get_graph, get_issues):
    k8s_scenarios.apply("allow_fe_to_be")
    istio_scenarios.apply("deny_all_backend")
    get_graph(NS_A)

    issues = get_issues()

    conflicts = [issue for issue in issues if issue.get("type") == "policy conflict"]
    assert len(conflicts) == 1, f"expected 1 policy conflict, got {conflicts}"
    conflict = conflicts[0]
    assert conflict.get("engine") == "istio"
    src = conflict.get("src") or {}
    dst = conflict.get("dst") or {}
    assert src.get("label") == "frontend" and src.get("namespace") == NS_A
    assert dst.get("label") == "backend"  and dst.get("namespace") == NS_A


# ── mesh conflict: allow rule exists, mesh mTLS blocks the pair ───────────────

def _cross_ns_allow_np() -> V1NetworkPolicy:
    """k8s NP on NS_MESH backend allowing ingress from NS_A frontend pods.
    namespaceSelector uses the `name` label create_namespace stamps."""
    peer = V1NetworkPolicyPeer(
        namespace_selector=V1LabelSelector(match_labels={"name": NS_A}),
        pod_selector=V1LabelSelector(match_labels={"app": "frontend"}),
    )
    spec = V1NetworkPolicySpec(
        pod_selector=V1LabelSelector(match_labels={"app": "backend"}),
        policy_types=["Ingress"],
        ingress=[V1NetworkPolicyIngressRule(_from=[peer])],
    )
    return V1NetworkPolicy(metadata=V1ObjectMeta(name="allow-a-fe"), spec=spec)


def test_mesh_conflict_flagged_when_strict_denies_permitted_edge(get_graph, get_issues):
    # k8s NP on NS_MESH backend grants ingress from NS_A frontend → allow rule
    # keyed on both workload IDs (not the ns-node) so PolicyIssues can probe the
    # mesh. ns_strict PA pushes NS_MESH backend to STRICT; NS_A is unenrolled so
    # its frontend can't speak mTLS → mesh denies → MeshConflicts.
    cluster.k8s.apply(NS_MESH, _cross_ns_allow_np())
    mesh_scenarios.apply("ns_strict")
    get_graph(NS_A, NS_MESH)

    issues = get_issues()

    conflicts = [issue for issue in issues if issue.get("type") == "mesh conflict"]
    assert len(conflicts) >= 1, f"expected mesh conflict, got issue types: {[i.get('type') for i in issues]}"
    conflict = conflicts[0]
    src_membership = conflict.get("srcMembership") or {}
    dst_membership = conflict.get("dstMembership") or {}
    assert src_membership.get("inMesh") is False, src_membership
    assert dst_membership.get("inMesh") is True, dst_membership
    dst = conflict.get("dst") or {}
    assert dst.get("namespace") == NS_MESH, dst
