# network-policy-visualizer

> **A Kubernetes policy reachability & gap analyzer.**
> Resolves what your pods can actually reach across NetworkPolicy, Istio AuthorizationPolicy, and ambient-mesh mTLS — then shows the gaps. Read-only by design, runs as a single static binary.

**Stop guessing what your pods can actually reach.**

Policy is written per-engine and per-namespace, but reachability is *emergent*. No one can answer "can A reach B, and why?" by reading YAML in three places. This tool intersects all the layers and puts the answer on one screen.

<!-- screenshot: full graph overview (existing: docs/fullGraph.png) -->
![Full graph](docs/fullGraph.png)

---

## The problem

Modern Kubernetes pod-to-pod security is a layer cake, and the layers do not talk to each other:

- **K8s NetworkPolicy** — L3/L4. Default-open until something selects you, then default-deny in the locked direction.
- **Istio AuthorizationPolicy** — L4 + L7, ALLOW + DENY, ingress-only. Ignored entirely if the workload is not in the mesh.
- **Istio ambient mesh** — ztunnel handles L4 + mTLS, but only for pods labeled `istio.io/dataplane-mode: ambient`. mTLS posture comes from `PeerAuthentication` with a global → namespace → workload precedence chain.

Each layer is hard to reason about alone. Stacked, the failure modes multiply:

- A pod with **zero policies is wide open by default**. `kubectl get networkpolicies` never lists the workloads that lack one. The gap is invisible.
- The **two policy engines have different semantics**. Effective reachability is an **AND across engines** — a workload allowed by one and blocked by another is blocked at runtime. The graph everyone draws by hand is a lie.
- **Ambient mesh enrollment is a single label** — easy to set, easy to miss, easy to override at the wrong scope. An `AuthorizationPolicy` on a non-enrolled pod reads like enforcement and does nothing.
- **PeerAuthentication STRICT silently breaks legacy clients**. A non-mesh caller hitting a STRICT destination fails at the transport layer before any policy is even evaluated.
- **Ambient + NetworkPolicy is a footgun.** Ambient traffic is delivered via ztunnel on HBONE port **15008**. A port-restricted NetworkPolicy that does not allow 15008 silently drops every mesh-routed packet — the workload looks healthy, mesh traffic just disappears.

Reading three layers of YAML in three languages and intersecting them in your head is not a reliable workflow. This tool does the resolution.

---

## Who it's for

**SREs on call.** "Service A can't reach service B" stops being a 40-minute YAML excavation. Pin A, click B, read the verdict per engine + mesh. The blocking layer is named.

**Platform / ops engineers rolling out mesh.** Enrolling a namespace in ambient is one label. The downstream effects (NetworkPolicy interop, PeerAuthentication precedence, AuthorizationPolicy scope) are not obvious until something breaks. This shows membership, mTLS posture, and the ambient-vs-NP port trap before you ship.

**DevOps engineers inheriting a cluster.** Helm-installed everything looks fine in `kubectl get pods`. Open the visualizer, every pod wearing `WAN⇆` is reachable from the public internet — or has no policy at all and defaults to it. Triage from there.

**Security review and audit.** Filter on `WAN⇆` / `WAN↑`. Every workload that can reach `0.0.0.0/0` lights up. The Tables view lists the same data row-by-row, sortable and filterable for spreadsheet handoff.

---

## What's in the product today

### The graph — every policy, every engine, on one canvas

Live nodes for every pod, cronjob, and namespace. Edges for everything each policy permits or denies, **tagged by engine** so k8s and Istio render as separate edges between the same pair. Immediately see when one engine allows what another blocks.

- Direction-aware arrows (ingress / egress / both) + explicit DENY styling for Istio deny rules.
- L7 details (hosts, methods, paths) attached to Istio edges.
- Namespace-rollup view collapses each ns to one node for tenant-isolation checks.
- Status badges on every workload (table below), computed from the **union of every selecting policy across every engine**, intersected so a workload only earns an "open" badge if every engine permits it.

### Edge panel — the connection's full truth

