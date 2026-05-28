"""Status-key matrix for the k8s engine.

Each parametrize row = one independent test case. Assertions read the
k8s engine's per-engine status (statusesBySource["k8s"]) so results don't
depend on what other engines emit for the same node.
"""

import pytest

from libraries.constants import NS_A
from libraries.graph_api import node_statuses_by_source

from . import scenarios


@pytest.mark.parametrize(
    "scenario, expected_keys",
    [
        pytest.param(
            "deny_all_backend",
            ["air-gapped"],
        ),
        pytest.param(
            "allow_fe_to_be",
            ["internet-egress"],
        ),
        pytest.param(
            "pod_ns_egress",
            ["internet-ingress", "cross-namespace"],
        ),
        pytest.param(
            "allow_internet_egress",
            ["internet-full"],
        ),
        pytest.param(
            "allow_internet_ingress",
            ["internet-full"],
        ),
        pytest.param(
            "deny_egress_only",
            ["internet-ingress"],
        ),
    ],
)
def test_backend_k8s_status_keys(scenario, expected_keys, get_graph):
    scenarios.apply(scenario)
    graph = get_graph(NS_A)
    actual = node_statuses_by_source(graph, "backend", NS_A, "k8s")
    assert set(expected_keys) == set(actual)
