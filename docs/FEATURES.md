# Feature wishlist — next quarter+

Ranked by what pages someone at 3am, not by what demos well. Severity tags inline. Backend gaps cited at `file:line`. Advisory — items here are not committed unless marked **[shipped]**.

## 0. Shipped since this list was written

- **Calico `GlobalNetworkPolicy` / `NetworkPolicy` engine** — was 5.3. Live + demo, registered in `defaultSources`, selector parser vendored at `internal/thirdparty/calicoselector`.
- **Manifest-directory mode as a product surface** — `-f <dir>` runs the full analysis against a manifest tree with no cluster. Reverses 6.1: `DemoClient` walking arbitrary trees is now the mechanism behind both `-f` and `-report`, not a maintainer-only shortcut. `test-data/scenarios/` are its fixtures.
- **Headless scan (`-report`)** — one scan, table or JSON to stdout, `-output` for a JSON artifact, logs to stderr, exit 2 on scan failure. `cli/`. Turns the tool into a pre-merge gate, not just a dashboard.
- **Severity on every finding** — one table (`SeverityForType`, `internal/models/issues.go`), not per-detector. Same ranking in drawer, tables, and CLI. Actionable vs informational counts split.
- **All workload kinds as first-class nodes** — Deployment, StatefulSet, DaemonSet, CronJob, standalone Job, bare pod. Each kind gets its own cytoscape silhouette (`ui/src/components/PolicyGraph/parts/styles.ts`), a legend entry, and a filter toggle. `DemoClient` parses `CronJob` too, so `-f` trees behave like the live path. Previously only Deployment/CronJob/CIDR/Namespace were togglable — everything else rendered as an unlabeled rectangle.
- **Fetch failure surfaced instead of guessed around** — a PeerAuthentication read that fails for the root ns or a workload ns emits a `failed to fetch` finding naming the ns and the error (`internal/mesh/istio/buildMeshMembership.go`) rather than resolving mTLS mode from partial data. Partial payment on 2.2's partial-render banner; the banner itself is still open.

## 1. Operational survival

Tool needs to not become the outage. Today's data path is single-cluster, single-replica, full-namespace sweep on every `/api/graph` call.

1. **Cluster-scoped read RBAC + least-privilege manifest.** `[3am-pager]` No bundled `ClusterRole` ships today. Operators will grant `cluster-admin` to get it running, then security will rip it back out. Ship a `ClusterRole` granting `get,list,watch` on the eight GVKs in `/app/internal/k8s/k8s.go:11-19` and nothing else. Document that the tool needs cluster-wide read of `NetworkPolicy`, `AuthorizationPolicy`, `PeerAuthentication`, `Pod`, `CronJob`, `Namespace` — namespace-scoped install is not viable since cross-ns selectors break.
2. **Bound the graph response.** `[3am-pager]` `handleGraph` at `/app/internal/api/network-policy.go:12` will happily serialize every workload + every edge in the cluster when called with no `namespaces` param. On a 3k-pod cluster with default-deny everywhere that's a multi-MB JSON the browser will choke on. Hard-cap node count server-side (e.g. 500 workloads) and return a `truncated: true` flag + the dropped namespace list so the UI can prompt for a narrower selection.
3. **Per-engine failure isolation.** `[trust]` `Builder.PopulateCache` at `/app/internal/store/buildStore.go:110-116` aborts the whole request on any single engine error. One flaky Istio CRD lookup kills the k8s NetPol view too. Switch to per-engine error collection: store `EvaluationResult` for engines that succeed, surface a structured `engineErrors map[string]string` on the graph response, render failed engines as a banner in the UI rather than a blank page.
4. **Watch-driven cache, not request-driven.** `[scale]` Informers are already wired in `/app/internal/k8s/informerClient.go` — good — but `PopulateCache` still does a synchronous full re-evaluation per HTTP request (`buildStore.go:70`). On a busy cluster with a polling UI tab open, every 30s tab refresh re-evaluates every policy for every selected ns. Decouple: evaluate on informer event (debounced ~2s), serve `/api/graph` from the last-evaluated snapshot, ship the snapshot timestamp in the response.
5. **Memory budget + eviction for the cache.** `[scale]` `models.Cache` at `/app/internal/store/buildStore.go:71-76` grows unbounded — `NsIndex` and `EvaluationResults` are maps that only get written, never trimmed. A long-lived process that sweeps all namespaces over a week will hold stale `WorkloadNode`s for deleted pods. Tie cache lifetime to informer delete events, or LRU-evict NSIndex entries not touched in N minutes.
6. **Request context, not `context.Background()`.** `[scale]` `buildStore.go:111` passes `context.Background()` into engine `Evaluate`. Client disconnects don't cancel the in-flight evaluation; a user refresh-spamming the graph queues unbounded work. Plumb `c.Request().Context()` through `PopulateCache`.
7. **Multi-cluster as separate sources, not a federated graph.** `[ergonomics]` Don't merge clusters into one graph — that lies about reachability. Add a cluster selector in cluster-state response, keep one `Builder` per kubeconfig context, swap at request time.

## 2. Trust signals

Operators believe a tool when it admits what it doesn't know.

