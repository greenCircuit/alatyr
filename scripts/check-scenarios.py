#!/usr/bin/env python3
"""Assert every demo scenario still produces its finding.

Reads the same registry the UI reads (ui/src/data/scenarios.json) and checks
each scenario's `target` against a running DEMO_MODE backend: the issue type
still fires, and — when the scenario names a workload — it still fires on that
workload. Scenarios with no target (dashboard entry points) only need their
namespaces to exist.

A scenario that silently stops producing its finding is worse than no demo,
because the caption keeps asserting something the panel no longer shows. Run
this in the same job that snapshots the API, before the snapshot is published.
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

DEFAULT_REGISTRY = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
    "ui", "src", "data", "scenarios.json",
)


def get_json(base_url, path):
    with urllib.request.urlopen(base_url + path, timeout=60) as response:
        return json.load(response)


# An issue counts as touching a workload if it names it on either endpoint or as
# the node-scoped subject — the UI resolves targets the same way
# (ui/src/store/scenario.ts).
def issue_touches(issue, workload):
    for key in ("src", "dst", "node"):
        node = issue.get(key)
        if node and node.get("namespace") == workload["namespace"] \
                and node.get("label") == workload["label"]:
            return True
    return False


# Returns a failure string, or None when the scenario still holds.
def check_scenario(scenario, issues, namespaces):
    missing_ns = [ns for ns in scenario.get("namespaces", []) if ns not in namespaces]
    if missing_ns:
        return f"namespace(s) not in cluster: {', '.join(missing_ns)}"

    target = scenario.get("target")
    if not target:
        return None

    issue_type = target.get("issueType")
    if not issue_type:
        return None

    of_type = [issue for issue in issues if issue.get("type") == issue_type]
    if not of_type:
        return f"no issue of type {issue_type!r} fires anywhere in the demo cluster"

    workload = target.get("workload")
    if workload and not any(issue_touches(issue, workload) for issue in of_type):
        named = f"{workload['namespace']}/{workload['label']}"
        return (f"issue {issue_type!r} fires, but not on {named} — "
                f"the caption names a workload the finding no longer touches")
    return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--registry", default=DEFAULT_REGISTRY)
    args = parser.parse_args()

    with open(args.registry, encoding="utf-8") as handle:
        scenarios = json.load(handle)

    try:
        # /api/issues reads the server's graph cache and returns an empty list
        # until something populates it. Warm it first or every scenario reports
        # a false regression.
        get_json(args.base_url, "/api/graph")
        issues = get_json(args.base_url, "/api/issues") or []
        cluster_state = get_json(args.base_url, "/api/cluster-state")
    except (urllib.error.URLError, OSError) as error:
        print(f"backend unreachable at {args.base_url}: {error}", file=sys.stderr)
        return 2

    namespaces = set(cluster_state.get("availableNs") or [])

    failures = []
    for scenario in scenarios:
        problem = check_scenario(scenario, issues, namespaces)
        label = scenario["id"]
        if problem:
            failures.append(f"{label}: {problem}")
            print(f"FAIL {label} — {problem}")
        else:
            print(f"ok   {label}")

    if failures:
        print(f"\n{len(failures)} scenario(s) no longer hold; "
              f"fix the fixture or the registry before publishing the demo",
              file=sys.stderr)
        return 1
    print(f"\nall {len(scenarios)} scenarios hold")
    return 0


if __name__ == "__main__":
    sys.exit(main())
