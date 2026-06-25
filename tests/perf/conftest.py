"""Perf-suite fixtures: sweep config from env + a results collector that writes
a CSV and prints a summary table once the whole sweep finishes.

Config (all optional):
  PERF_NS_SIZES              comma list of namespace counts to sweep (default 1, 5, 10, 20)
  PERF_PODS_PER_NS           pods per namespace                      (default 6)
  PERF_POLICIES_PER_ENGINE   policies per engine per namespace       (default 6)
  PERF_WARM_ITERS            warm repeats after the cold call        (default 5)
  PERF_KEEP                  keep the topology after the run (1/true)(default off)
"""

from __future__ import annotations

import csv
import os
from pathlib import Path

import pytest

from . import topology


def _int_env(name: str, default: int) -> int:
    raw = os.environ.get(name)
    return int(raw) if raw else default


def ns_sizes() -> list[int]:
    raw = os.environ.get("PERF_NS_SIZES", "1,5,10,20")
    return [int(part) for part in raw.split(",") if part.strip()]


@pytest.fixture(scope="session")
def pods_per_ns() -> int:
    return _int_env("PERF_PODS_PER_NS", 5)


@pytest.fixture(scope="session")
def policies_per_engine() -> int:
    return _int_env("PERF_POLICIES_PER_ENGINE", 5)


@pytest.fixture(scope="session")
def warm_iters() -> int:
    return _int_env("PERF_WARM_ITERS", 5)


@pytest.fixture(scope="session")
def results():
    """Collect one row per size, write CSV + print a table on teardown."""
    rows: list[dict] = []
    yield rows

    if not rows:
        return
    out_dir = Path(os.environ.get("PERF_RESULTS_DIR", ".cache"))
    out_dir.mkdir(parents=True, exist_ok=True)
    out_file = out_dir / "perf-initial-load.csv"
    fields = list(rows[0].keys())
    with out_file.open("w", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)

    header = "  ".join(f"{name:>14}" for name in fields)
    print(f"\n\n=== initial-load sweep ===\n{header}")
    for row in rows:
        print("  ".join(f"{str(row[name]):>14}" for name in fields))
    print(f"\nwrote {out_file}\n")


@pytest.fixture(scope="session", autouse=True)
def _cleanup_topology():
    yield
    if os.environ.get("PERF_KEEP", "").lower() in ("1", "true", "yes"):
        return
    topology.teardown_all()