1. **Data freshness stamp on every response.** `[trust]` Graph response has no `evaluatedAt`. After point 1.4 lands, surface the snapshot timestamp + per-engine last-success timestamp; UI renders "stale 4m ago" badge when older than threshold.
2. **Partial-render banner.** `[trust]` Pair with 1.3. Mesh-membership fetch failures already surface as `failed to fetch` findings (see §0) — the gap left is engine-level failure, which still aborts the whole request. UI should say "Istio engine failed: <error>, showing k8s view only" — not silently drop the engine from the filter list.
3. **Unclassified-CIDR surfacing.** `[trust]` `IsIpBlockLanAccess` at `/app/internal/policy/networkPolicyHelpers.go:40` is the catch-all bucket — anything not `0.0.0.0/0`, not cluster-internal, not API server lands in "LAN". On real clusters that bucket hides cloud-provider metadata IPs, peered VPCs, on-prem ranges. Emit a new status key `cidr-unclassified` with the raw CIDR attached so operators can see what was lumped into LAN.
4. **Default-deny blast-radius signal.** `[trust]` Today an unselected pod in a default-deny ns looks identical to a pod with explicit allows. Emit a `policy-orphan` status key for pods in a default-deny ns that no policy selects — these are the ones quietly broken in prod.
5. **Show last-modified on each policy.** `[trust]` Edge panel shows policy name + ns. Add `metadata.resourceVersion` and `creationTimestamp` so on-call can correlate "what changed at 02:47".

## 3. Debugging features

1. **Snapshot diff.** `[ergonomics]` Backend stores last N snapshots (cheap once 1.4 lands), `/api/graph/diff?from=ts1&to=ts2` returns added/removed/modified edges and status-key flips. Single most-requested feature on incident calls.
2. **Reverse query: "what reaches dst".** `[ergonomics]` `/api/reachable` is pairwise today (see `node-data.go:58`). Add `/api/reachable-to?dstId=...` that walks every workload and returns the allow-set. Bounded by 1.2's node cap.
3. **Search by SA / image / label / port.** `[ergonomics]` Cluster-state already ships namespaces — add a workload index endpoint so the UI search box can resolve `sa:my-sa` or `image:nginx` without loading the whole graph.
4. **Why-this-edge hover trace.** `[ergonomics]` Render the chain: selector matched these labels → port range covers 8080 → no deny shadows it. Pure UI on data the backend already has.

## 4. Policy hygiene / lint

Ship as a separate `/api/lint` endpoint that returns findings keyed by policy. Not on the hot graph path. These are also the highest-value additions to the `-report` scan — lint findings need no cluster, so they gate a PR the same way they gate a dashboard.

1. **Orphan policies (selecting zero workloads).** `[hygiene]` Common after a Helm rename. Trivial pass over the existing `WorkloadNode` index.
2. **`0.0.0.0/0` ingress without an explicit annotation.** `[hygiene]` Single highest-signal lint — most prod incidents involving NetPol are "we forgot we left this open". Require an opt-in annotation like `policy.viz/internet-ingress=acknowledged` to silence the warning.
3. **AuthorizationPolicy principals referencing non-existent ServiceAccounts.** `[hygiene]` Dead-rule detector. SA renames are common during ambient migrations.
4. **Default-deny accidentally global.** `[hygiene]` Namespace with a single `podSelector: {}` NetPol and no others — every pod is locked, usually unintended.
5. **Shadowed Istio rules.** `[hygiene]` DENY at root-ns scope shadowing a workload-scope ALLOW. Already half-derivable from the per-engine `PolicyStatus` accumulator in `policy/istio/policyStatus.go`.

## 5. Next engine

Ranked by adoption-weighted pain when absent.

1. **Cilium `CiliumNetworkPolicy` + `CiliumClusterwideNetworkPolicy`.** Largest install base after vanilla NetPol. L7 (HTTP/Kafka/DNS) overlaps with Istio's L7 — your existing `L7Match` type extends cleanly. Cluster-wide variant is the one that gets people, since k8s NetPol has no cluster-scoped form.
2. **`AdminNetworkPolicy` (KEP-2091).** Now GA-track. Cluster-admin overrides everything else; without surfacing it your effective-reachability verdict will lie on any cluster that adopts it. Higher priority than Calico.
3. ~~**Calico `GlobalNetworkPolicy`.**~~ **[shipped]** — see §0.
4. **Gateway API `*RoutePolicy` / `BackendTLSPolicy`.** Skip until north-south is in scope. Today's tool is east-west.
5. **Kyverno / OPA admission.** Skip. Admission policies don't affect runtime reachability — different mental model, different tool.

## 6. What to cut

1. ~~**`DemoClient` walking arbitrary fixture trees.**~~ **[reversed — see §0.]** `-f` and `-report` promote it to a product surface, so the drift risk this item warned about is now real and worth guarding: e2e (`e2e_test.go`) runs the CLI against `test-data/scenarios/`, which keeps fixture-path and live-path behavior honest. Keep that suite green or the warning applies again.
2. **`Statuses` (effective intersection) on `WorkloadNode`.** `/app/internal/graph/renderEngine.go:148` collapses per-engine status into one effective view. On clusters running both NetPol and Istio that intersection is often misleading — one engine says "locked", the other says "no opinion", effective shows "no opinion". Keep `StatusesBySource`, drop the effective roll-up, push the choice of which engine to trust into the UI filter.
3. **Same-named-policy disambiguation by source in the edge key.** `renderEngine.go:21-28` includes `policySource` in `edgeKey`. Correct, but you'll want to revisit once Cilium L7 lands — three engines emitting overlapping edges on the same pair will clutter the canvas. Plan a per-engine layer toggle before engine #3 ships, not after.

## Files referenced

- `/app/cli/` (report mode)
- `/app/internal/models/issues.go`
- `/app/internal/k8s/k8s.go`
- `/app/internal/k8s/informerClient.go`
- `/app/internal/k8s/demo.go`
- `/app/internal/api/network-policy.go`
- `/app/internal/api/node-data.go`
- `/app/internal/api/cluster-state.go`
- `/app/internal/store/buildStore.go`
- `/app/internal/graph/renderEngine.go`
- `/app/internal/policy/networkPolicyHelpers.go`
