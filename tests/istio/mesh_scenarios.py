"""Istio PeerAuthentication scenarios for mesh / mTLS tests.

Each entry is a list of (namespace, PeerAuthenticationSpec) pairs. apply(name)
serialises each spec to the wire format and applies it. Mirrors scenarios.py
(AuthorizationPolicy), but drives mesh-layer resolution + issue detection
surfaced by /api/node-info.
"""

from __future__ import annotations

from libraries import cluster
from libraries.constants import NS_MESH, ROOT_NS
from libraries.istio_models import Selector
from libraries.peer_auth_models import Mtls, MtlsMode, PeerAuthenticationSpec

_BACKEND = Selector(matchLabels={"app": "backend"})


def _ns(mode: MtlsMode) -> PeerAuthenticationSpec:
    """ns-scoped PA: no selector → applies to every workload in the ns."""
    return PeerAuthenticationSpec(mtls=Mtls(mode=mode))


SCENARIOS: dict[str, list[tuple[str, PeerAuthenticationSpec]]] = {
    # ── verdict resolution (ns scope) ─────────────────────────────────────────
    "ns_strict":     [(NS_MESH, _ns(MtlsMode.STRICT))],
    "ns_disable":    [(NS_MESH, _ns(MtlsMode.DISABLE))],
    "ns_permissive": [(NS_MESH, _ns(MtlsMode.PERMISSIVE))],

    # every matching PA is UNSET → install-default (permissive) + all-unset issue
    "ns_unset":      [(NS_MESH, _ns(MtlsMode.UNSET))],

    # precedence: workload-selector STRICT beats ns-scoped PERMISSIVE
    "workload_overrides_ns": [
        (NS_MESH, _ns(MtlsMode.PERMISSIVE)),
        (NS_MESH, PeerAuthenticationSpec(selector=_BACKEND, mtls=Mtls(mode=MtlsMode.STRICT))),
    ],

    # mesh scope: selectorless root-ns PA applies cluster-wide
    "mesh_strict":   [(ROOT_NS, _ns(MtlsMode.STRICT))],

    # per-port override: workload STRICT with port 8080 DISABLE
    "port_override": [
        (NS_MESH, PeerAuthenticationSpec(
            selector=_BACKEND,
            mtls=Mtls(mode=MtlsMode.STRICT),
            portLevelMtls={8080: Mtls(mode=MtlsMode.DISABLE)},
        )),
    ],

    # ── issue detection ───────────────────────────────────────────────────────

    # two ns-scoped PAs → "duplicate at scope" (oldest wins, rest ignored)
    "dup_at_ns": [
        (NS_MESH, _ns(MtlsMode.STRICT)),
        (NS_MESH, _ns(MtlsMode.PERMISSIVE)),
    ],

    # root-ns PA with selector → "root selector ignored"
    "root_selector_ignored": [
        (ROOT_NS, PeerAuthenticationSpec(selector=_BACKEND, mtls=Mtls(mode=MtlsMode.STRICT))),
    ],

    # ns-scoped PA (no selector) with portLevelMtls → "port level without selector"
    "port_without_selector": [
        (NS_MESH, PeerAuthenticationSpec(
            mtls=Mtls(mode=MtlsMode.STRICT),
            portLevelMtls={8080: Mtls(mode=MtlsMode.DISABLE)},
        )),
    ],
}


def apply(name: str) -> None:
    if name not in SCENARIOS:
        raise KeyError(f"unknown mesh scenario {name!r}; known: {sorted(SCENARIOS)}")
    policy_name = name.replace("_", "-")
    for index, (namespace, spec) in enumerate(SCENARIOS[name]):
        cluster.istio.apply_pa(namespace, {
            "metadata": {"name": f"{policy_name}-{index}"},
            "spec": spec.model_dump(by_alias=True, exclude_none=True, mode="json"),
        })
