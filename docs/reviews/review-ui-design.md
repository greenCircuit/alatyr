# ClusterStatusView + Mesh — Craft Audit

## 1. Mesh section screams louder than the danger callout above it — P0
- WHERE: `/app/ui/src/components/ClusterStatusView/index.tsx:187-227` (Mesh Section) sits **below** `ExposedCallout` (line 185). Mesh carries two tinted column backgrounds (`/app/ui/src/components/ClusterStatusView/Panel.module.css:57-65`), two colored left stripes (allow-green + info-blue), two uppercase colored eyebrows w/ emoji glyphs 🌐 / ⌕.
- WHY: Page scan target is "am I on fire". Tinted double-column Mesh block outweighs the single-stripe `ExposedCallout` above it. Eye lands on Mesh chrome first, not on the red exposed-count. Under stress the loudest surface must be the one that pages you.
- FIX: Strip `.meshColTruth` / `.meshColScope` backgrounds. Drop left stripes. Replace emoji headers w/ plain `.eyebrow` reading `CLUSTER-WIDE` and `IN CURRENT SCOPE`. The 12px hint below disambiguates. Two neutral columns divided by whitespace — same pattern as every other Section.

## 2. Four mesh-verdict presentations of the same primitive — P0
- WHERE: `/app/ui/src/components/ClusterStatusView/ClusterMeshSummary.tsx:100-103` (`swatch-dot` resized inline to 8×8 borderRadius 2); `/app/ui/src/components/ClusterStatusView/MeshRollup.tsx:141-147` (bordered pill w/ `●`); `/app/ui/src/components/PolicyGraph/PolicyGraph.module.css:33` `.meshMark` (STR/PERM/DIS/UNS/—); `/app/ui/src/components/DetailPanel/shared/MtlsChip.tsx` pill. `MtlsChip.tsx:3-5` claims it replaces the four dialects — it doesn't.
- WHY: Five renderings of `strict` across five surfaces. Casing drifts on the same page: `STRICT` uppercase, `permissive` lowercase, `DISABLE` uppercase, `unset` lowercase (`ClusterMeshSummary.tsx:45-49`). Ad-hoc, not systemic.
- FIX: `MtlsChip` becomes the only renderer. Add `size` prop (`xs|sm`). Cluster-summary legend and MeshRollup chip inner glyph both call `<MtlsChip variant="dot" size="xs">`. Verdict labels always uppercase mono (`STRICT/PERMISSIVE/DISABLE/UNSET`) — no mixed casing.

## 3. Four chip dialects in one card family — P1
- WHERE: `/app/ui/src/components/TablesView/IssueRollup.tsx:73-87` (bordered chip + `●` dot); `/app/ui/src/components/TablesView/EngineRollup.tsx:69-81` (bordered chip + `EngineLogo` 12px); `/app/ui/src/components/TablesView/StatusRollup.tsx:66-79` (bordered chip + **filled solid symbol square**, `/app/ui/src/components/TablesView/Rollup.module.css:20`); `/app/ui/src/components/ClusterStatusView/MeshRollup.tsx:133-147` (bordered chip + `●` dot, count via `ms-auto`).
- WHY: StatusRollup's filled `.symbol` block carries visual weight the others don't — reads as "priority" against neighbors on the same page. MeshRollup right-aligns count (`ms-auto`); the other three attach count inline. Widths jitter across the 3-col grid.
- FIX: One chip contract: `[glyph 10px] [label] [count]`. Dot for issue/mesh/severity, `EngineLogo` for engine. Kill filled `.symbol` in StatusRollup — use a dot. Count always attached to label, never `ms-auto`.

## 4. Type ladder collapses in card headers — P1
- WHERE: `/app/ui/src/components/ClusterStatusView/Panel.module.css:21-31` title=14/700, subtitle=11.5/regular. `/app/ui/src/components/ClusterStatusView/StatCards.tsx:32` value=30/700 inline. `/app/ui/src/components/ClusterStatusView/ClusterMeshSummary.tsx:56,67` = 20/700 inline. `/app/ui/src/components/ClusterStatusView/MeshRollup.tsx:91,97` = 20/700 inline. Section title (14) is smaller than every metric it sits above.
- WHY: Three ad-hoc sizes (30 / 20 / 14) do not compound into a scale. `/app/ui/src/components/DetailPanel/DetailPanel.module.css:236-249` already defines `body:13 / section:15 / hero:18`. This view ignores its own project's scale.
- FIX: One scale: `eyebrow 11 / body 13 / section 15 / metric 22 / hero 30`. Inline `fontSize: 20` in `ClusterMeshSummary` and `MeshRollup` → `.metric` class. Section title 14 → 15 to match `.section`.

