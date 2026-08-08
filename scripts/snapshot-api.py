#!/usr/bin/env python3
"""Freeze the demo-mode API into static JSON for GitHub Pages.

Boots nothing itself — expects a backend already running with DEMO_MODE=true.
Crawls every endpoint the UI calls and writes the responses under
ui/public/demo/. Parameterized endpoints (node-info, reachable, manifest)
collapse into one lookup map per endpoint, keyed by their query params, so the
static frontend resolves them client-side without one file per combination.
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

import yaml

# Fixture kind -> the `kind` query param the manifest endpoint expects
# (PolicySource name, not the k8s Kind).
MANIFEST_KIND_BY_FIXTURE_KIND = {
    "NetworkPolicy": "k8s",
    "AuthorizationPolicy": "istio",
    "PeerAuthentication": "pa",
    "GlobalNetworkPolicy": "calico",
}

# Endpoints with no query params — snapshot verbatim, one file each.
STATIC_ENDPOINTS = {
    "graph.json": "/api/graph",
    "cluster-state.json": "/api/cluster-state",
    "issues.json": "/api/issues",
    "mesh-status.json": "/api/mesh-status",
    "cluster-metrics.json": "/api/cluster-metrics",
}

# Reachability is O(nodes^2). Past this the payload stops being a demo asset;
# fail loudly rather than ship a 100MB page.
MAX_REACHABLE_PAIRS = 8000


def get_json(base_url, path, params=None):
    url = base_url + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    with urllib.request.urlopen(url, timeout=60) as response:
        return json.load(response)


# Returns None instead of raising for 4xx — a missing manifest or an
# unreachable pair is data about the demo cluster, not a script failure.
def get_json_optional(base_url, path, params=None):
    try:
        return get_json(base_url, path, params)
    except urllib.error.HTTPError as error:
        if 400 <= error.code < 500:
            return None
        raise


def snapshot_node_info(base_url, nodes):
    details = {}
    for node in nodes:
        detail = get_json_optional(base_url, "/api/node-info", {"nodeId": node["id"]})
        if detail is not None:
            details[node["id"]] = detail
    return details


# Every ordered pair of nodes, because the operator pins an arbitrary source and
# then probes arbitrary targets; the reverse direction is a separate verdict.
def snapshot_reachability(base_url, nodes):
    pair_count = len(nodes) * (len(nodes) - 1)
    if pair_count > MAX_REACHABLE_PAIRS:
        sys.exit(f"refusing to snapshot {pair_count} reachability pairs "
                 f"(limit {MAX_REACHABLE_PAIRS}) — trim the demo fixtures")

    verdicts = {}
    for source in nodes:
        for target in nodes:
            if source["id"] == target["id"]:
                continue
            params = {
                "srcId": source["id"], "srcNs": source.get("namespace", ""),
                "dstId": target["id"], "dstNs": target.get("namespace", ""),
            }
            verdict = get_json_optional(base_url, "/api/reachable", params)
            if verdict is not None:
                verdicts["|".join(params.values())] = verdict
    return verdicts


# Manifest targets come from the fixtures rather than the graph: a policy that
# selects nothing produces no edge but is still openable from the tables view.
def collect_manifest_refs(fixture_dir):
    refs = []
    for filename in sorted(os.listdir(fixture_dir)):
        if not filename.endswith((".yaml", ".yml")):
            continue
        with open(os.path.join(fixture_dir, filename)) as handle:
            for document in yaml.safe_load_all(handle):
                if not isinstance(document, dict):
                    continue
                kind = MANIFEST_KIND_BY_FIXTURE_KIND.get(document.get("kind"))
                if not kind:
                    continue
                metadata = document.get("metadata", {})
                refs.append({
                    "kind": kind,
                    "namespace": metadata.get("namespace", ""),
                    "name": metadata.get("name", ""),
                })
    return refs


def snapshot_manifests(base_url, refs):
    manifests = {}
    for ref in refs:
        manifest = get_json_optional(base_url, "/api/manifest", ref)
        if manifest is not None:
            manifests["|".join([ref["kind"], ref["namespace"], ref["name"]])] = manifest
    return manifests


def write_json(out_dir, filename, payload):
    path = os.path.join(out_dir, filename)
    with open(path, "w") as handle:
        json.dump(payload, handle, separators=(",", ":"))
    print(f"{filename}: {os.path.getsize(path) / 1024:.0f} KiB")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--out", default="ui/public/demo")
    parser.add_argument("--fixtures", default="test-data")
    args = parser.parse_args()

    os.makedirs(args.out, exist_ok=True)

    responses = {name: get_json(args.base_url, path)
                 for name, path in STATIC_ENDPOINTS.items()}
    nodes = responses["graph.json"]["nodes"]
    print(f"graph: {len(nodes)} nodes, {len(responses['graph.json']['edges'])} edges")

    responses["node-info.json"] = snapshot_node_info(args.base_url, nodes)
    responses["reachable.json"] = snapshot_reachability(args.base_url, nodes)
    responses["manifest.json"] = snapshot_manifests(
        args.base_url, collect_manifest_refs(args.fixtures))

    for filename, payload in responses.items():
        write_json(args.out, filename, payload)


if __name__ == "__main__":
    main()
