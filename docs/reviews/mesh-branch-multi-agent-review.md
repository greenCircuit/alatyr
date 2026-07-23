# Branch Review — `43-add-istio-worload-membership-when-buildong-graph`

Three specialized agents reviewed the branch: backend correctness, frontend user journeys, and visual craft. Findings from each are captured verbatim below, with an added `Why valid` (independent take on why the finding matters), `Use case` (concrete operator scenario), and `Fix` (actionable step) per item. Cross-agent conclusions at the end.

Scope of the branch: Istio ambient mesh membership computed backend-side per workload, exposed via `/api/mesh-status` + `/api/mesh-metrics`, wired into the graph store, filter panel, workloads table, detail panel, and a redesigned cluster status page.

---

## Section 1 — Backend SRE review

Reviewer: `backend-sre`. Focus: Go pipeline correctness, cache safety, API surface, issue-integration false positives, test coverage.

### P0-1 — Workload-level ambient opt-in on a non-ambient namespace is silently dropped
- **Finding**: `internal/mesh/istio/buildMeshMembership.go:42-50` gates the whole workload branch on the namespace label. A workload carrying `istio.io/dataplane-mode=ambient` inside a non-enrolled ns is stamped `InMesh: false`. `inAmbientMesh` in `internal/mesh/istio/utils.go:54` documents "workload label wins over namespace label" — this path violates the contract. `GetWorkloadMesh` uses the correct helper, so the on-click detail panel disagrees with the graph.
- **Why valid**: This is the single most damaging class of bug for this feature — the frontend renders the backend's word as ground truth. Two disagreeing sources of "in mesh?" on the same page destroy operator trust the day it happens.
- **Use case**: Team X enrolled a single workload into ambient ahead of a full-ns rollout, using the workload label. Graph shows it as OUT of mesh. Detail panel shows IN. Operator can't reconcile, escalates.
- **Fix**: Call `inAmbientMesh(node.Labels, nsLabels)` per workload in every branch. Only skip PA fetching when the ns has zero enrolled workloads — never skip label evaluation.

### P0-2 — `IngressNamespace` typo (`"istio ingress"` vs `istio-ingress`)
- **Finding**: `internal/mesh/istio/istio.go:44` — real cluster ns is `istio-ingress`; constant contains a space. Every `participatesInMesh` / `Membership` check against this constant misses in production. Gateway pods never get stamped `InMesh` via the ns rule; HBONE-15008 gate on rules touching them false-clears. No test uses the real value.
- **Why valid**: One-character constant that only fires on real cluster names. Tests silently pass. Simple to fix; easy to miss forever.
- **Use case**: Istio ingress gateway pods appear as "not in mesh" on every install. Operator debugging ingress-side mTLS chases ghosts.
- **Fix**: Change constant to `"istio-ingress"`. Add a fixture in `buildMeshMembership_test.go` using the real ns name.

### P1-3 — HBONE validator resolves ingress peer via `DstID` not `SrcID`
- **Finding**: `internal/mesh/istio/detect.go:159-162` — same `cache.Workload(rule.DstID, rule.DstNamespace)` for both directions. k8s ingress rules swap: `DstID` = protected workload, `SrcID` = peer. Gate always resolves the protected workload (in-mesh), so "peer not in mesh" skip never fires. Every ingress test uses in-mesh dst → CI is green, prod is wrong.
- **Why valid**: Correctness bug in a validator that emits `MeshTransportBlocked` issues. Bad issue text costs operator minutes per incident and erodes signal quality.
- **Use case**: NetworkPolicy allowing ingress from `0.0.0.0/0:8080` on an ambient workload gets falsely flagged. Operator opens a triage that has no cause.
- **Fix**: Ingress path: peer = (`SrcID`, `SrcNamespace`). Egress path: (`DstID`, `DstNamespace`). Add a test where SrcID = external out-of-mesh peer.

