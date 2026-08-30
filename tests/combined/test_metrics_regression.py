"""Regression tests for /metrics values — the layer above smoke.

Smoke tests (test_metrics_endpoint.py) only prove the endpoint serves declared
metric names. These tests prove the values change correctly when the cluster
state changes: a scrape that reads clean while the UI shows the problem is the
exact failure mode that made the metrics unit tests worth writing.

Every test warms /api/graph so the snapshot loop has run before the scrape.
"""

from __future__ import annotations

import re
import time
from typing import Iterable

import pytest

from k8s import scenarios as k8s_scenarios
from libraries import cluster
from libraries.constants import NS_A, POLICY_NAMESPACES


# ---------- Prometheus text parsing helpers ----------
#
# Full prometheus-client parser isn't in requirements.txt on purpose — the
# regex here is scoped to the two operations these tests need: pull one sample
# by name+labels, and iterate every sample of one family. Anything richer
# belongs in a dedicated dependency, not shoe-horned in.


_LABEL_TOKEN = r'([^=,{}]+)="((?:\\.|[^"\\])*)"'


def _parse_labels(label_body: str) -> dict[str, str]:
    """Extract {k: v} from the body inside a Prometheus label brace block.
    Handles escaped quotes; values containing = or , are not expected on any
    alatyr_* series and would blow up the label-cardinality budget anyway."""
    return {name: value for name, value in re.findall(_LABEL_TOKEN, label_body)}


def _iter_samples(body: str, name: str) -> Iterable[tuple[dict[str, str], float]]:
    """Yield (labels, value) for every sample of one metric family.
    Skips histogram-derived _bucket/_count/_sum series — those need dedicated
    assertions, not this helper.
    """
    pattern = re.compile(
        rf"^{re.escape(name)}(?:\{{([^}}]*)\}})?\s+([0-9eE+\-.NaN]+)",
        re.MULTILINE,
    )
    for match in pattern.finditer(body):
        label_body = match.group(1) or ""
        value = match.group(2)
        try:
            yield _parse_labels(label_body), float(value)
        except ValueError:
            continue


def _sample_value(body: str, name: str, **labels: str) -> float | None:
    """Return the value of the sample whose label set is a superset of
    `labels`. None when no series matches — tests distinguish "missing" from
    "0" because a missing coverage series is the silent-failure symptom."""
    for sample_labels, value in _iter_samples(body, name):
        if all(sample_labels.get(k) == v for k, v in labels.items()):
            return value
    return None


def _series_count(body: str, name: str) -> int:
    return sum(1 for _ in _iter_samples(body, name))


# ---------- fixture-shared warm-up ----------


def _warm(get_graph, get_metrics, *namespaces: str) -> str:
    """Warm /api/graph so the snapshot loop has covered the requested
    namespaces, then return the /metrics body. Callers assert against the
    body directly.
    """
    get_graph(*namespaces)
    # cacheRefreshSec = 1s in tests; RecordSnapshot runs on that tick. Give
    # one full cycle so gauges reflect the just-warmed state before scraping.
    time.sleep(1.5)
    return get_metrics().text


# ---------- value-shift ----------


def test_workloads_covered_moves_after_policy_apply(get_graph, get_metrics):
    """Apply a k8s NetworkPolicy that selects one workload → the k8s coverage
    gauge for NS_A must move from 0 to at least 1. Regression guard for the
    full pipeline: informer → cache → RecordSnapshot → GaugeVec → wire.
    """
    baseline = _warm(get_graph, get_metrics, *POLICY_NAMESPACES)
    baseline_covered = _sample_value(
        baseline, "alatyr_workloads_covered", namespace=NS_A, engine="k8s"
    ) or 0.0

    # allow_fe_to_be selects the backend workload in NS_A via podSelector.
    k8s_scenarios.apply("allow_fe_to_be")
    after = _warm(get_graph, get_metrics, *POLICY_NAMESPACES)
    after_covered = _sample_value(
        after, "alatyr_workloads_covered", namespace=NS_A, engine="k8s"
    )

    assert after_covered is not None, (
        "alatyr_workloads_covered{namespace=NS_A,engine=k8s} absent after "
        "apply — RecordSnapshot did not populate coverage"
    )
    assert after_covered - baseline_covered >= 1, (
        f"coverage did not shift: baseline={baseline_covered}, after={after_covered}"
    )


def test_policies_count_moves_after_apply(get_graph, get_metrics):
    """alatyr_policies{namespace=NS_A,engine=k8s} increments by 1 after one
    NetworkPolicy applies. Split from coverage: this proves the manifest
    dedup path (recordPolicies) fires, coverage proves the NodePolicies path.
    """
    baseline = _warm(get_graph, get_metrics, *POLICY_NAMESPACES)
    baseline_count = _sample_value(
        baseline, "alatyr_policies", namespace=NS_A, engine="k8s"
    ) or 0.0

    k8s_scenarios.apply("deny_all_backend")
    after = _warm(get_graph, get_metrics, *POLICY_NAMESPACES)
    after_count = _sample_value(
        after, "alatyr_policies", namespace=NS_A, engine="k8s"
    )

    assert after_count is not None, "alatyr_policies absent after apply"
    assert after_count - baseline_count >= 1, (
        f"policies gauge did not shift: baseline={baseline_count}, after={after_count}"
    )


# ---------- reset-over-the-wire ----------


