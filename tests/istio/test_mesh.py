"""Mesh-layer (Istio ambient mTLS) E2E tests.

Mesh state is NOT in /api/graph — it is resolved on demand by /api/node-info
(membership + mTLS + interop issues) and /api/reachable (cross-node mTLS
verdict). Both read the cache /api/graph populates, so every test warms it with
get_graph(...) first.

Workloads live in NS_MESH, an ambient-enrolled namespace (ns-label opt-in, set
in conftest topology). resolveMtls / ValidateExternalRules run only for
enrolled, non-namespace nodes.
"""

import pytest
from kubernetes.client import (
    V1LabelSelector,
    V1NetworkPolicy,
    V1NetworkPolicyEgressRule,
    V1NetworkPolicyIngressRule,
    V1NetworkPolicyPeer,
    V1NetworkPolicyPort,
    V1NetworkPolicySpec,
    V1ObjectMeta,
)

from libraries import cluster
from libraries.constants import NS_A, NS_MESH
from libraries.graph_api import (
    find_node,
    interop_issues,
    mesh_membership,
    mtls_issues,
    mtls_state,
)

from . import mesh_scenarios

# Stable issue-code prefixes (see internal/mesh/istio/peerauth.go).
_ISSUE_ALL_UNSET = "all peer authentications are set to Unset"
_ISSUE_DUPLICATE = "duplicate at scope"
_ISSUE_ROOT_SELECTOR = "root selector ignored"
_ISSUE_PORT_NO_SELECTOR = "port level without selector"


def _detail(get_graph, get_node_info, label: str = "backend", namespace: str = NS_MESH) -> dict:
    """Warm the cache then fetch one workload's NodeDetail."""
    graph = get_graph(namespace)
    node = find_node(graph, predicate_label=label, predicate_namespace=namespace)
    return get_node_info(node["id"], namespace)


def _has_issue(issues: list[str], code: str) -> bool:
    return any(code in issue for issue in issues)


def _restrict_port_np(name: str, target_app: str, peer_app: str, direction: str, port: int) -> V1NetworkPolicy:
    """NetworkPolicy on target_app allowing only `port` to/from peer_app. Omitting
    the ztunnel HBONE port is what ValidateExternalRules flags for ambient pods."""
    peer = [V1NetworkPolicyPeer(pod_selector=V1LabelSelector(match_labels={"app": peer_app}))]
    ports = [V1NetworkPolicyPort(protocol="TCP", port=port)]
    spec_kwargs = {
        "pod_selector": V1LabelSelector(match_labels={"app": target_app}),
        "policy_types": [direction],
    }
    if direction == "Ingress":
        spec_kwargs["ingress"] = [V1NetworkPolicyIngressRule(_from=peer, ports=ports)]
    else:
        spec_kwargs["egress"] = [V1NetworkPolicyEgressRule(to=peer, ports=ports)]
    return V1NetworkPolicy(metadata=V1ObjectMeta(name=name), spec=V1NetworkPolicySpec(**spec_kwargs))


# ── mesh status: mTLS verdict resolution ──────────────────────────────────────

@pytest.mark.parametrize(
    "scenario, expected_verdict",
    [
        pytest.param("ns_strict",             "strict",     id="ns_strict"),
        pytest.param("ns_disable",            "disable",    id="ns_disable"),
        pytest.param("ns_permissive",         "permissive", id="ns_permissive"),
        # every matching PA UNSET → install-default permissive
        pytest.param("ns_unset",              "permissive", id="ns_unset_fallback"),
        # workload-selector STRICT wins over ns-scoped PERMISSIVE
        pytest.param("workload_overrides_ns", "strict",     id="precedence_workload"),
        # selectorless root-ns PA applies mesh-wide
        pytest.param("mesh_strict",           "strict",     id="mesh_scope"),
    ],
)
def test_mtls_verdict(scenario, expected_verdict, get_graph, get_node_info):
    mesh_scenarios.apply(scenario)
    detail = _detail(get_graph, get_node_info)
    assert mtls_state(detail)["verdict"] == expected_verdict


def test_enrolled_membership(get_graph, get_node_info):
    # No PA → enrolled workload reports membership + permissive default.
    detail = _detail(get_graph, get_node_info)
    membership = mesh_membership(detail)
    assert membership["inMesh"] is True
    assert membership["provider"] == "istio"
    assert membership["mode"] == "ambient"
    assert mtls_state(detail)["verdict"] == "permissive"


def test_port_override(get_graph, get_node_info):
    # workload STRICT with port 8080 DISABLE → portOverrides carries the per-port mode.
    mesh_scenarios.apply("port_override")
    detail = _detail(get_graph, get_node_info)
    state = mtls_state(detail)
    assert state["verdict"] == "strict"
    assert state["portOverrides"]["8080"] == "disable"


# ── reachability: transport-layer (mTLS) cross-node verdict ────────────────────

def test_mesh_can_talk(get_graph, get_reachability):
    # Both endpoints enrolled + STRICT → mesh permits (strict↔strict is fine).
    mesh_scenarios.apply("ns_strict")
    graph = get_graph(NS_MESH)
    frontend = find_node(graph, predicate_label="frontend", predicate_namespace=NS_MESH)
    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_MESH)

    result = get_reachability(frontend["id"], NS_MESH, backend["id"], NS_MESH)
    assert result["mesh"]["istio"]["verdict"] == "allow"
    assert result["verdict"] == "allow"


