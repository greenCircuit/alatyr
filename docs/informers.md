# client-go Informers — How They Work and Why We Want Them

Reference doc for moving this tool off per-request, per-namespace k8s List calls onto an informer-backed cache. Written against `k8s.io/client-go v0.36.0` (already a dependency).

## The problem we have today

The graph build (`store.PopulateCache`) refetches everything from the apiserver on **every** `GET /api/graph`:

- `fetchNsIndex` runs per namespace; each does 3 serial calls (`GetPods`, `GetCronJobs`, `GetNs`).
- The k8s engine loops namespaces calling `GetPolicies(ns)` serially.
- The istio engine loops namespaces calling `GetAuthorizationPolicies(ns)` serially + 1 root-ns.
- Engines run serially.

For N namespaces that's ~**5N+1 API round-trips per request**, much of it serialized, all under a write lock. On a medium cluster (tens of namespaces, hundreds–thousands of pods) this is the dominant cost of "the graph takes several seconds to load." Pod objects are also fat (status, `managedFields`), so payload bytes + deserialize CPU add up.

Every request re-pays the full cost even when nothing in the cluster changed.

## What an informer is

An informer maintains a **local, always-current, in-memory mirror** of a set of cluster objects, kept in sync by watching the apiserver. Your code reads from the local mirror (a *Lister*) instead of calling the apiserver. Reads become memory lookups with zero network latency, independent of cluster size after warm-up.

It is the standard pattern for any long-running process that watches the same resources repeatedly — controllers, the scheduler, kubelet, and every operator built on controller-runtime all use it. One-shot CLIs like `kubectl` do not (they List once and exit, which is what this tool does today).

## How it works — the server computes the diff, not you

You do **not** compare snapshots or diff caches yourself. The flow:

1. **Reflector** does one initial **LIST** → all objects + a collection `resourceVersion` (a cursor).
2. It opens a **WATCH** from that `resourceVersion`. The apiserver streams `ADDED` / `MODIFIED` / `DELETED` events. **The server computes the deltas**; `resourceVersion` advances as the cursor. After the initial sync, only changes cross the wire.
3. Events flow through a **DeltaFIFO** queue into a thread-safe **Indexer** — the local mirror, continuously updated.
4. Your code reads via a **Lister** (e.g. `podLister.Pods(ns).List(selector)`) — a local memory read, no apiserver call.
5. If the watch drops or the `resourceVersion` expires (`410 Gone`), the reflector automatically re-LISTs and resumes the watch. Handled for you.

```
apiserver ──LIST──▶ Reflector ──▶ DeltaFIFO ──▶ Indexer (local mirror)
    │                                                  ▲
    └──WATCH (deltas, keyed by resourceVersion)────────┘
                                                   Lister reads ◀── your code
```

Optional `OnAdd` / `OnUpdate` / `OnDelete` event handlers can be registered for reactive work (reconcile loops). A read-on-demand server like this one does **not** need them — it just reads the Lister when a request comes in.

### resourceVersion, precisely

- The collection `resourceVersion` from the initial LIST is the watch cursor; the server uses it to stream only what changed since.
- Each object also carries its own `metadata.resourceVersion`, bumped on every write. You do **not** inspect it manually — the Indexer is kept current for you.
- `ListOptions{ResourceVersion: "0"}` is a *different* knob: it asks the apiserver to serve a LIST from its watch cache (fast, possibly slightly stale) instead of a quorum etcd read. Useful for the initial sync. It does not skip the call and is unrelated to delta detection.

### Resync is not a re-List

The informer "resync period" periodically re-delivers the **local store** to registered event handlers (for reconcile patterns). It does **not** re-LIST from the apiserver. We have no reconcile loop, so set the resync period to **0**.

## What problems informers solve for us

| Problem today | With informers |
|---|---|
| ~5N+1 API round-trips per request | One LIST + ongoing WATCH per resource type at startup; requests read local memory |
| Latency scales with namespace count | Read latency is ~constant, independent of cluster size |
| Refetch even when nothing changed | Server pushes only deltas; cache stays warm |
| Fat pod payloads re-deserialized every request | Deserialized once on event; reads hit in-memory objects |
| Write lock serializes concurrent loads | Listers are concurrent-safe reads |

## Concerns and how informers address them

