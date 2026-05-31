# network-policy-visualizer

A read-only visualizer for cluster policy coverage. Point it at a cluster, get a graph of which workloads can talk to which — across **both** Kubernetes NetworkPolicy and Istio AuthorizationPolicy — and, more importantly, which workloads have **no policy at all** and are wide open.

![Full graph](docs/fullGraph.png)

## What it shows

A live graph rendered from your cluster's pods, services, NetworkPolicies, and Istio AuthorizationPolicies. Three layers of information:

**Status badges** mark each workload's effective security posture. Badges are computed from the union of every policy that selects the workload — across every engine — and then intersected: a workload only earns an "open" badge if every engine permits it. The rules are evaluated, not just listed.

| Badge | Status key | When it fires |
|---|---|---|
| `WAN⇆` | `internet-full` | Internet reachable in both directions. **Also fires on workloads with no policy** — default-open means the pod can reach `0.0.0.0/0` both ways. This is the "unprotected" case. |
| `WAN↑` | `internet-egress` | Ingress locked, but egress can reach `0.0.0.0/0` |
| `WAN↓` | `internet-ingress` | Egress locked, but ingress allows traffic from `0.0.0.0/0` |
| `LAN⇆` / `LAN↑` / `LAN↓` | `lan-*` | Explicit `ipBlock` peer targets a private LAN outside the cluster (not `0.0.0.0/0`, not pod/service CIDR, not the API server) |
| `API↑` | `api-server-egress` | Explicit `ipBlock` peer matches a configured Kubernetes API-server CIDR |
| `⊘` | `air-gapped` | Both ingress and egress are locked **and** no internet / LAN / API-server escape hatch exists |
| `⇆` | `cross-namespace` | A peer's `namespaceSelector` targets a namespace other than the workload's own |
| `NS⇆` / `NS↑` / `NS↓` | `ns-full-access` / `ns-egress-access` / `ns-ingress-access` | A policy peer is a catch-all that matches every pod in the same namespace (empty `podSelector`, no `namespaceSelector` or same-ns selector) |
| `L7` | `l7-applied` | An Istio AuthorizationPolicy selecting this workload uses L7 matchers (hosts / methods / paths) — reachability decisions depend on HTTP attributes the graph can't fully simulate |

A workload with **no badges at all** is a degenerate case — it has policies that lock both directions but no escape hatches were detected, yet the air-gapped check didn't fire. Usually means custom IP rules the classifier doesn't recognise. The badges to hunt for in practice are `WAN⇆` on workloads you didn't expect to be public.

**Edges** show what each policy actually permits or denies. Edges are tagged by engine — k8s and Istio render as separate edges between the same pair, so you can see when a workload is allowed by one engine but blocked by another (which, at runtime, means blocked).

- Workload → workload (specific pod selectors on both ends)
- Workload → namespace (catch-all peer collapses to the namespace node)
- Workload → CIDR (`ipBlock` peers)
- Direction-aware arrows (ingress / egress / both)
- Istio `DENY` policies render with a distinct style — explicit deny is a different signal from "no allow matched"
- L7 details (methods, paths, hosts) attached to the edge for click-through

**Reachability checks** answer "can A talk to B?" without forcing you to mentally trace selectors across multiple policies in multiple namespaces in multiple engines. Pin a source workload, click any other node, and a side-by-side panel shows the verdict per engine: which policies selected each end, which allow/deny rules matched, and which engine (if any) blocked the path. If k8s says yes but Istio says no, the panel makes that explicit instead of hiding it behind a single boolean.

![Selected node](docs/selectNode.png)

## Why it matters

NetworkPolicy and Istio AuthorizationPolicy are the two main ways to lock down pod-to-pod traffic in Kubernetes. Each layer is hard enough to reason about in isolation; running both, which is increasingly common, multiplies the failure modes:

- Policies are **additive within each engine** — multiple rules stack, so one file rarely tells the whole story.
- A workload with **zero policies is wide open by default**, in both engines. `kubectl get networkpolicies,authorizationpolicies` only lists policies, never the workloads that lack them. The gap is invisible.
- Peers mix podSelectors, namespaceSelectors, ipBlocks, principals, namespaces, and `0.0.0.0/0`-with-`except` lists. Easy to misread "looks locked down" for "actually locked down".
- Cross-namespace intent is opaque — a policy in ns-A allowing ns-B traffic only makes sense if you have both namespaces' labels open in another tab.
- **The two engines have different semantics.** K8s NP locks a direction once any policy selects it (default-deny inside that lock). Istio AuthZ uses ALLOW + DENY rules, supports L7 matchers, and is ingress-only. A policy review that only covers one engine misses half the picture.
- **Effective reachability is an AND across engines.** Traffic flows iff every engine in the path permits it. Reading two layers of YAML in two languages and intersecting them in your head is not a reliable workflow.

This tool puts the whole picture on one screen.

### Footguns it surfaces

Anyone who's run more than one cluster has hit at least half of these. The visualizer is designed to make each one visible at a glance.