### P1-4 — Dead `len(rule.Ports)==0` check inside a port loop
- **Finding**: `internal/mesh/istio/detect.go:168, 184` — empty-check lives inside `for _, port := range rule.Ports`, so it never runs when Ports is empty. `detectGlobalHBONEOpen` currently masks the miss, but a restricted-peer + empty-ports stanza (peer-specific, all ports open) silently escapes.
- **Why valid**: Currently benign, but fragile — an unrelated change to the global detector would surface latent bugs here immediately.
- **Use case**: Future rule tightening removes the global fallback; suddenly a class of policies is unflagged and no one notices.
- **Fix**: Hoist the empty-check above the loop.

### P1-5 — Cache concurrency contract on mesh cache fields
- **Finding**: `internal/store/buildStore.go:193-204` — safe today because `handleGraph` holds the full write lock during populate. Three coupled fields (`MeshMembership`, `MeshMetrics`, `MeshIssues`) with in-place append assume single-writer.
- **Why valid**: Not reachable today. The moment a background refresh with overlapping ns sets is added, `MeshIssues` will double-append.
- **Use case**: Future feature adds a scheduled refresh. First refresh double-fires all mesh issues silently.
- **Fix**: Comment the invariant explicitly at the write site + add a `sync.Once`-style guard, or convert append to replace-in-place.

### P2-6 — Root PA-fetch failure issue lacks `Node`
- **Finding**: `internal/mesh/istio/buildMeshMembership.go:31-36` — issue emitted with no workload/ns anchor. UI filters that key on Node/Src/Dst drop it silently.
- **Why valid**: Silent issue = no operator ever sees a control-plane fetch failure. That's a page-worthy signal being suppressed by the UI it feeds.
- **Use case**: Istiod becomes unreachable; PA fetch fails; user sees ambient posture rendered as "0% enrolled" with no warning.
- **Fix**: Attach a synthesized root-ns marker Node to root-scope failure issues, or route to a dedicated `SystemHealth` rendering path.

### P2-7 — PERMISSIVE default correctly not flagged
- **Finding**: `buildIssues.go:153-172` MeshConflicts only fires on `Verdict=="deny"`. `CanReach` returns "allow" for PERMISSIVE dst against non-mesh src. No false positive.
- **Why valid**: Explicitly checked — this is the class of bug that would paint every fresh Istio install red. Confirmed absent.
- **Use case**: N/A — this is the "no regression" line item.
- **Fix**: None. Regression-test the assumption.

### P2-8 — Test coverage gaps
- **Finding**: No test for workload-ambient-on-non-ambient-ns (P0-1). No test for real `istio-ingress` value (P0-2). `ValidateExternalRules` ingress tests never exercise the k8s swap (P1-3). No test for `NSNode == nil` in `BuildMeshMembership` — would panic on `idx.NSNode.Labels`.
- **Why valid**: Each of the P0/P1 findings above is missing a specific test that would have caught it.
- **Use case**: PR containing any of the P0 fixes must land with the guarding test alongside, or it will regress.
- **Fix**: Add the four tests above. Cheap.

### P3-9 — Cleanup
- Commented imports at `internal/mesh/istio/istio.go:12-18`.
- Comment at `internal/models/cache.go:14-16` says "populated by store" but writer is `mesh.MeshSource.BuildMeshMembership`.

---

## Section 2 — Frontend UX review

Reviewer: `frontend-ux`. Focus: SRE operator journeys — triage, filter, trust, drilldown, empty-state, detail-panel.

### P0-1 — Silent failure on `fetchMeshMetrics` / `fetchMeshStatus`
- **Finding**: `ui/src/store/graphStore.ts:208-224` catch branches only stuff `error: String(e)`. `ClusterMeshSummary` treats `metrics == null` as *loading* — skeleton forever. `MeshRollup` renders every workload as "out of mesh" because `filters.ts:34-45` treats missing entry as `inMesh: false`.
- **Why valid**: Silent lie at the trust boundary. Reader has no way to know the numbers are fake. This is the single worst kind of UI bug for a triage tool.
- **Use case**: `/api/mesh-metrics` errors during a routine deploy. Operator sees `0% enrolled cluster-wide` and pages the wrong team about a mesh regression that isn't happening.
- **Fix**: Track `loading | error | data` per slice in the store. On error: inline "mesh metrics unavailable — retry" pill in place of the skeleton. `MeshRollup` short-circuits to explicit "membership unavailable" when `Object.keys(meshStatus).length === 0` AND the fetch state is `error`.