### RBAC scoping (namespace-restricted ServiceAccounts)
Cluster-wide List breaks a namespace-scoped SA token (it can't list at cluster scope). Informers do **not** force cluster scope:

```go
factory := informers.NewSharedInformerFactoryWithOptions(
    clientset, 0 /* resync */, informers.WithNamespace(ns))
```

Construct one factory per permitted namespace (or a filtered factory) to watch only what the SA may see. Cluster-wide vs per-namespace is purely a factory-construction choice; downstream lister calls are identical.

### Don't pull full manifests
Vanilla k8s has **no field projection** on List/Watch (no field-mask / GraphQL-style selection). Two real levers:

- **`SetTransform(fn)`** on a typed informer — runs per object before storage, strips fields you don't need (`managedFields`, `status`, annotations). Cuts **in-memory size + CPU**. The object still arrives full over the wire.
- **Metadata-only informer** (`metadatainformer` / `PartialObjectMetadata`) — fetches only `metadata` (name, labels, annotations, ownerRefs). Cuts **wire bytes**, but drops `spec` and `status`.

Caveat for our data model (`graph.AssembleNsIndex`):
- Pods need `Status.Phase` (to skip `Succeeded`/`Failed`). Metadata-only loses it — replace with a server-side **field selector** `status.phase!=Succeeded,status.phase!=Failed`, then metadata-only works (we only need labels + ownerRefs).
- CronJobs need `Spec.JobTemplate.Spec.Template.Labels` (in spec). Metadata-only can't supply it → use a typed informer + `SetTransform`.

Pragmatic default: **typed informers + `SetTransform` stripping `managedFields`** everywhere. Most of the win, least friction. Move pods to metadata-only later if memory pressure shows up.

## How it lands in this codebase

The seam already exists: `k8s.KubernetesClient` is an interface, and engines + `fetchNsIndex` call methods like `GetPods(ns)`. Plan:

1. Add an informer-backed `KubernetesClient` implementation whose `GetPods` / `GetCronJobs` / `GetPolicies` / `GetAuthorizationPolicies` / `GetNs` read from listers instead of the apiserver.
2. Build `SharedInformerFactory` (k8s) + the istio informer factory at server start, with `WithNamespace` per the configured scope.
3. `factory.Start(stopCh)` then `factory.WaitForCacheSync(stopCh)` before serving — first start blocks on warm-up, then every request is a local read.
4. Apply `SetTransform` to strip `managedFields`.
5. Pure functions downstream (`AssembleNsIndex`, engine `buildRules`/`buildStatusKeys`, render) are **unchanged** — they already take fetched objects.

`PopulateCache`'s own `EvaluationResults`/`NsIndex` cache can stay as a per-request assembly layer, or shrink once reads are cheap; that's a follow-up decision, not required for the informer swap.

## Demo mode is unaffected

Development runs in `DEMO_MODE=true`, which uses `k8s.DemoClient` (embedded YAML fixtures), never a cluster. Informers do not touch this path.

`main.go` switches on `DEMO_MODE` and selects one `KubernetesClient` impl: `DemoClient` (fixtures) or the real `Client` (apiserver). `DemoClient` already serves `GetPods` / `GetPolicies` / etc. from **in-memory maps** — zero latency, no apiserver. Informers exist solely to eliminate apiserver round-trips, which demo mode does not make. There is nothing to optimize there.

So the informer machinery (factory, listers, watch, `SetTransform`) lives **inside the real `Client` only**. Both impls keep satisfying the same `Get*` methods; the `DEMO_MODE` switch is untouched; demo dev behaves exactly as before.

**Design rule that keeps it this way:** do **not** leak lister types into the `KubernetesClient` interface. Keep methods as `GetPods(ns) ([]Pod, error)` etc. The real `Client` hides the lister behind those methods; `DemoClient` returns fixtures behind the same signature. If the interface exposed something like `PodLister()`, `DemoClient` would have to fake a lister — breaking the seam. Keeping `Get*` is what makes informers invisible to demo.

**Optional — exercising the informer path without a cluster.** To test the informer *wiring* itself without k3s, `k8s.io/client-go/kubernetes/fake` supports informers via its object tracker (watch reactors fire on tracker changes). Seed it from the same fixture YAML and build informers on the fake clientset. Only worth it to test the watch code — not needed for normal demo dev, since `DemoClient` bypasses all of it.

## When NOT to use informers
- Truly one-shot execution (run once, print, exit) — the initial LIST+sync overhead buys nothing.
- Watching enormous resource sets you barely read — memory cost of mirroring may exceed savings. (Not our case: we read pods/policies on every request.)

## Prior art — Kiali uses exactly this

Kiali (the reference Istio dashboard) solves the identical "don't hit the apiserver per request" problem with the same machinery. Verified against its `kubernetes/cache` package, not from memory:

- **`KubeCache`** — informer/lister-backed cache. `NewKubeCache()` "starts all informers"; reads (`GetPods`, `GetServices`, `GetDeployments`, the Istio CRDs `AuthorizationPolicy` / `PeerAuthentication` / …) come from **cached listers**, not live apiserver calls. Same shape as our proposed `Get*` → lister.
- **`StripUnusedFields()`** — a transform applied to objects before storage to cut memory. Exactly the `SetTransform` / managedFields-stripping above.
- **`Refresh(namespace)`** — empty ns rebuilds the whole cache, specific ns scopes it. The cluster-vs-namespace scope choice.
- **`Stop()`** — terminates all informers; lifecycle owned by the cache object (process-scoped singleton). Maps 1:1 to "who manages the informer."
- Caches **both** core k8s objects **and** Istio CRDs — same dual-source shape as our k8s + istio engines.

Takeaway: informers + listers + a strip transform is the mainstream choice for a read-mostly k8s/Istio visualizer. Kiali's `Refresh` / `Stop` / singleton lifecycle is the same structure we'd build.

### The RBAC trick worth stealing

Earlier tension: per-namespace RBAC vs a cheap cluster-wide bootstrap (1 LIST/type) — seemingly can't have both. Kiali sidesteps it by **separating two identities**:

- **Fetch identity** — the cache is a singleton using Kiali's own **service-account token with broad read**, which fills the informers once (cheap cluster-wide bootstrap).
- **Access control** — per-user access is filtered **on read**, against the requesting user's token, *after* the data is already cached ("user-level access must be filtered based on their token permissions externally").

So Kiali gets both the cheap bootstrap **and** tenant restriction — fetch broadly with a privileged SA, authorize per request. Our tool currently uses one identity (the kubeconfig SA) for both fetch and access. If multi-tenant restriction ever matters, adopt Kiali's split (privileged cache SA + per-request authz filter) rather than scoping the informer per namespace — the latter forces the O(N)-LISTs-at-boot tradeoff described above.

## References
- client-go `tools/cache` (Reflector, DeltaFIFO, Indexer, SharedInformer)
- `k8s.io/client-go/informers` (SharedInformerFactory, `WithNamespace`)
- `k8s.io/client-go/metadata/metadatainformer` (metadata-only)
- `SharedIndexInformer.SetTransform` (field stripping)
- Kiali cache package — https://pkg.go.dev/github.com/kiali/kiali/kubernetes/cache
- Kiali repo — https://github.com/kiali/kiali

# Initial
=== initial-load sweep ===
    namespaces     pods_per_ns  policies_per_engine           nodes           edges         cold_ms     warm_p50_ms
             1               5               5               5              20            16.4             6.3
             5               5               5              25             100          1102.4          2001.3
            10               5               5              50             200          2594.3          3999.3
            20               5               5             100             400         10214.3         10214.3
             1              10               5              11             125            51.1           399.4
             1              20               5              21             500            15.0           399.8
             1              40               5              41            2000            19.9           396.8
             1              60               5              61            4500            32.7           353.4
             1               5               1               5               4           193.7           399.9
             1               5               5               5              20           152.5           399.4
             1               5              10               5              40            39.1           399.8
             1               5              20               5              80            12.5           399.6

# After informents, just raw compute not including getting whe using informents

    namespaces     pods_per_ns  policies_per_engine           nodes           edges         cold_ms     warm_p50_ms
             1               5               5               5              20             4.2             1.5
             5               5               5              25             100             6.0             2.5
            10               5               5              50             200             5.7             4.1
            20               5               5             100             400             6.9             6.1
             1               5               5               5              20             1.5             1.3
             1              25               5              25             720             7.1             6.4
             1              59               5              59            4205            29.0            35.3
             1             100               5             101           12500           108.6            87.8
             1               5               1               5               4             1.5             1.2
             1               5               5               5              20             1.7             1.3
             1               5              10               5              40             2.0             1.6
             1               5              20               5              80             2.0             1.8