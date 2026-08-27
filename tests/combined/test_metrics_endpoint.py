"""Smoke tests for the Prometheus /metrics scrape endpoint.

The metrics listener runs on a separate Echo instance (main.go) on METRICS_PORT
so scrape traffic bypasses Recover + logging + HTTP-metrics middleware. If it
serves a 500, wrong content-type, or omits the alatyr_* namespace, every
downstream dashboard and alert goes dark silently — the scrape still succeeds
from Prometheus's point of view, it just gets nothing useful. These smokes
guard the wiring, not individual metric values (those live in unit tests).
"""

from __future__ import annotations

import re

from libraries.constants import POLICY_NAMESPACES

# Every alatyr_* metric declared in internal/metrics/registry.go that must
# appear in a fresh scrape. Kept small on purpose — this is a smoke test, not
# a schema check. Runtime + process metrics (go_*, process_*) come from
# registered collectors, not our factory, so we assert one from each family.
_REQUIRED_ALATYR_METRICS = [
    "alatyr_build_info",
    "alatyr_engine_enabled",
    "alatyr_workloads",
    "alatyr_evaluation_timestamp_seconds",
    "alatyr_evaluation_duration_seconds",
    "alatyr_http_requests_total",
    "alatyr_http_request_duration_seconds",
    "alatyr_informer_cache_synced",
]

_REQUIRED_RUNTIME_METRICS = [
    "go_goroutines",
    "process_cpu_seconds_total",
]


def _assert_metric_present(body: str, name: str) -> None:
    # Match the declaration line (HELP/TYPE) rather than a sample so counters
    # at 0 and gauges without labels still count as present.
    pattern = rf"^# (HELP|TYPE) {re.escape(name)}\b"
    assert re.search(pattern, body, re.MULTILINE), (
        f"expected metric {name} in /metrics body; first 400 chars:\n{body[:400]}"
    )


def test_metrics_endpoint_serves_prometheus_text(get_metrics):
    response = get_metrics()

    assert response.status_code == 200, response.text
    content_type = response.headers.get("Content-Type", "")
    # promhttp negotiates text/plain or application/openmetrics-text depending
    # on Accept. Either is a valid Prometheus scrape body.
    assert "text/plain" in content_type or "openmetrics" in content_type, content_type
    body = response.text
    assert body.startswith("# HELP") or body.startswith("# TYPE"), body[:200]


def test_metrics_endpoint_exposes_alatyr_series(get_metrics, get_graph):
    # Warm /api/graph first so http_requests_total has at least one sample —
    # otherwise the counter is registered but its declaration is the only
    # evidence it exists, which _assert_metric_present already covers.
    get_graph(*POLICY_NAMESPACES)
    body = get_metrics().text

    for name in _REQUIRED_ALATYR_METRICS:
        _assert_metric_present(body, name)


def test_metrics_endpoint_exposes_runtime_collectors(get_metrics):
    body = get_metrics().text

    for name in _REQUIRED_RUNTIME_METRICS:
        _assert_metric_present(body, name)


def test_metrics_endpoint_records_http_traffic(get_metrics, get_graph):
    # Every /api/graph call must increment alatyr_http_requests_total for the
    # matched route pattern. Regression guard for the middleware wiring (main.go
    # e.Use order) — if middleware ordering breaks, this series drops to zero.
    get_graph(*POLICY_NAMESPACES)
    body = get_metrics().text

    match = re.search(
        r'^alatyr_http_requests_total\{[^}]*path="/api/graph"[^}]*\}\s+([0-9.e+-]+)',
        body,
        re.MULTILINE,
    )
    assert match, "no alatyr_http_requests_total sample for /api/graph"
    assert float(match.group(1)) >= 1.0, match.group(0)


def test_metrics_endpoint_stamps_build_info(get_metrics):
    # build_info is set once at startup; labels carry version + commit. The
    # value is always 1 — its purpose is the label set, not the number.
    body = get_metrics().text

    match = re.search(
        r"^alatyr_build_info\{[^}]*\}\s+1\b",
        body,
        re.MULTILINE,
    )
    assert match, "alatyr_build_info missing or not set to 1"
    # version + commit label keys must be present (values may be defaults).
    assert 'version="' in match.group(0), match.group(0)
    assert 'commit="' in match.group(0), match.group(0)


def test_metrics_endpoint_marks_registered_engines(get_metrics):
    # SetEnabledEngines fires at startup with the registered engine names.
    # Missing = engines silently dropped from the registry — user has no
    # dashboard signal that policies from that engine aren't being evaluated.
    body = get_metrics().text

    engine_samples = re.findall(
        r'^alatyr_engine_enabled\{engine="([^"]+)"\}\s+1\b',
        body,
        re.MULTILINE,
    )
    assert engine_samples, "no alatyr_engine_enabled=1 samples"
    assert "k8s" in engine_samples, engine_samples
    assert "istio" in engine_samples, engine_samples


def test_metrics_endpoint_handles_gzip_negotiation(get_metrics):
    # Prometheus sends Accept-Encoding: gzip on every scrape. Smoke check that
    # the endpoint responds correctly under that header — either compressed
    # (Content-Encoding: gzip, requests auto-decodes) or plain (server declined
    # negotiation). Either is fine; a crash or corrupt body is not.
    response = get_metrics(accept_encoding="gzip")

    assert response.status_code == 200
    body = response.text
    assert body.startswith("# HELP") or body.startswith("# TYPE"), body[:200]
    encoding = response.headers.get("Content-Encoding", "")
    assert encoding in ("", "gzip"), encoding
