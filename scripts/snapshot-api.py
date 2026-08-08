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


# Manifest targets are harvested from the payloads already snapshotted, because
# the UI can only open a manifest it found in one of them: graph edges feed the
# policies table, node-info / issues / reachable / mesh-status feed every other
# ManifestButton. A policy no payload mentions is unreachable in the UI, so
# snapshotting it would be dead weight.
#
# Three ref shapes exist across those payloads:
#   PolicyEdge  {policySource, namespace, policyName}
#   PolicyRef   {source, namespace, name}
#   PARef       {namespace, name}  — PeerAuthentication, kind implied by the UI
def harvest_manifest_refs(payloads):
    refs = {}

    def visit(value):
        if isinstance(value, list):
            for item in value:
                visit(item)
            return
        if not isinstance(value, dict):
            return

        namespace = value.get("namespace")
        if isinstance(namespace, str):
            if isinstance(value.get("policySource"), str) and isinstance(value.get("policyName"), str):
                kind, name = value["policySource"], value["policyName"]
            elif isinstance(value.get("source"), str) and isinstance(value.get("name"), str):
                kind, name = value["source"], value["name"]
            elif isinstance(value.get("name"), str):
                # only PARef reaches here — mesh payloads are the sole source of
                # bare {namespace, name} objects
                kind, name = "pa", value["name"]
            else:
                kind = name = None
            if kind and name:
                refs[(kind, namespace, name)] = {
                    "kind": kind, "namespace": namespace, "name": name,
                }

        for nested in value.values():
            visit(nested)

    visit(payloads)
    return list(refs.values())


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
    args = parser.parse_args()

    os.makedirs(args.out, exist_ok=True)

    responses = {name: get_json(args.base_url, path)
                 for name, path in STATIC_ENDPOINTS.items()}
    nodes = responses["graph.json"]["nodes"]
    print(f"graph: {len(nodes)} nodes, {len(responses['graph.json']['edges'])} edges")

    responses["node-info.json"] = snapshot_node_info(args.base_url, nodes)
    responses["reachable.json"] = snapshot_reachability(args.base_url, nodes)
    refs = harvest_manifest_refs(responses)
    print(f"manifests: {len(refs)} refs")
    responses["manifest.json"] = snapshot_manifests(args.base_url, refs)

    for filename, payload in responses.items():
        write_json(args.out, filename, payload)


if __name__ == "__main__":
    main()