### P0-2 — Cluster-wide vs scoped mesh numbers at identical visual weight
- **Finding**: `index.tsx:192-227`, `ClusterMeshSummary.tsx:56-70`, `MeshRollup.tsx:89-101` — left and right columns both render `nsPct%` / `wlPct%` in identical `20px fw-bold tnum`. Disclaimer at `fs-12` disappears under stress.
- **Why valid**: This is exactly the bug the redesign was supposed to fix. Multiple passes have added tint, badges, icons — still failed the "two matching big numbers side-by-side" test.
- **Use case**: Operator scans "47% ns enrolled" left, "41% ns enrolled" right, tries to reconcile. Reads 6pp regression that doesn't exist.
- **Fix (structural)**: (a) collapse to one column when `scopeMatchesCluster === true` (already computed at `index.tsx:163`), or (b) restyle the scope column's number as a delta ("47% ns · −6pp vs cluster"). Both remove the "matching pair" pattern.

### P1-3 — No "changed since last poll" cue on triage
- **Finding**: Every number on the page is a snapshot. Operator must know the baseline to answer "did the rollout break something?" — on a shared cluster they don't.
- **Why valid**: The stated goal of the page is a 10-second triage scan. Snapshots alone force the operator to open two tabs and eyeball diffs, which defeats the point.
- **Use case**: 3am pager on a mesh rollout. Operator opens the page, sees "8/13 workloads enrolled". Is that up or down from an hour ago?
- **Fix**: Cache last poll's `MeshMetrics` in the client store; render `41 (-3)` when a counter shifts. Zero backend work required. Flag; don't implement in this branch.

### P1-4 — Mesh chip click applies filter AND jumps to graph — chip promises more than graph delivers
- **Finding**: `index.tsx:223` — `onSelect={(value) => { toggleMeshFilter(value); setView('graph'); }}`. Filter derivation at `store/filters.ts:38-45`. Other live filters (ns/engine/action/direction/search) still apply on the graph.
- **Why valid**: The chip count reflects the current scope, but landing on the graph applies FURTHER filters. Count and visible nodes diverge.
- **Use case**: Chip says "STRICT: 41". Operator clicks, graph shows 12 STRICT nodes because a namespace filter was active. Operator can't tell if the page is broken or the filter is misleading.
- **Fix**: Simplest — extend the chip tooltip: "41 workloads · your other filters still apply". Alternative: reset non-mesh filters on the specific jump. Ask policy call.

### P1-5 — Filter panel name collision: "Mesh" filter vs "Mesh overlay" display toggle
- **Finding**: `FilterPanel/parts/filter-dropdowns.tsx:388-449` MeshDropdown vs `display-dropdown.tsx:107-122` Mesh overlay checkbox. Two toolbar affordances say "Mesh". One filters what shows up; the other controls how it's colored.
- **Why valid**: Discoverability tax. Every operator who's never opened the display dropdown will look for the mesh filter there, not find it, look for the display toggle in filter, not find it.
- **Use case**: Operator wants to filter to STRICT. Opens Display panel first because "Mesh" caught their eye. Wastes 15 seconds.
- **Fix (1 line)**: Rename display checkbox from "Mesh overlay" to "Color by mesh state" at `display-dropdown.tsx:118`. Verb "color" disambiguates.

### P1-6 — MeshCard shows *what*, not *why*
- **Finding**: `DetailPanel/shared/mesh.tsx:76-103` renders `provider · mode` on success but goes blank on the failure branch. "In mesh / not in mesh" answers the wrong question.
- **Why valid**: The failure branch is when the panel gets opened — success cases don't need debugging. Missing exactly the data the operator opened the panel for.
- **Use case**: Workload shows OUT of mesh. Operator opens detail to answer "why?" — sees only the badge. Bounces to `kubectl describe`.
- **Fix**: Data ask — backend adds `MeshMembership.reason?: string` naming the failure (missing ns label / wrong revision / opted-out label). UI slot already exists below the chip. Flag; don't implement here.

