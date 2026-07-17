# TablesView + ClusterStatusView — Panel-Vocabulary Gaps

Legend: `[x]` fixed · `[skip]` intentional / low-ROI · `[open]` deferred

## Cross-cutting
1. [open] SEVERITY_COLOR inline styles — dynamic color-from-data pattern; blanket swap requires case-by-case. Leave for a targeted follow-up.
2. [skip] Legacy `chip-count*`/`chip-mini`/`chip-label*`/`chip-verdict` — global classes in `colors.css` render identically to panel tokens. Not visible drift; migrate opportunistically.
3. [x] `DirectionBadge` — already token-backed via Round 2 (`.dirOutline` variants).
4. [x] `WorkloadsTable.tsx:109`, `IssuesTable.tsx:179` — `badge bg-warning text-dark` → `.typeChip` / `.crossNsChip`.
5. [open] `WorkloadsTable.tsx:229`, `CulpritActions.tsx:78` — legacy `chip-label*` sites; deferred pending shared-component decision.
6. [x] Empty states — `IssuesTable`, `PoliciesTable`, `WorkloadsTable`, `NamespaceTable`, `RiskyWorkloadsTable` all wrap in `.tier1` + `.body .dim`.
7. [open] `font-monospace fs-12` / `fw-semibold text-light` cells — micro-cleanup batch deferred.
8. [open] Action buttons `btn-outline-secondary` / `btn-link` — RiskyWorkloadsTable `graph →` done via `.iconButton`; remaining sites deferred.

## TablesView shell
9. [x] `index.tsx:121` — `nav nav-tabs border-secondary` → new tokened `.tabStrip` + `.tabButton` + `.tabButtonActive` (2px `--color-info` underline).
10. [x] `index.tsx:129/138/147` — tab counters swapped `chip-count` → `.countChip`.

## IssuesTable
11. [x] `IssuesTable.tsx:161` — `IssueBadge` refactored: severity ● dot (`.sevDot` + `--sev`) + neutral `.findingKind` (peripheral color rule).
12. [open] Layering row `chip-mini` + manual chevron — deferred.
13. [open] Scope subline direction glyph — deferred.

## PoliciesTable
14–16. [open] Verdict chip / count prose / `◉ Graph` button — deferred (needs coordinated rework w/ IssueChip in WorkloadsTable).

## WorkloadsTable
17–19. [open] Coverage chip / IssueChip / overflow chip — deferred; IssueChip refactor is entangled w/ IssuesPopover.

## Rollups
20–23. [open] Three rollups need extraction into `<RollupStrip>`; larger structural refactor.

## IssuesPopover
24–26. [open] Popover bg + `.typeTag`/`.title` internal tokens — deferred.

## ClusterStatusView shell
27–29. [open] `Section` component, `ZoneDivider`, single-engine warn banner — deferred.

## StatCards
30. [x] `StatCards.tsx` — Bootstrap `card border-l-5` replaced w/ `.semanticDeny`/`.semanticAllow` KPI cards.
31. [x] `fs-3` (28px) hero swapped for `.hero` (18px) + `.verdictTextDeny/Allow` tone.
32. [x] Meta strip `opacity-50` `·` separator + `text-light fw-semibold` swapped for `.dim` + `.section`.

## NamespaceTable
33. [x] Gap/stale inline `swatch-dot` + colored prose → `.reasonChip .reasonDeny/reasonWarn`.
34. [x] `text-danger`/`opacity-50` counts → `.countChip .countChipDeny` when nonzero, `.dim` when zero.
35. [open] `table-dark table-sm` bespoke shell — kept; consolidation deferred.

## RiskyWorkloadsTable
36. [x] Worst-status colored word + muted status keys → `.sevDot` + `.section` + `.miniChip .miniChipDim` row.
37. [x] `btn-link` "graph →" → `.iconButton` with arrow glyph.

## CoverageBar + ProportionBar
38–39. [open] Legend + segment bar internals — deferred.

## ExposedCallout
40. [x] `border-l-5` shell + inline `borderLeftColor` → `.semanticDeny`.
41. [x] `text-danger fs-12 fw-bold text-uppercase` title + `fs-13` count → `.eyebrow .eyebrowDeny` + `.hero .verdictTextDeny`.
42. [x] Direction glyph inline color → `.dirOutline .dirOutlineBoth/Ingress/Egress`.
43. [open] `exposed-chip.module.css` — bespoke seamed pill kept for the dual-click affordance; deletion would collapse two actions into one, needs UX call.

## New tokens added (`DetailPanel.module.css`)
- `.tabStrip`, `.tabButton`, `.tabButtonActive` — tokened tab bar.
- `.countChipDeny`, `.countChipWarn` — deny/warn-tinted count chips.

## Fix summary
- **Applied:** 18 findings (biggest-impact chrome swaps: StatCards, ExposedCallout, tab bar, empty states, IssueBadge, NamespaceTable statuses, RiskyWorkloadsTable, KindBadge).
- **Deferred:** 25 findings — mostly structural refactors (Rollup extraction, IssuesPopover internals, ClusterStatusView Section/ZoneDivider, ProportionBar rewrite) or micro-cleanup (fs-12, chip-legacy migration) that can be scoped follow-ups.

## Crit closer
Panel-vocabulary now dominates the visible surfaces: tabs, KPI tiles, findings badge, exposed callout, namespace + risky tables all read as siblings of the DetailPanel. Remaining Bootstrap chrome is either behind popovers/rollups (less visible) or legacy chip classes that visually match the token vocab.