Click any arrow. The panel shows what the rule says — engine, policy name, namespace, direction, ports, L7 matchers, SRC + DST workload labels — and the cross-engine reachability verdict at the top:

```
[DENY] end-to-end
reason: blocked by k8s
Engines: istio=allow  k8s=deny
Mesh:    istio=allow
```

The arrow on canvas is one engine's opinion. The banner is the truth across every engine and the mesh. When they disagree, the lying-graph trap surfaces immediately — no guessing why "the policy says allow but traffic dies."

<!-- screenshot: edge click panel showing cross-engine verdict (e.g. istio allow vs k8s deny) -->

### Tables view — audit, hygiene, search

Two sortable, filterable tables sharing the same filters as the graph:

- **Workloads** — namespace, type, effective status keys, per-engine policy counts, labels. Row click opens the detail panel inline; explicit `→ Graph` button when you want the spatial view. A status-rollup strip across the top counts every status key live and pivots the table when you click a chip.
- **Policies** — one row per `(engine, namespace, policy name, action)`. Rules + endpoints columns surface how broad each policy is. Click a row → drill into every workload it actually affects. Engine-rollup strip across the top counts policies per engine.

Built for the auditor's question: "show me every workload with `internet-ingress`" or "list every policy selecting more than N workloads." Answer in two clicks.

<!-- screenshot: tables view showing both Workloads and Policies tabs with rollup chips -->

### Reachability checks — "can A actually reach B?" answered across all three layers

Pin a source, click any destination. A side-by-side panel renders the verdict per layer:

- **K8s NetworkPolicy** — which policies select each end, which allow / deny rules matched per direction, whether the path is locked or open.
- **Istio AuthorizationPolicy** — same breakdown with L7 matchers shown.
- **Istio mesh / mTLS** — src + dst mesh membership, resolved PeerAuthentication mode on each side, transport-layer verdict. STRICT destination + non-mesh source → mesh blocks, even when both policy engines allow.

The exact PeerAuthentication object that forced the mesh verdict is named.

![Selected node](docs/selectNode.png)

### Ambient-mesh misconfiguration detector

Clicking any workload runs cross-cutting validators that don't fit inside a single policy. The headline one today:

> **"This workload is in the Istio ambient mesh, but its NetworkPolicy only allows ingress on port(s) `[8080]`. Ambient delivers traffic through ztunnel on port 15008, which is blocked. Add an ingress rule allowing TCP port 15008 so mesh traffic can reach the workload."**

This catches the single most common ambient rollout failure: a perfectly valid NetworkPolicy and a perfectly valid AuthorizationPolicy that, together, silently null out every packet ztunnel tries to deliver.

---

## Status badges — what each one means

Computed per workload from the intersection of every selecting policy across every engine.

| Badge | When it fires |
|---|---|
| `WAN⇆` | Internet reachable both directions — **including any pod with no policy at all** (default-open is the headline footgun). |
| `WAN↑` / `WAN↓` | One direction locked, the other reaches `0.0.0.0/0`. |
| `LAN⇆` / `LAN↑` / `LAN↓` | Explicit `ipBlock` peer targets a private LAN outside the cluster. |
| `API↑` | Egress reaches a configured Kubernetes API-server CIDR. |
| `⊘` | Both directions locked, no escape hatch — properly air-gapped. |
| `⇆` | Peer namespaceSelector targets a different namespace. |
| `NS⇆` / `NS↑` / `NS↓` | Catch-all peer matches every pod in the same namespace. |
| `L7` | Istio AuthorizationPolicy uses L7 matchers — reachability depends on HTTP attributes the graph can't fully simulate. |

The badge to hunt for in practice: **`WAN⇆` on workloads you did not expect to be public**. That's almost always a missing policy.

---

## When you'd actually use it