### P2-7 — Cluster with no mesh = permanent skeleton OR ambiguous zeros
- **Finding**: `ClusterMeshSummary.tsx:27-28` — if fetch returns null → skeleton stuck. If it returns `workloadsEnrolled: 0` with zero mtls counts → renders "0% enrolled" identical to a broken mesh.
- **Why valid**: Opposite triage paths, same rendering. Operator can't tell "no mesh installed" from "mesh broken".
- **Use case**: Team demoing the app on a cluster without Istio. Sees "0% enrolled · gray bar". Files a ticket.
- **Fix (~3 lines)**: When `workloadsTotal > 0 && workloadsEnrolled === 0 && (mtlsStrict+Permissive+Disabled+Unset === 0)`, render "No mesh detected in this cluster" block instead of skeleton+bar.

### P2-8 — MeshRollup buckets read as a partition but are OR'd across two axes
- **Finding**: `MeshRollup.tsx:78-85` — 6 flat chips: `in-mesh` + `out` + 4 mTLS. `in-mesh` already sums STRICT+PERM+DIS+UNSET. `MESH_FILTER_GROUPS` in `constants.ts:95-98` already declares two axes; the rollup flattens them. Selecting `in-mesh` AND `mtls-strict` OR's per `filters.ts:38-45`.
- **Why valid**: Mental model mismatch. Reader assumes chips partition; behavior is set-union. Filter results contradict intuition.
- **Use case**: Operator selects "in-mesh" + "STRICT" expecting AND, gets OR. Result count doesn't match what they meant.
- **Fix (hierarchy)**: Split into two labeled rows — `Membership: in / out`, `mTLS: STRICT / PERMISSIVE / DISABLE / UNSET` — matching `MESH_FILTER_GROUPS`. Drop the redundant `in-mesh` chip since mTLS chips sum to it.

### P2-9 — Node selection cleared when mesh filter toggles
- **Finding**: `graphStore.ts:229-231` — `toggleMeshFilter` sets `selectedNode: null, selectedEdges: []`. Panel disappears mid-thought when narrowing.
- **Why valid**: Namespace-filter also clears (may drop the node out of view) — but mesh filter is a soft narrowing. Loss of context on a filter refine.
- **Use case**: Operator has a workload open, clicks "STRICT" chip to refine, workload disappears from panel. Loses their place.
- **Fix**: Only clear when `meshStatus[selectedNode.id]` would drop the selection out of scope. Or match namespace pattern — ask.

### P3-10 — `ClusterMeshSummary` extracts colors via `meshBadgeMeta` calls
- **Finding**: `ClusterMeshSummary.tsx:30-33` — `meshBadgeMeta` is a label formatter, not a palette. Should read `MTLS_COLOR` from `data/policies.ts:52-57`.
- **Why valid**: Code hygiene. Not user-visible.
- **Use case**: N/A.
- **Fix**: Swap the calls.

### Cross-cutting note
- Filter panel now has 8 dropdowns; the higher-leverage move is grouping Filter vs Display dropdowns — orthogonal to this branch.
- `filters.ts:38-45` treats missing `meshStatus[id]` identically to `inMesh: false`. In failed-fetch case this is the same silent lie as P0-1. Same fix covers both.

---

## Section 3 — UI Design review

Reviewer: `ui-design-auditor`. Focus: hierarchy legibility, consistency, typographic ladder, color system, motion, density, craft.

