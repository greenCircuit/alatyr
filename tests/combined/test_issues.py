"""Smoke tests for /api/issues — whole-cluster policy conflict scan.

Endpoint reads from the cache /api/graph populates, so every case fetches
the graph first. Base + one conflict edge case.
"""

from k8s import scenarios as k8s_scenarios
from istio import scenarios as istio_scenarios
from libraries.constants import NS_A


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
