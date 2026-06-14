"""Shared fixtures for graph E2E tests.

Session-scoped:
  - topology: 3 ns x 2 pods, created once before any test runs
  - api: single requests.Session reused across tests

Function-scoped:
  - policy_wipe (autouse): deletes every NetworkPolicy + AuthorizationPolicy
    in the test namespaces after each test, so suite-scoped pods survive
  - get_graph / get_cluster_state: HTTP helpers bound to the api session
"""

from __future__ import annotations

from typing import Callable

import pytest
import requests

from libraries import cluster
from libraries.constants import (
    AMBIENT_LABEL_KEY,
    AMBIENT_LABEL_VALUE,
    BACKEND_URL,
    NS_MESH,
    POLICY_NAMESPACES,
    ROOT_NS,
    TEST_NAMESPACES,
)


@pytest.fixture(scope="session")
def topology():
    for namespace in TEST_NAMESPACES:
        cluster.create_namespace(namespace)
        cluster.create_pod(namespace, "frontend", {"app": "frontend"})
        cluster.create_pod(namespace, "backend", {"app": "backend"})
    # Ambient-enrolled ns: ns-label opts every workload into the mesh, so mtls
    # resolves and ValidateExternalRules runs.
    cluster.create_namespace(NS_MESH, labels={AMBIENT_LABEL_KEY: AMBIENT_LABEL_VALUE})
    cluster.create_pod(NS_MESH, "frontend", {"app": "frontend"})
    cluster.create_pod(NS_MESH, "backend", {"app": "backend"})
    # Mesh root ns holds mesh-scoped (selectorless) PeerAuthentications. No pods;
    # created here because kwok ships no istio install. create_namespace tolerates
    # 409 so a pre-existing istio-system is fine.
    cluster.create_namespace(ROOT_NS)
    yield
    for namespace in TEST_NAMESPACES + [NS_MESH]:
        cluster.delete_namespace(namespace)


@pytest.fixture(autouse=True)
def policy_wipe(topology):
    # autouse → every test gets clean policies, no need to opt in.
    # Includes the mesh ns + root ns (istio-system) so PeerAuthentications
    # don't leak across tests.
    yield
    cluster.delete_all_policies_in(*POLICY_NAMESPACES)


@pytest.fixture(scope="session")
def api() -> requests.Session:
    session = requests.Session()
    yield session
    session.close()


@pytest.fixture
def get_graph(api) -> Callable[..., dict]:
    def _get(*namespaces: str) -> dict:
        params = {"namespaces": ",".join(namespaces)} if namespaces else None
        response = api.get(f"{BACKEND_URL}/api/graph", params=params)
        response.raise_for_status()
        return response.json()
    return _get


@pytest.fixture
def get_cluster_state(api) -> Callable[[], dict]:
    def _get() -> dict:
        response = api.get(f"{BACKEND_URL}/api/cluster-state")
        response.raise_for_status()
        return response.json()
    return _get


@pytest.fixture
def get_node_info(api) -> Callable[..., dict]:
    """Hit /api/node-info for one workload.

    Returns the per-engine map: `{engineName: {rules: [...], policies: [...]}}`.
    Engines that have no rules AND no selecting policies for the node are
    omitted from the response. Reads from the cache populated by /api/graph.
    """
    def _get(node_id: str, namespace: str) -> dict:
        response = api.get(
            f"{BACKEND_URL}/api/node-info",
            params={"nodeId": node_id, "namespace": namespace},
        )
        response.raise_for_status()
        return response.json()
    return _get


@pytest.fixture
def get_reachability(api) -> Callable[..., dict]:
    """Hit /api/reachable for one src→dst pair.

    Reachability reads from the cache populated by /api/graph, so callers
    must request the graph for the relevant namespaces first.
    """
    def _get(src_id: str, src_ns: str, dst_id: str, dst_ns: str) -> dict:
        response = api.get(
            f"{BACKEND_URL}/api/reachable",
            params={"srcId": src_id, "srcNs": src_ns, "dstId": dst_id, "dstNs": dst_ns},
        )
        response.raise_for_status()
        return response.json()
    return _get