### P0-1 — Mesh section screams louder than the danger callout above it
- **Finding**: `index.tsx:187-227` (Mesh) sits below `ExposedCallout` (line 185). Mesh has two tinted column backgrounds (`Panel.module.css:57-65`), two colored left stripes (allow-green + info-blue), two uppercase colored eyebrows with emoji glyphs 🌐 / ⌕. `ExposedCallout` is a single-stripe attention block. Mesh chrome now outweighs Exposed.
- **Why valid**: Page scan target is "am I on fire". The loudest surface must be the one that pages you. Multiple color / chrome / emoji layers on Mesh have inverted the priority.
- **Use case**: 3am pager. Operator's eye lands on the two-column tinted Mesh block first, not on the red exposed count above it. Extra half-second scanning is the exact failure mode this page was supposed to prevent.
- **Fix**: Strip `.meshColTruth` / `.meshColScope` backgrounds. Drop left stripes. Replace emoji headers with plain `.eyebrow` "CLUSTER-WIDE" / "IN CURRENT SCOPE". Rely on the 12px hint below to disambiguate. Neutral columns divided by whitespace — same shape as every other Section.

### P0-2 — Four mesh-verdict presentations of the same primitive
- **Finding**: `ClusterMeshSummary.tsx:100-103` (`swatch-dot` resized inline 8×8), `MeshRollup.tsx:141-147` (bordered pill w/ `●`), `PolicyGraph/PolicyGraph.module.css:33` `.meshMark` (STR/PERM/DIS/UNS/—), `DetailPanel/shared/MtlsChip.tsx` (pill). `MtlsChip.tsx:3-5` comment claims it unifies all four — it doesn't. Casing drifts on the same page: `STRICT`, `permissive`, `DISABLE`, `unset`.
- **Why valid**: The primitive at the center of the whole branch is rendered 4-5 different ways. Casing drift on the same page reads as bug, not variety.
- **Use case**: Operator scanning STRICT count in three places — different visual, has to re-decode each. Reads slower.
- **Fix**: `MtlsChip` becomes the ONLY renderer. Add `size` prop (`xs | sm | md`) and `variant` (`dot | pill`). Cluster summary legend AND MeshRollup chip inner glyph AND graph badge all call it. Verdict labels always uppercase mono (`STRICT/PERMISSIVE/DISABLE/UNSET`).

### P1-3 — Four chip dialects in one card family
- **Finding**: `IssueRollup.tsx:73-87` (border + `●`), `EngineRollup.tsx:69-81` (border + `EngineLogo` 12px), `StatusRollup.tsx:66-79` (border + filled solid symbol square `Rollup.module.css:20`), `MeshRollup.tsx:133-147` (border + `●`, count via `ms-auto`).
- **Why valid**: One card family, four chip dialects. StatusRollup's filled `.symbol` reads as "priority" vs neighbors on the same page. MeshRollup right-aligns count; other three attach inline — widths jitter across a 3-col grid.
- **Use case**: Chip-heavy status page reads as busy because the eye keeps re-tuning to each dialect. Slower scan.
- **Fix**: One chip contract: `[glyph 10px] [label] [count]`. Dot for issue/mesh/severity, `EngineLogo` for engine. Kill filled `.symbol` in StatusRollup — use a dot. Count always attached to label, never `ms-auto`.

### P1-4 — Type ladder collapses in card headers
- **Finding**: `Panel.module.css:21-31` title 14/700, subtitle 11.5/regular. `StatCards.tsx:32` value 30/700 inline. `ClusterMeshSummary.tsx:56,67` = 20/700 inline. `MeshRollup.tsx:91,97` = 20/700 inline. Section title (14) is smaller than every metric it heads.
- **Why valid**: `DetailPanel/DetailPanel.module.css:236-249` already defines `body:13 / section:15 / hero:18`. This view ignores its own project's scale.
- **Use case**: Reader can't tell section title from a data value.
- **Fix**: One scale: `eyebrow 11 / body 13 / section 15 / metric 22 / hero 30`. Inline `fontSize: 20` in `ClusterMeshSummary` and `MeshRollup` → `.metric` class. Section title 14 → 15 to match `.section`.

