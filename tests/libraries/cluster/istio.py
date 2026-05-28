"""Istio AuthorizationPolicy helpers."""

from __future__ import annotations

from kubernetes.client.rest import ApiException

from .common import custom

_AUTHZ_GROUP = "security.istio.io"
_AUTHZ_VERSION = "v1beta1"
_AUTHZ_PLURAL = "authorizationpolicies"


def apply(namespace: str, body: dict) -> None:
    """Apply an AuthorizationPolicy. body must contain metadata + spec;
    apiVersion and kind are injected automatically."""
    custom().create_namespaced_custom_object(
        group=_AUTHZ_GROUP,
        version=_AUTHZ_VERSION,
        namespace=namespace,
        plural=_AUTHZ_PLURAL,
        body={
            "apiVersion": f"{_AUTHZ_GROUP}/{_AUTHZ_VERSION}",
            "kind": "AuthorizationPolicy",
            **body,
        },
    )


def delete_all_in(namespace: str) -> None:
    """Wholesale wipe of every AuthorizationPolicy in namespace. Per-test teardown."""
    try:
        existing = custom().list_namespaced_custom_object(
            group=_AUTHZ_GROUP,
            version=_AUTHZ_VERSION,
            namespace=namespace,
            plural=_AUTHZ_PLURAL,
        )
    except ApiException as exc:
        if exc.status == 404:
            return
        raise
    for item in existing.get("items", []):
        policy_name = item.get("metadata", {}).get("name")
        if not policy_name:
            continue
        custom().delete_namespaced_custom_object(
            group=_AUTHZ_GROUP,
            version=_AUTHZ_VERSION,
            namespace=namespace,
            plural=_AUTHZ_PLURAL,
            name=policy_name,
        )
