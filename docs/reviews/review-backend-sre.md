# Backend SRE review — branch 43-add-istio-workload-membership

Ranked by 3am-pager priority.

## P0 — Silently-wrong mesh membership

### 1. Workload-level ambient opt-in on non-ambient ns is silently dropped
`/app/internal/mesh/istio/buildMeshMembership.go:42-50` — outer branch tests ns label only. A workload carrying `istio.io/dataplane-mode=ambient` in a ns without the label gets stamped `InMesh: false`. `inAmbientMesh` at `/app/internal/mesh/istio/utils.go:54` documents "workload label wins over namespace label" — eager path violates that contract. Detail endpoint `GetWorkloadMesh` uses `Membership` (correct), so the on-click panel disagrees with the graph. Classic "graph quietly lies" case. Fix: run `inAmbientMesh(node.Labels, nsLabels)` per workload always; skip PA fetch only when zero workloads in ns are enrolled.

### 2. `IngressNamespace = "istio ingress"` typo (space, not hyphen)
`/app/internal/mesh/istio/istio.go:44` — real ns is `istio-ingress`. Every `participatesInMesh` + `Membership` check against this constant misses. Gateway pods never get stamped `InMesh` via the ns rule; HBONE-15008 gate on rules touching them false-clears. No test uses the real value, so it hasn't been caught.

## P1 — Correctness bug in HBONE validator

### 3. Ingress peer resolved via `DstID` not `SrcID`
`/app/internal/mesh/istio/detect.go:159-162` — `cache.Workload(rule.DstID, rule.DstNamespace)` runs for both directions. k8s ingress rules swap so `DstID` = protected (ambient) workload, `SrcID` = peer (see `/app/internal/policy/k8spolicy/buildRules.go:41-42`). Effect on ingress path: gate resolves the ambient workload itself (always in-mesh), "peer not in mesh" skip never fires. NetworkPolicy allowing ingress from `0.0.0.0/0` on 8080 gets flagged `MeshTransportBlocked`. Every ingress test uses in-mesh dst → passes CI while lying in prod. Fix: peer = `SrcID`/`SrcNamespace` for ingress; `DstID`/`DstNamespace` for egress.

### 4. Dead `len(rule.Ports)==0` check inside port loop
`/app/internal/mesh/istio/detect.go:168, 184` — check lives inside `for _, port := range rule.Ports` which doesn't execute when Ports is empty. Empty-ports allow-all-peer stanzas are caught by `detectGlobalHBONEOpen` (benign today), but a restricted-peer + empty-ports stanza (peer-specific, all ports open) silently escapes the "not missing" branch. Fragile; hoist the empty-check above the loop.

## P1 — Cache concurrency contract

### 5. Coupled mesh cache fields under handleGraph write lock
`/app/internal/store/buildStore.go:193-204` — safe today because `handleGraph` (`/app/internal/api/network-policy.go:24`) holds `s.mu.Lock()` for the full `PopulateCache` call. `MeshStatuses`/`MeshMetrics` (`/app/internal/api/meshStatus.go:22, 31`) take RLock and hold the map ref; a subsequent write replaces the map, no torn read. Fine. Flag only because three coupled cache fields (`MeshMembership`, `MeshMetrics`, `MeshIssues`) with in-place append at line 202 assume single-writer. Any future background refresh with overlapping ns sets will double-append MeshIssues. Not reachable now; watch it.

## P2 — Issue payload gaps

### 6. Root PA-fetch failure Issue lacks `Node`
`/app/internal/mesh/istio/buildMeshMembership.go:31-36` — no workload/ns anchor on the emitted Issue. UI filters that key on Node/Src/Dst drop it silently. The ns-scoped failure at line 68 already attaches `idx.NSNode` — do the same for root by synthesizing a root-ns marker or a distinct rendering path.

### 7. PERMISSIVE default is not flagged — good
`buildIssues.go:153-172` MeshConflicts only fires on `Verdict=="deny"`. `CanReach` at `/app/internal/mesh/istio/detect.go:50-52` returns "allow" for PERMISSIVE dst against non-mesh src. Correct behavior; no false positive here.

## P2 — Test coverage gaps

- Ambient workload label on a non-ambient ns — would catch P0#1.
- `IngressNamespace` matches real `istio-ingress` — would catch P0#2.
- `ValidateExternalRules` ingress tests never exercise the k8s swap (`SrcID` is unset, `DstID` holds the peer). Add: `SrcID` = out-of-mesh peer, `DstID` = the workload under test, non-ambient `SrcNamespace` → assert not flagged. Catches P1#3.
- `BuildMeshMembership` has no test for `NSNode == nil`. Would panic on `idx.NSNode.Labels` at `buildMeshMembership.go:41`. Fetch pipeline guards it now, but a nil check + one-line test is cheap insurance.

## P3 — Cleanup

- `/app/internal/mesh/istio/istio.go:12-18` — commented imports still present.
- `/app/internal/models/cache.go:14-16` — comment says "populated by store" but the writer is `mesh.MeshSource.BuildMeshMembership`. Tighten wording.