### P1-5 — MeshRollup flattens two semantic groups into one 6-chip row
- **Finding**: `MeshRollup.tsx:78-86` — 2 membership + 4 mTLS-verdict chips in one flex row (`Rollup.module.css:47-51`, flex 33.333% - 4px).
- **Why valid**: (Same problem the frontend-ux review flagged from mental-model side.) Reader re-parses which chips answer which question each load.
- **Use case**: Operator asking "how enrolled" scans membership; operator asking "am I mostly PERMISSIVE" scans verdicts. Flat mix forces re-decoding.
- **Fix**: Two subgrids. Eyebrow `MEMBERSHIP` + row of 2 (in mesh / out). Eyebrow `MTLS` + row of 4. Same chip visual; grouping carried by eyebrow + whitespace.

### P2-6 — Section header link is Bootstrap `btn-link` in a house style
- **Finding**: `index.tsx:51-53` — `btn btn-link btn-sm p-0 fs-12 text-secondary` used in every Section header (six occurrences). `btn-link` inherits Bootstrap blue on hover + underline. Against `#1a1f26` bg with `text-secondary` at rest → blue on hover: affordance flickers.
- **Why valid**: `.ghostButton` in `DetailPanel/DetailPanel.module.css:531` exists for exactly this — this view rolled its own.
- **Use case**: Every hover on a section link makes the color jump. Distracts from the actual data.
- **Fix**: Replace six `btn btn-link` links with `.ghostButton` (lift class to a shared module).

### P2-7 — Vertical rhythm inside the Mesh card jitters
- **Finding**: `Panel.module.css:39-43` `meshInnerGrid` gap 16; `.meshCol` padding 14/16, `gap: 12`; `ClusterMeshSummary.tsx:52` outer `gap-3` (16), inner `gap-4` (24) between the two %s, `gap-3` (16) between title and bar. Adjacent gaps of 12/16/24.
- **Why valid**: Whitespace must obey a single scale. 12/16/24 reads as jitter.
- **Use case**: Card doesn't feel composed. Design polish.
- **Fix**: Within a card — `gap-2` (8) chip-to-chip, `gap-3` (12) row-to-row, `gap-4` (16) header-to-body.

### P2-8 — Inline `style={{}}` leaks everywhere
- **Finding**: 27 occurrences under `ClusterStatusView/`. Values mostly static: `{ height:10, borderRadius:4 }`, `{ fontSize:20 }`, `{ gridTemplateColumns:'1fr 1fr' }`. Project rules forbid this pattern for static values.
- **Why valid**: Style rule in CLAUDE.md — inline styles reserved for state-driven values. Static shape belongs in modules.
- **Use case**: Any theme change requires touching 27 sites instead of one CSS class.
- **Fix**: `gridTemplateColumns` → `.grid5 / .grid2 / .grid13-1`. Font sizes → `.metric` / `.hero`. mTLS bar shape → `.mtlsBar` shared between summary + rollup. Only genuinely state-driven values (segment width %, bucket color) stay inline.

### P3-9 — Skeleton shape does not match real DOM
- **Finding**: `ClusterMeshSummary.tsx:11-25` skeleton = two 80×24 side-by-side + 10px bar + four 60×10 chips. Real content = two stacked (%, sub-label) pairs + bar + five-item legend. Layout shifts on load.
- **Why valid**: Layout shift on first paint is a craft signal — the design didn't get double-checked.
- **Use case**: Operator sees content jump on refresh.
- **Fix**: Mirror real DOM in skeleton — two stacked pairs (22px placeholder + 12px sub-line), 10px bar, five legend chips.

### Cut list (design)
- Emoji glyphs 🌐 / ⌕ (`index.tsx:197,210`) — cross-OS variance in an operator dashboard.
- `.meshColTruth` / `.meshColScope` tinted bg + left stripes (`Panel.module.css:57-65`).
- Filled `.symbol` square in StatusRollup (`Rollup.module.css:20-26`).
- `.chipPill`, `.chipFlat` variants (`Rollup.module.css:61-88`) if unused after unification.
- `btn btn-link` action in Section header — Bootstrap primitive in a house style.
- Inline `· N partial` in `ClusterMeshSummary.tsx:59-63` — either a caution mini-chip or drop.

---

## Conclusions

### Cross-cutting themes

