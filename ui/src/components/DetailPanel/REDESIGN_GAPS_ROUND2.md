# DetailPanel Redesign Gaps — Round 2

Legend: `[x]` fixed · `[skip]` data-plumbing / backend needed · `[open]` deferred

## Cross-cutting

1. [x] `ManifestModal.tsx:116` — ManifestButton Bootstrap `btn btn-sm btn-outline-secondary` → `.ghostButton`.
2. [x] `rows.tsx / edge-composites.tsx` — `fst-italic` sites folded into new `.catchAll` token.
3. [x] `edge-composites.tsx:246` — dropped `text-start text-light`; `.cardButton` now sets `text-align:left` + `color:inherit`.
4. [x] `rows.tsx / edge-composites.tsx / issues.tsx` — inline `style={{flex:1,minWidth:0}}` swept to new `.flexFill`.
5. [x] Inline `style={{marginTop:N}}` swept via `gap` on `.card` (now flex-column gap-1); remaining WorkloadView `mt:8` between details summary + body kept (not inside `.card`).
6. [x] `badges.tsx:181` — legacy DirectionBadge rewired to `.dirOutline` variants (egress/ingress/mesh/both).
7. [skip] StalenessStamp — backend/fetch-time plumbing needed (`lastEvaluated` timestamp on graph response).
8. [open] Findings inline PolicyRef vs CulpritActions engine chrome — CulpritActions shared w/ Issues table + carries per-direction reason/allowed/layering data; slimming down risks table regression. Defer to scoped rewrite.

## Workload

9. [x] `WorkloadView.tsx:142-151` — count capsule order swapped to `in / out` per mockup.
10. [x] `WorkloadView.tsx:153` — inline `marginLeft:auto` → `.pushRight`.
11. [skip] Verdict text glyph — mockup ships identical `✗/⚠/✓` prefix (`panels.tsx:664-670`); LIVE keeps.
12. [skip] MeshCard SA / principals / port overrides — `MeshMembership` type (`data/policies.ts`) lacks `serviceAccount` + `principals`/`notPrincipals` fields. Backend feed change needed.
13. [x] `mesh.tsx:16` MtlsSourceRow — dropped `.cardAllow` stripe (double signal alongside `effective` miniChip); neutral `.card` shell + chip.

## Edge

14. [x] `EdgeView.tsx:37` — single-policy branch now wraps in `d-flex flex-column gap-2`.
15. [x] `edge-composites.tsx:112-124` — EdgeReachabilityBanner switched to `.verdictCallout .verdictDeny/Allow/Warn` + `.verdictText*` tone classes; dropped inline color-mix + `color/fontWeight`.
16. [x] Covered by #15 (same inline color-mix removed).
17. [skip] deadLetter port warn — `PolicyEdge` no `dst.listensOn` field; would require backend surface.
18. [open] EdgeReachabilityBanner `Blocked by:` culprit inline — needs picking first deny engine's first culprit from `result.engines`; defer to iteration.

## Reachability

19. [x] SubsystemChip + BidirectionalChip refactored: SubsystemChip → EngineBadge + ActionIcon; BidirectionalChip → `.reasonChip` variants. Inline color-mix + `bg` gone.
20. [x] `DirectionBlock` — replaced `.card` + inline borderLeft w/ `.semanticDeny/Allow/Warn` shell keyed on `dir.reason`; section labels now `.eyebrowDeny/Allow/Warn`.
21. [x] `EngineCard` — `.card` + inline borderLeft → `.semanticDeny/Allow/Warn` + `.reasonChip` verdict.
22. [x] `SelectingPolicyChip` — inline borderLeft + `opacity-75` → semantic shell keyed on `state` + `.cardMuted` token + `.reasonChip` label.
23. [x] Inline `color: var(--color-deny/allow/caution)` on section labels swapped for `.eyebrowDeny/Allow/Warn` (covered by #20).
24. [x] `ReachabilityView.tsx:551` — `<summary>` now uses `.disclosureSummary` alone (dropped 10px eyebrow affordance).
25. [x] `PortsSummary` — dropped per-row `.card .cardFlush` wrapper; flat rows in flex-column.
26. [x] Swap button inline `alignSelf:center` dropped; `.endpointRow > *:nth-child(2)` centers via token.
27. [skip] MeshSideCard PA chain `effective` list — `MtlsState.sources` exists; render already lives in `MtlsBlock` (MeshCard). Adding to compact `MeshSideCard` in reach column duplicates it. Defer decision.

## Cut list — applied

- `text-light`, `text-start`, `fst-italic`, `opacity-75` — gone from panel files (grep confirms).
- Inline `marginTop|marginBottom|alignSelf|marginLeft:auto` — swept, remaining occurrences justified.
- Custom `SubsystemChip` / `BidirectionalChip` internals — folded to token vocab.
- Bootstrap `.badge` in `DirectionBadge` — gone.
- Inline `borderLeftColor` on reachability cards — gone (DirectionBlock/EngineCard/SelectingPolicyChip).

## Deferred summary

- **Data plumbing** (backend/feed changes): #7 staleness stamp, #12 mesh SA/principals, #17 deadLetter port.
- **Scope decisions**: #8 findings culprit inline (shared component risk), #18 EdgeBanner blocked-by (iteration), #27 MeshSideCard PA chain (duplication vs MeshCard).
