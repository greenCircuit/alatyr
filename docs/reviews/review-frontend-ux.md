# Frontend UX review — mesh feature (branch `43-add-istio-worload-membership-when-buildong-graph`)

Persona: platform/SRE operator, tens of ns, hundreds-to-thousands workloads, time-pressured. Findings ranked by triage priority.

---

## P0 — Silent failure on `fetchMeshMetrics` / `fetchMeshStatus`

**Where:** `/app/ui/src/store/graphStore.ts:208-224` (`loadMeshStatus`, `loadMeshMetrics`).

**Why:** both catch branches only stuff `error: String(e)` on the store. `ClusterMeshSummary` treats `metrics == null` as *loading* (`/app/ui/src/components/ClusterStatusView/ClusterMeshSummary.tsx:28`) — skeleton forever. `MeshRollup` renders whatever `nodes` + empty `meshStatus` yield: every workload as "out of mesh" (`/app/ui/src/store/filters.ts:34-45` — missing entry == `inMesh: false`). Operator sees `0% enrolled` cluster-wide vs a plausible-looking scoped chart, concludes mesh rollout regressed, pages wrong team. Silent lie worse than no panel.

**Fix:** distinguish `loading | error | data` on both slices. Skeleton on loading, inline "mesh metrics unavailable — retry" pill on error. `MeshRollup` short-circuits to explicit "membership unavailable" rather than pretending everything is out of mesh.

---

## P0 — Cluster-wide vs scoped mesh numbers, same visual weight, both labeled with "%"

**Where:** `/app/ui/src/components/ClusterStatusView/index.tsx:192-227`, `ClusterMeshSummary.tsx:56-70`, `MeshRollup.tsx:89-101`.

**Why:** left column ("Cluster-wide truth · filters do NOT apply") and right column ("Current scope view") both render `nsPct%` / `wlPct%` in identical `20px fw-bold tnum`. Under stress operator scans two matching rows of big numbers side by side, reads them as the same measurement, then chases "why is 40% ≠ 47%". Disclaimer at `fs-12` drops off retina at 3am.

**Fix (hierarchy, not 3 lines):** either (a) collapse to one column when `scopeMatchesCluster === true` (already computed at `index.tsx:163`) or (b) restyle the scope column's number as a *delta* against cluster-wide ("47% ns · -6pp vs cluster") so the two objects stop competing.

---

## P1 — Triage journey (Q1): status page has no "changed since last poll" cue

**Where:** `ClusterStatusView/index.tsx` — `StatCards`, Mesh section.

**Why:** "did the rollout break something?" needs a delta. Every number here is a snapshot. Under 10s only if operator already knows the baseline — on a shared cluster they don't. This is what forces the operator to open two tabs and eyeball diffs.

**Fix:** cache last poll's `MeshMetrics` client-side; render `41 (-3)` when a counter shifts. No backend field required. Flag rather than implement now.

---

## P1 — Drilldown journey (Q4): chip click applies filter AND jumps to graph — chip promises more than graph delivers

**Where:** `ClusterStatusView/index.tsx:223` — `onSelect={(value) => { toggleMeshFilter(value); setView('graph'); }}`. Filter derivation at `store/filters.ts:38-45`.

**Why:** "STRICT: 41" chip counts current-scope workloads. Clicking flips to graph, which also applies whatever node-type / policy-source / direction / search filters were live. Operator lands on graph showing 12 STRICT workloads, can't reconcile with 41. Broken promise.

**Fix:** either narrow tooltip ("41 workloads · your other filters still apply") — one string, or reset non-mesh filters on that specific jump — policy call. Ask.

---

## P1 — Filter panel discoverability (Q2): "Mesh" filter dropdown collides with "Mesh overlay" toggle in Display

**Where:** `/app/ui/src/components/FilterPanel/parts/filter-dropdowns.tsx:388-449` (MeshDropdown) vs `/app/ui/src/components/FilterPanel/parts/display-dropdown.tsx:107-122` (Mesh overlay checkbox).

**Why:** two toolbar affordances say "Mesh". One filters *what shows up*, the other controls *how it's colored*. Same word, different verb → tool sprawl.