def test_cant_talk_outside_mesh(get_graph, get_reachability):
    # dst requires STRICT mTLS; src is unenrolled (cannot speak mTLS) → mesh denies.
    mesh_scenarios.apply("ns_strict")
    graph = get_graph(NS_A, NS_MESH)
    src = find_node(graph, predicate_label="backend", predicate_namespace=NS_A)
    dst = find_node(graph, predicate_label="backend", predicate_namespace=NS_MESH)

    result = get_reachability(src["id"], NS_A, dst["id"], NS_MESH)
    mesh_verdict = result["mesh"]["istio"]
    assert mesh_verdict["verdict"] == "deny"
    assert result["verdict"] == "deny"
    # New in this branch: reason string + effectiveSource pointer for UI drill-down.
    assert mesh_verdict.get("reason"), mesh_verdict
    effective = mesh_verdict.get("effectiveSource") or {}
    # ns_strict applies a selectorless PA in NS_MESH; oldest-wins → that PA is the source.
    assert effective.get("namespace") == NS_MESH, mesh_verdict
    assert effective.get("name"), mesh_verdict


# ── interop issues: NP/AuthZ on ambient pod that omits the ztunnel HBONE port ──

def test_capture_ingress_error(get_graph, get_issues):
    # Ingress NP selects frontend, allows FROM backend on 8080 only. k8s engine
    # rewrites SrcID=peer(backend), DstID=selected(frontend). GetNodeData filters
    # by SrcID, so backend's per-workload scan surfaces the rule — the resulting
    # hboneIssue attaches Node=backend.
    cluster.k8s.apply(NS_MESH, _restrict_port_np("ingress-no-hbone", "frontend", "backend", "Ingress", 8080))
    graph = get_graph(NS_MESH)
    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_MESH)
    issues = interop_issues(get_issues(), backend["id"])
    assert _has_issue(issues, "ambient ingress traffic is blocked"), issues
    assert _has_issue(issues, "15008"), issues


def test_capture_egress_error(get_graph, get_issues):
    # Egress NP on backend to frontend on 8080 only → backend egress is missing HBONE.
    cluster.k8s.apply(NS_MESH, _restrict_port_np("egress-no-hbone", "backend", "frontend", "Egress", 8080))
    graph = get_graph(NS_MESH)
    backend = find_node(graph, predicate_label="backend", predicate_namespace=NS_MESH)
    issues = interop_issues(get_issues(), backend["id"])
    assert _has_issue(issues, "ambient egress traffic is blocked"), issues
    assert _has_issue(issues, "15008"), issues


# ── PA-config issues ───────────────────────────────────────────────────────────

def test_capture_mesh_duplicate_pa_error(get_graph, get_node_info):
    # Two ns-scoped PAs at the same scope → oldest wins, rest reported as duplicate.
    mesh_scenarios.apply("dup_at_ns")
    detail = _detail(get_graph, get_node_info)
    assert _has_issue(mtls_issues(detail), _ISSUE_DUPLICATE), mtls_issues(detail)


@pytest.mark.parametrize(
    "scenario, issue_code",
    [
        pytest.param("ns_unset",              _ISSUE_ALL_UNSET,        id="all_unset"),
        pytest.param("root_selector_ignored", _ISSUE_ROOT_SELECTOR,    id="root_selector_ignored"),
        pytest.param("port_without_selector", _ISSUE_PORT_NO_SELECTOR, id="port_without_selector"),
    ],
)
def test_pa_issues(scenario, issue_code, get_graph, get_node_info):
    mesh_scenarios.apply(scenario)
    detail = _detail(get_graph, get_node_info)
    assert _has_issue(mtls_issues(detail), issue_code), mtls_issues(detail)


# ── membership shape: unenrolled + namespace-type nodes ───────────────────────

def test_unenrolled_workload_reports_not_in_mesh(get_graph, get_node_info):
    # NS_A has no ambient label → workloads must report inMesh=false + no Mtls.
    detail = _detail(get_graph, get_node_info, label="backend", namespace=NS_A)
    membership = detail.get("mesh") or {}
    assert membership.get("inMesh") is False, membership
    # Mtls should not resolve for unenrolled workloads.
    assert not membership.get("mtls"), membership


def test_namespace_node_inherits_ns_ambient_enrollment(get_graph, get_node_info):
    # Namespace-type node (ns-<name>) in an ambient-labelled ns reports the
    # namespace's own mesh posture — inMesh=true, mode=ambient. Guards against
    # regressions that would flip the ns node to un-enrolled and break the
    # cluster status rollup counters.
    get_graph(NS_MESH)
    ns_id = f"ns-{NS_MESH}"
    detail = get_node_info(ns_id, NS_MESH)
    membership = detail.get("mesh") or {}
    assert membership.get("inMesh") is True, membership
    assert membership.get("provider") == "istio", membership
    assert membership.get("mode") == "ambient", membership
    