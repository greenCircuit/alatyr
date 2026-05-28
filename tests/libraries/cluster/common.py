"""Shared cluster primitives: kubeconfig loading, kubernetes API client
accessors, namespace/pod/ServiceAccount lifecycle. Engine-specific helpers
live in cluster/k8s.py and cluster/istio.py.
"""

from __future__ import annotations

import os

from kubernetes import client, config
from kubernetes.client.rest import ApiException


_loaded = False


def _load_config() -> None:
    kubeconfig = os.environ.get("KUBECONFIG")
    if kubeconfig:
        config.load_kube_config(config_file=kubeconfig)
    else:
        config.load_kube_config()


def core() -> client.CoreV1Api:
    global _loaded
    if not _loaded:
        _load_config()
        _loaded = True
    return client.CoreV1Api()


def net() -> client.NetworkingV1Api:
    global _loaded
    if not _loaded:
        _load_config()
        _loaded = True
    return client.NetworkingV1Api()


def custom() -> client.CustomObjectsApi:
    global _loaded
    if not _loaded:
        _load_config()
        _loaded = True
    return client.CustomObjectsApi()


def create_namespace(name: str, labels: dict | None = None) -> None:
    body = client.V1Namespace(
        metadata=client.V1ObjectMeta(
            name=name,
            labels={"name": name, **(labels or {})},
        )
    )
    try:
        core().create_namespace(body=body)
    except ApiException as exc:
        if exc.status != 409:
            raise
    # kwok runs apiserver only — no controller-manager creates the "default"
    # ServiceAccount, so the ServiceAccount admission plugin rejects pods.
    _ensure_default_service_account(name)


def _ensure_default_service_account(namespace: str) -> None:
    body = client.V1ServiceAccount(metadata=client.V1ObjectMeta(name="default"))
    try:
        core().create_namespaced_service_account(namespace=namespace, body=body)
    except ApiException as exc:
        if exc.status != 409:
            raise


def delete_namespace(name: str) -> None:
    try:
        core().delete_namespace(name=name)
    except ApiException as exc:
        if exc.status != 404:
            raise


def create_pod(namespace: str, name: str, labels: dict) -> None:
    body = client.V1Pod(
        metadata=client.V1ObjectMeta(name=name, labels=labels),
        spec=client.V1PodSpec(
            containers=[client.V1Container(name="app", image="registry.k8s.io/pause:3.9")]
        ),
    )
    try:
        core().create_namespaced_pod(namespace=namespace, body=body)
    except ApiException as exc:
        if exc.status != 409:
            raise