1. **The Mesh block is over-designed and over-loud** (backend P0 not withstanding). Both the UX review (P0-2) and the design review (P0-1) landed on the same finding from different angles: the two-column, tinted, iconed, emoji-headed Mesh section outshouts the actual alarm signal above it, and its numbers still get misread. Every design pass to make the distinction more obvious added chrome instead of removing it. The right move is subtractive — drop tint + stripes + emojis, use standard eyebrows + whitespace, and either collapse to one column when scope==cluster or restyle the scope value as a delta.

2. **Silent failure at every layer.** Backend P0-1 (workload label ignored on non-ambient ns), P0-2 (typo constant), and frontend P0-1 (fetch errors render as skeleton or "everything is out of mesh") are the same failure mode: a fault that renders as normal-looking data. Trust in the whole surface collapses the day either happens. Both need explicit failure states in the wire format and the UI.

3. **One primitive rendered N ways.** Backend P1-3 (peer resolution wrong for ingress) and design P0-2 (four mTLS renderings) reflect the same slippage: the primitive at the center of the branch — mesh membership + mTLS verdict — is inconsistently expressed across code paths. Both need a single source of truth (Go helper + `MtlsChip`).

4. **Test coverage is a lagging indicator.** Every P0/P1 backend finding has a matching missing test. The test suite is the map of what the design has forgotten.

### Merged priority stack

- **P0 (must fix before merge)**
  - Backend #1: workload ambient label wins over ns label (silent lie).
  - Backend #2: `IngressNamespace` typo.
  - Frontend #1: silent failure on mesh fetch.
  - Frontend #2 / Design #1: Mesh block visual weight + column read.

- **P1 (fix in this branch if possible)**
  - Backend #3: HBONE peer resolution direction bug.
  - Backend #4: dead port-empty check.
  - Design #2: unify mesh verdict rendering under `MtlsChip`.
  - Design #3: unify chip dialects across all four rollups.
  - Design #4: adopt project's own type scale.
  - Design #5: split MeshRollup into two axes.
  - Frontend #4: mesh chip → graph mismatch (tooltip or reset).
  - Frontend #5: "Mesh overlay" → "Color by mesh state" rename.

- **P2 (schedule)**
  - Backend #5: mesh cache write contract comment + future-proofing.
  - Backend #6: root PA-fetch issue needs a Node.
  - Backend #8: test coverage gaps (all four).
  - Frontend #6: MeshCard "why not in mesh" — data ask on backend.
  - Frontend #7: no-mesh empty state.
  - Frontend #8: MeshRollup partition mental model.
  - Frontend #9: node selection cleared on mesh filter.
  - Design #6: ghost button in Section header.
  - Design #7: rhythm inside Mesh card.
  - Design #8: inline style cleanup.

- **P3 (nice to have)**
  - Backend #9: import + comment cleanup.
  - Frontend #10: `meshBadgeMeta` color extraction.
  - Design #9: skeleton shape mirror.

### What's fine

- Mesh cache is single-writer-safe today under `handleGraph` write lock (backend P1-5, watch-item).
- PERMISSIVE default install is correctly NOT flagged as a warning (backend P2-7).
- MeshRollup correctly skips `namespace` + `external` node types (frontend cross-cutting).
- Zone / grid / card structure of the page is right — no reviewer flagged the layout system itself; feedback is on how the Mesh block INSIDE that system was executed.

### Recommended fix order

1. Land backend P0-1 + P0-2 with tests. Nothing else is safe until the backend stops lying.
2. Land frontend P0-1 (fetch error states). Same reason: UI must not lie either.
3. Subtractive Mesh redesign: strip column tints, drop emojis, single eyebrow per column, resolve the "matching pair of big numbers" problem via delta or collapse.
4. Unify mesh verdict rendering under `MtlsChip`.
5. Backend P1 fixes (peer direction, port-empty hoist).
6. UX polish batch (chip unification, type scale, ghost button).
7. P2 items on schedule.

### Meta

Three agents, three angles, one dominant conclusion: the Mesh feature works, but its trust surface leaks in three places at once (a silent backend fault, a silent frontend fetch failure, and an over-designed presentation that hides which number is authoritative). Fix those three together and the branch ships.