**Fix (1 line):** rename Display checkbox label at `display-dropdown.tsx:118` from "Mesh overlay" to "Color by mesh state". Verb "color" disambiguates.

---

## P1 — Detail panel journey (Q6): MeshCard shows *what*, not *why*

**Where:** `/app/ui/src/components/DetailPanel/shared/mesh.tsx:76-103` (`MeshCard`).

**Why:** "in mesh / not in mesh" answers "is this in mesh". The debugging question is "why is it not". Missing namespace `istio-injection` label? Pod label mismatch? Wrong revision? Ambient not enrolled ns? The card renders `provider · mode` on the success branch but goes blank on the failure branch — zero data for the case operators open the panel for.

**Fix:** data ask, not code. Backend gap — `MeshMembership.reason?: string`. UI has the slot below the chip; render one line. Flag, don't implement here.

---

## P2 — Empty-state journey (Q5): cluster with no mesh = permanent skeleton (or ambiguous zeros)

**Where:** `ClusterMeshSummary.tsx:27-28`.

**Why:** cluster with no mesh installed either returns null (skeleton stuck) or `workloadsEnrolled: 0` with zero mtls counts. In the second case the panel shows "0% enrolled · gray bar" — operator can't tell "we don't run a mesh" from "mesh is on fire". Opposite triage paths, same rendering.

**Fix (~3 lines):** when `workloadsTotal > 0 && workloadsEnrolled === 0 && mtlsStrict+Permissive+Disabled+Unset === 0`, render "No mesh detected in this cluster" block instead of skeleton+bar.

---

## P2 — MeshRollup buckets: `in-mesh` + `out` + four mTLS chips OR'd, but the strip reads as a partition

**Where:** `MeshRollup.tsx:78-85`.

**Why:** chip strip renders as a categorical breakdown but `in-mesh` already contains STRICT+PERM+DIS+UNSET. `MESH_FILTER_GROUPS` at `constants.ts:95-98` already declares the two axes; the rollup flattens them. Selecting `in-mesh` AND `mtls-strict` OR's per `filters.ts:38-45` — mental model mismatch.

**Fix (hierarchy):** split into two labeled rows (Membership: in/out — mTLS: STRICT/PERM/DIS/UNSET) matching `MESH_FILTER_GROUPS`, or drop the redundant `in-mesh` chip since the mTLS chips sum to it.

---

## P2 — Node selection cleared when mesh filter toggles

**Where:** `/app/ui/src/store/graphStore.ts:229-231` — `toggleMeshFilter` sets `selectedNode: null, selectedEdges: []`.

**Why:** operator has a workload panel open, refines mesh filter to narrow view, panel disappears mid-thought. Unlike namespace toggle (where the selection might actually fall out of view), mesh filter is a soft narrowing.

**Fix:** only clear when `meshStatus[selectedNode.id]` would drop the selection out of scope. Or leave as-is to match namespace pattern — ask.

---

## P3 — `ClusterMeshSummary` extracts colors via four `meshBadgeMeta` calls

**Where:** `ClusterMeshSummary.tsx:30-33`.

**Why:** `meshBadgeMeta` is a label formatter, not a palette. Read `MTLS_COLOR` directly from `data/policies.ts:52-57`. Code clarity nit, not user-visible.

---

## Cross-cutting

- Filter panel now has 8 dropdowns. Mesh addition itself is fine; the higher-leverage move is grouping **Filter** vs **Display** dropdowns — orthogonal to this branch.
- `MeshRollup.tsx:37` correctly skips `namespace` + `external` node types. Good — prevents ghost workloads inflating counts.
- `filters.ts:38-45` treats missing `meshStatus[id]` identically to `inMesh: false`. In the failed-fetch case this is the same silent lie as P0#1. Same fix covers both.

---

**Clarifying question**

**Should:** the status page Mesh section optimize for (a) *first-open scan* — one column when scope == cluster, two when a filter is active — or (b) *always-two-column parity* so operator's eye trains on stable layout? P0#2 pivots on this.
