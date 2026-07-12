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

import time
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

# Informer cache budgets. Backend WATCH events propagate in ms, but tests can
# race both the prior test's DELETE and their own CREATE events. Both bounds
# are polled, so they cap the wait — they aren't sleeps.
_INFORMER_DRAIN_TIMEOUT = 5.0
_INFORMER_DRAIN_POLL = 0.05
# stable-fetch: return once two consecutive responses are identical. Catches
# the apply() → get_graph() race where the WATCH ADDED event for a freshly
# created policy hasn't reached the cache yet.
_STABLE_FETCH_TIMEOUT = 3.0
_STABLE_FETCH_POLL = 0.05


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
def policy_wipe(topology, api):
    # autouse → every test gets clean policies, no need to opt in.
    # Wipe runs BEFORE the test, then we block until the backend's informer
    # cache reflects the empty state (no edges across POLICY_NAMESPACES).
    # Wiping post-yield isn't enough on its own: the next test's apply()
    # can race the prior test's DELETE event, leaving stale policies in the
    # cache and producing extra status keys. Covers mesh ns + istio-system
    # so PeerAuthentications don't leak either.
    cluster.delete_all_policies_in(*POLICY_NAMESPACES)
    _wait_until_no_edges(api)
    yield


def _wait_until_no_edges(api: requests.Session) -> None:
    deadline = time.monotonic() + _INFORMER_DRAIN_TIMEOUT
    params = {"namespaces": ",".join(POLICY_NAMESPACES)}
    while time.monotonic() < deadline:
        response = api.get(f"{BACKEND_URL}/api/graph", params=params)
        response.raise_for_status()
        if not response.json().get("edges"):
            return
        time.sleep(_INFORMER_DRAIN_POLL)
    raise RuntimeError(
        f"informer cache still has edges after {_INFORMER_DRAIN_TIMEOUT}s — "
        "prior test's policies didn't drain"
    )


@pytest.fixture(scope="session")
def api() -> requests.Session:
    session = requests.Session()
    yield session
    session.close()


@pytest.fixture
def get_graph(api) -> Callable[..., dict]:
    def _get(*namespaces: str) -> dict:
        params = {"namespaces": ",".join(namespaces)} if namespaces else None
        return _stable_get(api, "/api/graph", params)
    return _get


def _stable_get(api: requests.Session, path: str, params: dict | None) -> dict:
    """Poll the endpoint until two consecutive responses are identical, then
    return. Absorbs the informer-cache race between a just-applied policy and
    the test's first read. Falls back to the last response on timeout so a
    truly mismatched cache surfaces as the real assertion failure, not a
    timeout."""
    deadline = time.monotonic() + _STABLE_FETCH_TIMEOUT
    last: dict | None = None
    while time.monotonic() < deadline:
        response = api.get(f"{BACKEND_URL}{path}", params=params)
        response.raise_for_status()
        body = response.json()
        if last is not None and body == last:
            return body
        last = body
        time.sleep(_STABLE_FETCH_POLL)
    assert last is not None
    return last


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

    Returns the full detail body: `{neighbors: {engineName: {In, Out}}, mesh?,
    issues?}`. `neighbors` is per-engine adjacency — In = node is the rule
    destination, Out = node is the source. Every registered engine gets a key
    (empty In/Out = no neighbors, not an absent key). Reads from the cache
    populated by /api/graph.
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


@pytest.fixture
def get_issues(api) -> Callable[[], list[dict]]:
    """Hit /api/issues. Whole-cluster scan for policy/mesh conflicts +
    missing DNS. Reads from the cache populated by /api/graph, so callers
    must request the graph for the relevant namespaces first.
    """
    def _get() -> list[dict]:
        response = api.get(f"{BACKEND_URL}/api/issues")
        response.raise_for_status()
        return response.json() or []
    return _get


@pytest.fixture
def get_manifest(api) -> Callable[..., requests.Response]:
    """Hit /api/manifest for one policy object. Returns the raw Response so
    callers can assert on status (200 vs 400) — manifest fetches straight from
    the apiserver, not the graph cache, so error paths are part of the contract.
    """
    def _get(kind: str, namespace: str, name: str) -> requests.Response:
        return api.get(
            f"{BACKEND_URL}/api/manifest",
            params={"kind": kind, "namespace": namespace, "name": name},
        )
    return _get
