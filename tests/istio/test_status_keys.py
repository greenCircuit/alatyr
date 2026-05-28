"""Status-key matrix for the Istio engine.

Each parametrize row exercises a distinct code path in buildPolicyStatus.go.
Assertions read statusesBySource["istio"] so results are engine-isolated.
"""

import pytest

from libraries.constants import NS_A
from libraries.graph_api import node_statuses_by_source

from . import scenarios


@pytest.mark.parametrize(
    "scenario, expected_keys",
    [
        # deny-all short-circuit — IngressLocked only; egress unlocked → internet-egress
        pytest.param("deny_all_backend",       ["internet-egress"],                                         id="deny_all"),
        # accumulator: own-ns source → InnerNsIngress
        pytest.param("allow_from_same_ns",     ["internet-egress", "ns-ingress-access"],                   id="allow_same_ns"),
        # accumulator: foreign-ns source → CrossNS
        pytest.param("allow_from_other_ns",    ["internet-egress", "cross-namespace"],                     id="allow_cross_ns"),
        # accumulator: 0.0.0.0/0 ipBlock → InternetIngress; both directions open → internet-full
        pytest.param("allow_internet_ingress", ["internet-full"],                                           id="allow_internet"),
        # accumulator: private CIDR ipBlock → LanIngress
        pytest.param("allow_lan_ingress",      ["internet-egress", "lan-ingress"],                         id="allow_lan"),
        # allow-all short-circuit — IngressLocked=false; InnerNsIngress+CrossNS set
        pytest.param("allow_any_source",       ["internet-full", "cross-namespace", "ns-ingress-access"],  id="allow_any"),
        # accumulator: cross-ns + to.operation.paths → HasL7
        pytest.param("allow_with_l7",          ["internet-egress", "cross-namespace", "l7-applied"],       id="allow_l7"),
        # subtraction: ALLOW ns-b − DENY ns-b = no survivors → IngressLocked only
        pytest.param("allow_deny_subtraction", ["internet-egress"],                                         id="subtraction"),
        # notNamespaces wildcard → over-claims InnerNsIngress + CrossNS
        pytest.param("allow_not_namespaces",   ["internet-egress", "ns-ingress-access", "cross-namespace"], id="not_namespaces"),
    ],
)
def test_backend_istio_status_keys(scenario, expected_keys, get_graph):
    scenarios.apply(scenario)
    graph = get_graph(NS_A)
    actual = node_statuses_by_source(graph, "backend", NS_A, "istio")
    assert set(expected_keys) == set(actual)
