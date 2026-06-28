"""Policy manifest endpoint (`/api/manifest`) — on-demand YAML fetch straight
from the apiserver, bypassing the graph cache. Verifies the real round-trip:
apiserver object → server-side noise stripped → clean YAML envelope. Unit tests
mock the client, so they can't catch a managedFields-stripping regression
against an object the apiserver actually populated.
"""

from k8s import scenarios
from libraries.constants import NS_A


def test_manifest_returns_clean_yaml_for_applied_netpol(get_manifest):
    scenarios.apply("allow_fe_to_be")

    response = get_manifest("k8s", NS_A, "allow-fe-be")

    assert response.status_code == 200, response.text
    body = response.json()
    assert body["kind"] == "NetworkPolicy"
    assert body["name"] == "allow-fe-be"
    manifest = body["yaml"]
    assert "kind: NetworkPolicy" in manifest
    assert "name: allow-fe-be" in manifest
    # apiserver stamps managedFields on every write — endpoint must strip it.
    assert "managedFields" not in manifest


def test_manifest_unknown_policy_returns_error(get_manifest):
    # Real apiserver NotFound → 400, not a 500 or empty 200.
    response = get_manifest("k8s", NS_A, "does-not-exist")
    assert response.status_code == 400
