"""Istio AuthorizationPolicy scenarios.

Each entry is a list of (namespace, AuthorizationPolicySpec) pairs.
apply(name) serialises each spec to the wire format and applies it.
"""

from __future__ import annotations

from libraries import cluster
from libraries.constants import NS_A, NS_B
from libraries.istio_models import (
    Action,
    AuthorizationPolicySpec,
    FromItem,
    Operation,
    Rule,
    Selector,
    Source,
    ToItem,
)

_BACKEND = Selector(matchLabels={"app": "backend"})

SCENARIOS: dict[str, list[tuple[str, AuthorizationPolicySpec]]] = {
    # ── edge tests ────────────────────────────────────────────────────────────
    "allow_b_to_backend_in_a": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule(**{"from": [FromItem(source=Source(namespaces=[NS_B]))],
                           "to":   [ToItem(operation=Operation(ports=["8080"]))]})],
        )),
    ],

    # ── status key matrix ─────────────────────────────────────────────────────

    # deny-all short-circuit: empty rule → ruleMatchesAnySource → IngressLocked only
    "deny_all_backend": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.DENY,
            rules=[Rule()],
        )),
    ],

    # accumulator: own-ns source → InnerNsIngress
    "allow_from_same_ns": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule(**{"from": [FromItem(source=Source(namespaces=[NS_A]))]})],
        )),
    ],

    # accumulator: foreign-ns source → CrossNS
    "allow_from_other_ns": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule(**{"from": [FromItem(source=Source(namespaces=[NS_B]))]})],
        )),
    ],

    # accumulator: ipBlock 0.0.0.0/0 → InternetIngress → both directions open
    "allow_internet_ingress": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule(**{"from": [FromItem(source=Source(ipBlocks=["0.0.0.0/0"]))]})],
        )),
    ],

    # accumulator: private CIDR → LanIngress
    "allow_lan_ingress": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule(**{"from": [FromItem(source=Source(ipBlocks=["10.0.0.0/8"]))]})],
        )),
    ],

    # allow-all short-circuit: empty rule → IngressLocked=false, InnerNsIngress+CrossNS=true
    "allow_any_source": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule()],
        )),
    ],

    # accumulator: cross-ns + to.operation.paths → HasL7
    "allow_with_l7": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule(**{"from": [FromItem(source=Source(namespaces=[NS_B]))],
                           "to":   [ToItem(operation=Operation(paths=["/api/*"]))]})],
        )),
    ],

    # subtraction: ALLOW ns-b − DENY ns-b = no survivors → IngressLocked only
    "allow_deny_subtraction": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule(**{"from": [FromItem(source=Source(namespaces=[NS_B]))]})],
        )),
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.DENY,
            rules=[Rule(**{"from": [FromItem(source=Source(namespaces=[NS_B]))]})],
        )),
    ],

    # explicit DENY from foreign ns → produces a DENY-action edge
    "deny_from_other_ns": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.DENY,
            rules=[Rule(**{"from": [FromItem(source=Source(namespaces=[NS_B]))]})],
        )),
    ],

    # notNamespaces wildcard → over-claims InnerNsIngress + CrossNS
    "allow_not_namespaces": [
        (NS_A, AuthorizationPolicySpec(
            selector=_BACKEND,
            action=Action.ALLOW,
            rules=[Rule(**{"from": [FromItem(source=Source(notNamespaces=["ns-evil"]))]})],
        )),
    ],
}

def apply(name: str) -> None:
    if name not in SCENARIOS:
        raise KeyError(f"unknown istio scenario {name!r}; known: {sorted(SCENARIOS)}")
    policy_name = name.replace("_", "-")
    for index, (namespace, spec) in enumerate(SCENARIOS[name]):
        cluster.istio.apply(namespace, {
            "metadata": {"name": f"{policy_name}-{index}"},
            "spec": spec.model_dump(by_alias=True, exclude_none=True, mode="json"),
        })