def test_stale_policy_series_drop_after_wipe(get_graph, get_metrics):
    """Apply → scrape (series present) → delete → scrape (series absent OR 0).
    Pins the reset-then-repopulate contract at the wire, not just at the unit
    level. If resetSnapshotVecs stops running, the just-deleted policy's
    label combo lingers on the gauge at its last value forever — dashboards
    keep counting a policy that no longer exists.
    """
    k8s_scenarios.apply("deny_all_backend")
    applied_body = _warm(get_graph, get_metrics, *POLICY_NAMESPACES)
    applied_count = _sample_value(applied_body, "alatyr_policies", namespace=NS_A, engine="k8s")
    assert applied_count is not None and applied_count >= 1, (
        f"post-apply policy count = {applied_count}, expected >= 1"
    )

    cluster.delete_all_policies_in(*POLICY_NAMESPACES)
    # Two full snapshot ticks: one for the DELETE to reach the informer, one
    # for RecordSnapshot to observe an empty NodePolicies map.
    time.sleep(3.0)
    after_body = _warm(get_graph, get_metrics, *POLICY_NAMESPACES)
    after_count = _sample_value(after_body, "alatyr_policies", namespace=NS_A, engine="k8s")

    # Reset() drops the label combo entirely — missing series is the pass
    # signal. A lingering 0 still passes because downstream sum() reads it
    # correctly, but a non-zero value proves the reset regressed.
    assert after_count in (None, 0.0), (
        f"stale alatyr_policies series survived wipe: {after_count}"
    )


# ---------- freshness ----------


def test_evaluation_timestamp_advances_between_scrapes(get_graph, get_metrics):
    """cacheRefreshSec = 1s in tests, so two scrapes ~2s apart must see the
    evaluation_timestamp_seconds gauge advance. A stuck timestamp is the
    silent-death signal — the process is up, the endpoint serves, but the
    snapshot loop crashed and no dashboard alert fires until age(timestamp)
    exceeds its threshold.
    """
    get_graph(*POLICY_NAMESPACES)
    first_body = get_metrics().text
    first_ts = _sample_value(first_body, "alatyr_evaluation_timestamp_seconds")
    assert first_ts is not None and first_ts > 0, "evaluation_timestamp not stamped"

    time.sleep(2.5)
    second_body = get_metrics().text
    second_ts = _sample_value(second_body, "alatyr_evaluation_timestamp_seconds")
    assert second_ts is not None, "evaluation_timestamp missing on second scrape"
    assert second_ts >= first_ts, (
        f"evaluation_timestamp regressed: first={first_ts}, second={second_ts}"
    )
    # Snapshot loop runs at 1Hz — a 2.5s window must produce at least one
    # advance. Equal timestamps means RecordSnapshot never ran.
    assert second_ts > first_ts, (
        f"evaluation_timestamp did not advance across 2.5s window: "
        f"first={first_ts}, second={second_ts} — snapshot loop stalled?"
    )


# ---------- informer sync ----------


def test_informer_cache_synced_reports_one_per_gvk(get_metrics):
    """Every GVK the backend watches must land at value 1 shortly after
    startup. Value 0 is exactly the "trust broken" signal — informer never
    synced, /api/graph is returning stale or empty data.
    """
    body = get_metrics().text
    samples = list(_iter_samples(body, "alatyr_informer_cache_synced"))
    assert samples, "alatyr_informer_cache_synced has zero samples — SetInformerSynced never called"
    for labels, value in samples:
        assert value == 1.0, (
            f"informer_cache_synced{labels} = {value}, want 1 — "
            "backend running against unsynced cache"
        )


# ---------- cardinality ceiling ----------


# Per-namespace gauges expected to stay bounded by (namespaces × small const).
# Baseline topology = 3 test namespaces + mesh + istio-system + a handful of
# default-cluster namespaces = <20 namespaces. Times a per-metric constant.
# A PR adding a workload label to a base gauge would blow past this ceiling.
_CARDINALITY_CEILING = {
    "alatyr_workloads": 40,
    "alatyr_workloads_unpoliced": 40,
    "alatyr_workloads_unpoliced_excluding_global": 40,
    "alatyr_engine_enabled": 8,
    "alatyr_mesh_namespaces_partially_enrolled": 4,
}


@pytest.mark.parametrize("name, ceiling", sorted(_CARDINALITY_CEILING.items()))
def test_metric_series_count_under_ceiling(get_graph, get_metrics, name, ceiling):
    """One test row per base gauge. Fails loudly when a PR adds a label to
    the metric definition and inflates cardinality per-workload or per-pod.
    Ceilings are deliberately loose — the failure mode is 10x growth, not
    2x.
    """
    body = _warm(get_graph, get_metrics, *POLICY_NAMESPACES)
    count = _series_count(body, name)
    assert count <= ceiling, (
        f"{name} has {count} series, ceiling {ceiling} — "
        "did a new label get added to the metric definition?"
    )


def test_total_alatyr_series_under_ceiling(get_graph, get_metrics):
    """Whole-body ceiling: catches cardinality inflation that doesn't map to
    a single named family (e.g. a new metric that fans across every workload).
    Loose bound based on baseline topology; tighten as the metric surface
    stabilises.
    """
    body = _warm(get_graph, get_metrics, *POLICY_NAMESPACES)
    # Count alatyr_* sample lines only — go_*/process_* are runtime metrics
    # whose cardinality is bounded by the Go runtime, not our code.
    sample_lines = re.findall(r"^alatyr_[a-z_]+", body, re.MULTILINE)
    assert len(sample_lines) < 1000, (
        f"/metrics carries {len(sample_lines)} alatyr_* sample lines — "
        "cardinality explosion likely"
    )
