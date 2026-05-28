"""k8s NetworkPolicy scenarios. Each entry = (namespace, V1NetworkPolicy)
with nested fields as plain dicts (camelCase keys — wire format). apply(name)
creates it. Tests reference by name.
"""

from __future__ import annotations

from kubernetes.client import V1NetworkPolicy

from libraries.cluster import k8s as cluster_k8s
from libraries.constants import NS_A


SCENARIOS: dict[str, tuple[str, V1NetworkPolicy]] = {
    "allow_fe_to_be": (
        NS_A,
        V1NetworkPolicy(
            metadata={"name": "allow-fe-be"},
            spec={
                "podSelector": {"matchLabels": {"app": "backend"}},
                "policyTypes": ["Ingress"],
                "ingress": [
                    {
                        "from": [{"podSelector": {"matchLabels": {"app": "frontend"}}}],
                        "ports": [{"protocol": "TCP", "port": 8080}],
                    },
                ],
            },
        ),
    ),
    "deny_all_backend": (
        NS_A,
        V1NetworkPolicy(
            metadata={"name": "deny-all-backend"},
            spec={
                "podSelector": {"matchLabels": {"app": "backend"}},
                "policyTypes": ["Ingress", "Egress"],
            },
        ),
    ),
    "pod_ns_egress": (
        NS_A,
        V1NetworkPolicy(
            metadata={"name": "allow"},
            spec={
                "podSelector": {"matchLabels": {"app": "backend"}},
                "policyTypes": ["Egress"],
                "ingress": [
                    {
                        "from": [{"namespaceSelector": {"matchLabels": {"name": "NS_B"}}}],
                    },
                ],
            },
        ),
    ),
    "allow_internet_egress": (
        NS_A,
        V1NetworkPolicy(
            metadata={"name": "allow-internet-egress"},
            spec={
                "podSelector": {"matchLabels": {"app": "backend"}},
                "policyTypes": ["Egress"],
                "egress": [
                    {"to": [{"ipBlock": {"cidr": "0.0.0.0/0"}}]},
                ],
            },
        ),
    ),
    "allow_internet_ingress": (
        NS_A,
        V1NetworkPolicy(
            metadata={"name": "allow-internet-ingress"},
            spec={
                "podSelector": {"matchLabels": {"app": "backend"}},
                "policyTypes": ["Ingress"],
                "ingress": [
                    {"from": [{"ipBlock": {"cidr": "0.0.0.0/0"}}]},
                ],
            },
        ),
    ),
    "deny_egress_only": (
        NS_A,
        V1NetworkPolicy(
            metadata={"name": "deny-egress-only"},
            spec={
                "podSelector": {"matchLabels": {"app": "backend"}},
                "policyTypes": ["Egress"],
            },
        ),
    ),
}


def apply(name: str) -> None:
    if name not in SCENARIOS:
        raise KeyError(f"unknown k8s scenario {name!r}; known: {sorted(SCENARIOS)}")
    namespace, policy = SCENARIOS[name]
    cluster_k8s.apply(namespace, policy)
