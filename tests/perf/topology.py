"""Scaled topology generator for the initial-load perf sweep.

Builds N namespaces, each with a fixed number of frontend/backend pods plus
one NetworkPolicy (k8s engine) and one AuthorizationPolicy (istio engine), so
both policy engines do real work during cache population. Namespace names are
prefixed per size so concurrent sizes never collide and teardown is wholesale.
"""

from __future__ import annotations

from typing import Callable

from kubernetes.client import (
    V1LabelSelector,
    V1NetworkPolicy,
    V1NetworkPolicyIngressRule,
    V1NetworkPolicyPeer,
    V1NetworkPolicySpec,
    V1ObjectMeta,
)
from kubernetes.client.rest import ApiException

from libraries import cluster


def _ignore_conflict(create: Callable[[], None]) -> None:
    """Run a create, swallowing 409 AlreadyExists so build() is idempotent.

    The shared cluster.k8s / cluster.istio apply helpers raise on 409 (unlike
    namespace/pod creates). Two sweep cases that resolve to the same
    (size, pods, policies) tuple share namespaces — without this the second
    case's policy creates collide.
    """
    try:
        create()
    except ApiException as exc:
        if exc.status != 409:
            raise


# Every namespace created by build() is registered here so the session-end
# cleanup can wipe them all regardless of which axis a test swept.
_created: set[str] = set()


def ns_names(size: int, pods_per_ns: int, policies_per_engine: int) -> list[str]:
    """Namespace names keyed by the full (size, pods, policies) combo.

    Encoding all three dimensions means different sweep cases never collide —
    a single-ns workload sweep and a single-ns policy sweep get distinct
    namespaces instead of stomping a shared `perf-1-ns0`.
    """
    prefix = f"perf-s{size}-p{pods_per_ns}-a{policies_per_engine}"
    return [f"{prefix}-ns{index}" for index in range(size)]


def _network_policy(name: str) -> V1NetworkPolicy:
    # Ingress lock on backend, allow from frontend — exercises k8s buildRules +
    # status-key derivation (locked ingress + an allow tuple).
    return V1NetworkPolicy(
        metadata=V1ObjectMeta(name=name),
        spec=V1NetworkPolicySpec(
            pod_selector=V1LabelSelector(match_labels={"app": "backend"}),
            policy_types=["Ingress"],
            ingress=[
                V1NetworkPolicyIngressRule(
                    _from=[
                        V1NetworkPolicyPeer(
                            pod_selector=V1LabelSelector(match_labels={"app": "frontend"})
                        )
                    ]
                )
            ],
        ),
    )


def _authorization_policy(name: str, namespace: str) -> dict:
    # ALLOW on backend from the frontend SA — exercises istio buildRules +
    # ALLOW/DENY split + per-workload PolicyStatus.
    return {
        "metadata": {"name": name},
        "spec": {
            "selector": {"matchLabels": {"app": "backend"}},
            "action": "ALLOW",
            "rules": [
                {
                    "from": [
                        {
                            "source": {
                                "principals": [
                                    f"cluster.local/ns/{namespace}/sa/default"
                                ]
                            }
                        }
                    ]
                }
            ],
        },
    }


def build(size: int, pods_per_ns: int, policies_per_engine: int) -> list[str]:
    """Create a size-namespace topology with both engines' policies.

    Per namespace: pods_per_ns pods (split frontend/backend) plus
    policies_per_engine NetworkPolicies AND policies_per_engine
    AuthorizationPolicies, so each engine fetches+evaluates a real policy set.
    Idempotent: every underlying create tolerates 409, so re-running a size is
    safe. Returns the namespace names so the caller can scope the graph request.
    """
    namespaces = ns_names(size, pods_per_ns, policies_per_engine)
    _created.update(namespaces)
    half = max(1, pods_per_ns // 2)
    for namespace in namespaces:
        cluster.create_namespace(namespace)
        for index in range(half):
            cluster.create_pod(namespace, f"frontend-{index}", {"app": "frontend"})
            cluster.create_pod(namespace, f"backend-{index}", {"app": "backend"})
        for index in range(policies_per_engine):
            _ignore_conflict(
                lambda ns=namespace, i=index: cluster.k8s.apply(
                    ns, _network_policy(f"perf-netpol-{i}")
                )
            )
            _ignore_conflict(
                lambda ns=namespace, i=index: cluster.istio.apply(
                    ns, _authorization_policy(f"perf-authz-{i}", ns)
                )
            )
    return namespaces


def teardown_all() -> None:
    """Delete every namespace any build() created this session."""
    for namespace in _created:
        cluster.delete_namespace(namespace)
    _created.clear()
