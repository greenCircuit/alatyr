"""Initial-load latency sweeps.

Measures whole-request latency of GET /api/graph (the initial graph load the
user feels) while growing one dimension at a time:

  test_ns_grows         vary namespace count   (fixed pods + policies per ns)
  test_workloads_grow   vary pods per ns       (single namespace)
  test_policy_grows     vary policies per ns   (single namespace)

One independent variable per test → each curve is attributable to that axis.
For each case it records cold_ms (first request) and warm_p50_ms (median of
PERF_WARM_ITERS repeats). Today both grow ~linearly; after informers warm
should flatten.

Opt-in: pytest -m perf
"""

from __future__ import annotations

import statistics
import time

import pytest

from . import topology
from .conftest import ns_sizes

# Per-axis sweep points. ns count comes from conftest (env-overridable); the
# density axes are local since they're the variable under study.
WORKLOAD_STEPS = [5, 25, 59, 100]
POLICY_STEPS = [1, 5, 10, 20]


def _time_graph(get_graph, namespaces: list[str]) -> tuple[float, dict]:
    start = time.perf_counter()
    graph = get_graph(*namespaces)
    elapsed_ms = (time.perf_counter() - start) * 1000.0
    return elapsed_ms, graph


def _record(size, pods_per_ns, policies_per_engine, warm_iters, results, get_graph):
    """Build the topology, time cold + warm, append one results row.

    Shared by every sweep — only the varied axis differs at the call site.
    """
    namespaces = topology.build(size, pods_per_ns, policies_per_engine)

    cold_ms, graph = _time_graph(get_graph, namespaces)
    warm_samples = [_time_graph(get_graph, namespaces)[0] for _ in range(warm_iters)]
    warm_p50 = statistics.median(warm_samples)

    results.append(
        {
            "namespaces": size,
            "pods_per_ns": pods_per_ns,
            "policies_per_engine": policies_per_engine,
            "nodes": len(graph.get("nodes") or []),
            "edges": len(graph.get("edges") or []),
            "cold_ms": round(cold_ms, 1),
            "warm_p50_ms": round(warm_p50, 1),
        }
    )

    # Sanity only — not a latency gate. A fast-but-empty graph can't pass.
    assert len(graph.get("nodes") or []) >= size


@pytest.mark.perf
@pytest.mark.parametrize("size", ns_sizes())
def test_ns_grows(size, pods_per_ns, policies_per_engine, warm_iters, results, get_graph):
    _record(size, pods_per_ns, policies_per_engine, warm_iters, results, get_graph)


@pytest.mark.perf
@pytest.mark.parametrize("pods", WORKLOAD_STEPS)
def test_workloads_grow(pods, policies_per_engine, warm_iters, results, get_graph):
    _record(1, pods, policies_per_engine, warm_iters, results, get_graph)


@pytest.mark.perf
@pytest.mark.parametrize("policies", POLICY_STEPS)
def test_policy_grows(policies, pods_per_ns, warm_iters, results, get_graph):
    _record(1, pods_per_ns, policies, warm_iters, results, get_graph)
