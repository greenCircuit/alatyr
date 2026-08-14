#!/usr/bin/env python3
"""Freeze the demo-mode API into static JSON for GitHub Pages.

Boots nothing itself — expects a backend already running with DEMO_MODE=true.
Crawls every endpoint the UI calls and writes the responses under
ui/public/demo/. Parameterized endpoints (node-info, reachable, manifest)
collapse into one lookup map per endpoint, keyed by their query params, so the
static frontend resolves them client-side without one file per combination.
"""

import argparse
import concurrent.futures
import json
import os
import sys
import threading
import urllib.error
import urllib.parse
import urllib.request

# Handlers serve from the graph cache under an RLock (internal/api/node-data.go),
# so parallel reads are safe and the snapshot is bounded by round-trips, not CPU.
WORKERS = int(os.environ.get("SNAPSHOT_WORKERS", "8"))

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


# Fans `jobs` — (result key, query params) pairs — across a thread pool and
# rewrites a single progress line as responses land, so a multi-thousand-request
# crawl shows movement instead of looking hung. Keys whose response was a 4xx
# are absent from the returned map.
def fetch_parallel(base_url, path, jobs):
    results = {}
    total = len(jobs)
    completed = 0
    lock = threading.Lock()

    def report():
        print(f"  {path}: {completed}/{total}", end="\r", flush=True)

    report()
    with concurrent.futures.ThreadPoolExecutor(max_workers=WORKERS) as pool:
        futures = {pool.submit(get_json_optional, base_url, path, params): key
                   for key, params in jobs}
        for future in concurrent.futures.as_completed(futures):
            payload = future.result()
            with lock:
                if payload is not None:
                    results[futures[future]] = payload
                completed += 1
                report()

    missing = total - len(results)
    suffix = f", {missing} with no response" if missing else ""
    print(f"  {path}: {total}/{total} done{suffix}      ")
    return results


def snapshot_node_info(base_url, nodes):
    jobs = [(node["id"], {"nodeId": node["id"]}) for node in nodes]
    return fetch_parallel(base_url, "/api/node-info", jobs)


# CIDR nodes carry no namespace, so the UI substitutes the node id (see
# graphStore.ts `src.namespace || src.id`). Mirror that exactly: the backend
# rejects an empty ns, and the snapshot key must match what the UI asks for.
def query_namespace(node):
    return node.get("namespace") or node["id"]


# Every ordered pair of nodes, because the operator pins an arbitrary source and
# then probes arbitrary targets; the reverse direction is a separate verdict.
def snapshot_reachability(base_url, nodes):
    pair_count = len(nodes) * (len(nodes) - 1)
    if pair_count > MAX_REACHABLE_PAIRS:
        sys.exit(f"refusing to snapshot {pair_count} reachability pairs "
                 f"(limit {MAX_REACHABLE_PAIRS}) — trim the demo fixtures")

    # A dropped pair is a hole the UI hits as a 404 at click time; fetch_parallel
    # reports the count so the snapshot never looks complete when it isn't.
    jobs = []
    for source in nodes:
        for target in nodes:
            if source["id"] == target["id"]:
                continue
            params = {
                "srcId": source["id"], "srcNs": query_namespace(source),
                "dstId": target["id"], "dstNs": query_namespace(target),
            }
            jobs.append(("|".join(params.values()), params))
    return fetch_parallel(base_url, "/api/reachable", jobs)


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
    jobs = [("|".join([ref["kind"], ref["namespace"], ref["name"]]), ref)
            for ref in refs]
    return fetch_parallel(base_url, "/api/manifest", jobs)


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

    responses = {}
    for name, path in STATIC_ENDPOINTS.items():
        print(f"  {path}", flush=True)
        responses[name] = get_json(args.base_url, path)

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