## 5. MeshRollup flattens two semantic groups into one 6-chip row — P1
- WHERE: `/app/ui/src/components/ClusterStatusView/MeshRollup.tsx:78-86` — 6 buckets into `.gridRollup` (`/app/ui/src/components/TablesView/Rollup.module.css:47-51`, flex 33.333% - 4px). 2 membership buckets + 4 mTLS-verdict buckets rendered flat.
- WHY: Operator asking "how enrolled" scans membership; operator asking "am I mostly PERMISSIVE" scans verdicts. Flat mix forces re-parse each load.
- FIX: Two subgrids. Eyebrow `MEMBERSHIP` + row of 2 (in mesh / out). Eyebrow `MTLS` + row of 4. Same chip visual; grouping carried by eyebrow + whitespace.

## 6. Section header link is Bootstrap `btn-link` in a house style — P2
- WHERE: `/app/ui/src/components/ClusterStatusView/index.tsx:51-53` — `btn btn-link btn-sm p-0 fs-12 text-secondary` in every Section header (six occurrences).
- WHY: `btn-link` inherits Bootstrap blue on hover + underline decoration; against `#1a1f26` with `text-secondary` at rest and blue on hover the affordance flickers. `.ghostButton` in `/app/ui/src/components/DetailPanel/DetailPanel.module.css:531` exists for exactly this.
- FIX: Replace the six Section link actions with `.ghostButton` (lift the class to a shared module if needed).

## 7. Vertical rhythm inside the Mesh card jitters — P2
- WHERE: `/app/ui/src/components/ClusterStatusView/Panel.module.css:39-43` `meshInnerGrid` gap 16; `.meshCol` padding 14/16; `.meshCol` `gap: 12`; `/app/ui/src/components/ClusterStatusView/ClusterMeshSummary.tsx:52` outer `gap-3` (16), inner `gap-4` (24) between the two %s, `gap-3` (16) between title and bar.
- WHY: Adjacent gaps of 12/16/24 read as jitter. Whitespace must obey a single scale with one step between siblings.
- FIX: Within a card — `gap-2` (8) chip-to-chip, `gap-3` (12) row-to-row, `gap-4` (16) header-to-body. One scale across `ClusterStatusView/`.

## 8. Inline `style={{}}` leaks everywhere — P2
- WHERE: 27 hits under `/app/ui/src/components/ClusterStatusView/`: `ClusterMeshSummary.tsx:14,15,17,20,56,60,67,78,85,102`; `MeshRollup.tsx:91,97,108,115,121,141,143`; `StatCards.tsx:32,49`; `index.tsx:230,242,273`. Values mostly static: `{ height:10, borderRadius:4 }`, `{ fontSize:20 }`, `{ gridTemplateColumns:'1fr 1fr' }`.
- WHY: Project rule marks this as code + design smell. Static values in inline objects are exactly what `.module.css` was built to hold. Only genuinely data-driven values (segment %, bucket color) belong inline.
- FIX:
  - `gridTemplateColumns` values → `.grid5`, `.grid2`, `.grid13-1` in `Panel.module.css`.
  - `{ fontSize: 20/30, lineHeight: 1 }` → `.metric` / `.hero`.
  - mtls bar `{ height:10, borderRadius:4, overflow:hidden }` → `.mtlsBar` shared between summary + rollup.
  - Keep inline where the value IS the data (segment width %, bucket color).

## 9. Skeleton shape ≠ real DOM — P3
- WHERE: `/app/ui/src/components/ClusterStatusView/ClusterMeshSummary.tsx:11-25`.
- WHY: Skeleton = two 80×24 side-by-side + 10px bar + four 60×10 chips. Real content = two stacked (%, sub-label) pairs + bar + five-item legend. Layout shift on load.
- FIX: Mirror real DOM — two stacked pairs (22px placeholder + 12px sub-line), 10px bar, five legend chips.

## Cut list
- Emoji glyphs 🌐 / ⌕ (`index.tsx:197,210`) — cross-OS variance in an operator dashboard.
- `.meshColTruth` / `.meshColScope` tinted bg + left stripes (`Panel.module.css:57-65`).
- Filled `.symbol` square in StatusRollup (`Rollup.module.css:20-26`).
- `.chipPill`, `.chipFlat` variants (`Rollup.module.css:61-88`) if unused after unification.
- `btn btn-link` action in Section header — Bootstrap primitive in a house style.
- Inline `· N partial` in `ClusterMeshSummary.tsx:59-63` — a caution mini-chip or drop it.