- **"I inherited this cluster — what's exposed?"** Every `WAN⇆` is the first question to ask.
- **"The Helm chart said it has network policies — does it really?"** Graph shows which workloads in the chart's namespace got covered and which it silently skipped.
- **"We just enabled ambient on this namespace. Did anything break?"** Detail panel surfaces mesh membership, resolved PeerAuthentication, and the ztunnel-port-15008 NetworkPolicy trap.
- **"Service A can't reach service B — whose fault is it?"** Reachability panel breaks down the verdict per engine + mesh. The blocking layer is named.
- **"We're rolling out STRICT mTLS — what breaks?"** Run reachability from non-mesh workloads into the soon-to-be-STRICT namespace. Any allow → deny flip is a client that needs to be enrolled or exempted before you ship.
- **"A pod got compromised — what could it have reached?"** Every outbound edge is in the detail panel. Faster than reconstructing during an incident.
- **"I want to verify tenants can't reach each other."** Aggregate-by-namespace view collapses each ns into one node. Cross-namespace edges between tenant namespaces are immediately visible — or absent.

---

## Status & roadmap

Today's scope is what's described above — k8s NetworkPolicy + Istio AuthorizationPolicy + ambient-mesh mTLS, graph view, tables view, click-driven reachability, ambient misconfiguration detection. Stable enough to use against a real cluster.

Deferred to later iterations:

- **Operational survival** — cluster-scoped RBAC bundle, response size caps, per-engine failure isolation, watch-driven cache instead of per-request evaluation. The current data path is single-replica and request-driven; fine for a few hundred pods, will need work for thousands.
- **More policy engines** — Cilium `CiliumNetworkPolicy` / `CiliumClusterwideNetworkPolicy`, `AdminNetworkPolicy` (KEP-2091), Calico `GlobalNetworkPolicy`. Each adds real reachability fidelity on clusters that use it.
- **Cluster-state metrics & trust signals** — data freshness stamp, per-engine last-success timestamps, partial-render banners when an engine fails. The product is silent today when an engine errors.
- **Policy hygiene linter** — orphan policies, `0.0.0.0/0` ingress without explicit annotation, AuthorizationPolicy principals referencing non-existent ServiceAccounts. Shippable from existing data; not built yet.
- **Snapshot diff** — "what changed in policy state since 5 minutes ago" for incident timelines.

See [`docs/FEATURES.md`](docs/FEATURES.md) for the ranked wishlist with concrete file-line references.

---

## Quick start

Against your live cluster:

```bash
go build -o graph .
KUBECONFIG=~/.kube/config ./graph
# UI → http://localhost:8080
```

The binary embeds the built UI — single static binary, no separate frontend deploy. Read-only on your cluster.

Demo mode (no cluster required):

```bash
DEMO_MODE=true ./graph
```

Loads sample data from embedded `test-data/` — includes a real `observability` namespace dump showing partial NetworkPolicy coverage as it appears in production, plus curated Istio scenarios (DENY rules, L7 ALLOW with hosts / methods / paths, ambient mesh enrollment with PeerAuthentication precedence).

## RBAC

Minimum permissions:

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods", "namespaces"]
    verbs: ["get", "list"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list"]
  - apiGroups: ["batch"]
    resources: ["cronjobs"]
    verbs: ["get", "list"]
  # Optional — only required if Istio is installed and you want AuthZ + mesh in the graph.
  - apiGroups: ["security.istio.io"]
    resources: ["authorizationpolicies", "peerauthentications"]
    verbs: ["get", "list"]
```

## Development

```bash
go test ./...                          # unit tests for graph + policy + mesh + reachability logic (no cluster needed)
cd ui && npm install && npm run dev    # Vite dev server on :5173, proxies /api → :8080
```

The frontend sends `?namespaces=a,b` and the backend queries only those. Per-namespace fetches run concurrently. Mesh / mTLS resolution is **lazy** — PeerAuthentications are fetched only when a workload is clicked or a reachability check runs, so the graph payload stays cheap. No watches, no caching layer — every request hits the API server fresh, so the picture is always live.

For architecture detail see the ADRs in [`docs/arch/`](docs/arch/) — in particular [`0003-istio-mesh-membership-and-mtls.md`](docs/arch/0003-istio-mesh-membership-and-mtls.md) for the mesh integration design — and [`docs/status-key-computation.md`](docs/status-key-computation.md). Dev container setup is in [`CLAUDE.md`](CLAUDE.md).
