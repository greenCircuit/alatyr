"""k8s NetworkPolicy create + wipe. Scenarios pass typed V1NetworkPolicy
objects directly; this module is a thin wrapper over the kubernetes client.
"""

from __future__ import annotations

from kubernetes.client import V1NetworkPolicy
from kubernetes.client.rest import ApiException

from .common import net, wait_for_informer


def apply(namespace: str, policy: V1NetworkPolicy) -> None:
    net().create_namespaced_network_policy(namespace=namespace, body=policy)
    wait_for_informer()


def delete_all_in(namespace: str) -> None:
    """Wholesale wipe of every NetworkPolicy in namespace. Per-test teardown."""
    try:
        net().delete_collection_namespaced_network_policy(namespace=namespace)
    except ApiException as exc:
        if exc.status != 404:
            raise
