"""Cluster mutation facade. Re-exports common primitives and exposes the
per-engine sub-modules (cluster.k8s, cluster.istio) for engine-specific
helpers. delete_all_policies_in orchestrates wipe across every engine.
"""

from .common import create_namespace, create_pod, delete_namespace
from . import istio, k8s


def delete_all_policies_in(*namespaces: str) -> None:
    """Wipe every k8s NetworkPolicy + Istio AuthorizationPolicy + PeerAuthentication
    in the given namespaces. Per-test teardown — keeps the suite-scoped pod
    topology intact.
    """
    for namespace in namespaces:
        k8s.delete_all_in(namespace)
        istio.delete_all_in(namespace)
        istio.delete_all_pa_in(namespace)


__all__ = [
    "create_namespace",
    "create_pod",
    "delete_namespace",
    "delete_all_policies_in",
    "istio",
    "k8s",
]