- **Default-open is backwards.** A pod with no policy can talk to anything, anywhere — there's no warning, no audit event, nothing in any dashboard. The graph flags every such workload with the `WAN⇆` (internet-full) badge — that's the headline signal.
- **`podSelector: {}` means *everything*, not nothing.** People write empty selectors intending "match nothing" and accidentally allow the whole namespace. Shows up as a fat edge from the namespace box.
- **`0.0.0.0/0` egress doesn't isolate intra-cluster traffic.** Every pod in every namespace is inside that CIDR — "allow internet" silently means "allow everything" unless an `except` list is set. Internet badges only fire when the rule is unambiguously external.
- **DNS is the silent killer of egress lockdowns.** Lock egress without allowing `kube-system` UDP 53 and every outbound hostname dies. The graph shows the missing edge.
- **Effective reachability is the OR of every selector inside an engine, AND across engines.** A workload may be selected by three k8s policies and two Istio policies, each contributing partial rules. Nobody resolves this reliably by reading YAML. The tool does the resolution and renders the result, then the reachability panel lets you verify any specific src→dst pair.
- **Istio DENY rules win silently.** A DENY policy in `istio-system` blocks traffic that every ALLOW policy seems to permit. Easy to forget when triaging "why can't service A reach service B" — DENY edges render with a distinct style so they don't hide in the allow noise.
- **An Istio AuthorizationPolicy on a workload outside the mesh has zero effect** — but reads as enforcement. The graph treats out-of-mesh workloads as not enforced by that engine, so you see the gap.

## When you'd actually use it

**"I inherited this cluster — what's exposed?"**
You're new on the team. Helm-installed everything looks fine in `kubectl get pods`. Open the visualizer, switch to a workload namespace, and every pod wearing the `WAN⇆` badge is reachable from the public internet — or, more often, has no policy at all and defaults to it. Those are the ones to ask about first.

**"The Helm chart said it has network policies — does it really?"**
Most upstream charts only ship policies for a handful of components. The visualizer shows which workloads in the chart's namespace ended up covered and which the chart silently skipped. "Loki has an ingress policy, Grafana doesn't" without reading a single YAML.

**"Security is asking what can reach the internet."**
Filter on the `WAN⇆` / `WAN↑` badges. Every workload that can reach `0.0.0.0/0` egress lights up. Screenshot, send. No grepping policy YAMLs across namespaces.

**"A pod got compromised — what could it have reached?"**
Click the workload. The detail panel lists every outbound edge: which pods, which namespaces, which CIDRs, on which ports. Faster than reconstructing it from policy files during an incident.

**"Service A can't reach service B — whose fault is it?"**
Pin service A, click service B. The reachability panel shows each engine's verdict: which selecting policies are in play on both ends, which allow rule should have matched and didn't, or which deny rule killed the path. Often the answer is "Istio is fine, k8s NP doesn't allow it" — without staring at two engines' YAML side by side.

**"We're rolling out Istio AuthorizationPolicy alongside existing NetworkPolicies. Are they consistent?"**
The graph renders both engines as separate edges between the same pairs. Where one engine has an edge and the other doesn't, runtime traffic is blocked. Reachability checks confirm it pair by pair. Cuts the manual cross-reference between two policy languages.

**"I'm about to add a deny-all default policy — will anything break?"**
Run the tool against the current cluster first. Anything currently showing `WAN⇆` without an obvious reason — that is, relying on default-open — is what will break. Lock those down explicitly before flipping the default.

**"I want to verify tenants can't reach each other."**
Aggregate-by-namespace view collapses each ns into one node. Cross-namespace edges between tenant namespaces are immediately visible — or, ideally, absent. Reachability spot-checks confirm specific cross-tenant pairs.

**"I changed a policy — did it do what I expected?"**
Refresh. Compare badges and edges before and after. Re-run the reachability check on the pair you intended to allow or block. No need to mentally simulate selector resolution across both engines.

---

## Quick start

Against your live cluster:

```bash
go build -o graph .
KUBECONFIG=~/.kube/config ./graph
# UI → http://localhost:8080
```

The binary embeds the built UI — single static binary, no separate frontend deploy. Read-only on your cluster (it only `LIST`s pods, namespaces, NetworkPolicies, and Istio AuthorizationPolicies — when the CRDs are installed).

Demo mode (no cluster required):

```bash
DEMO_MODE=true ./graph
```

Loads sample data from embedded `test-data/` — includes a real `observability` namespace dump showing partial NetworkPolicy coverage as it actually appears in production, plus curated Istio AuthorizationPolicy scenarios (DENY rules, L7 ALLOW with hosts / methods / paths).

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
  # Optional — only required if Istio is installed and you want AuthZ in the graph.
  - apiGroups: ["security.istio.io"]
    resources: ["authorizationpolicies"]
    verbs: ["get", "list"]
```

## Development

```bash
go test ./...                          # unit tests for graph + policy + reachability logic (no cluster needed)
cd ui && npm install && npm run dev    # Vite dev server on :5173, proxies /api → :8080
```

The frontend sends `?namespaces=a,b` and the backend queries only those. Per-namespace fetches run concurrently. No watches, no caching layer — every request hits the API server fresh, so the picture is always live.

For architecture detail see the ADRs in [`docs/arch/`](docs/arch/) and [`docs/status-key-computation.md`](docs/status-key-computation.md). Dev container setup is in [`CLAUDE.md`](CLAUDE.md).
