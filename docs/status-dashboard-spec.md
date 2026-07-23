# Cluster Status Dashboard — Design Brief

Purpose of this document: hand to a design/frontend teammate so they can propose a coherent visual system. Not an implementation spec — a product + content spec. Style is deliberately open.

---

## Problem

A platform/SRE operator opens the app because a mesh/policy change may have broken something in a shared Kubernetes cluster. They need to answer three questions in this order, within seconds:

1. **Is something on fire right now?** (blocking issues, exposed workloads with no policy)
2. **How much of the cluster is actually protected?** (policy coverage, mesh enrollment, mTLS posture)
3. **Where do I start drilling?** (which namespaces / workloads are worst)

The current status page shows all the right numbers but reads as a wall of stacked sections. Operators can't tell:
- Which sections belong together
- Which numbers are cluster-wide truth vs filter-scoped snapshots
- Which chip rows are clickable filters vs read-only breakdowns
- Where one section ends and the next begins

The page is meant to be **glanceable** — a 10-second scan, not a report. Today it reads as "admin console with all the data" rather than "situation room dashboard."

---

## Users

- **Primary**: platform / SRE engineer investigating a suspected policy regression or a scheduled mesh rollout. Knows Kubernetes primitives cold. Reads at 3am under pressure.
- **Secondary**: security engineer auditing posture (mTLS enforcement, workloads exposed to internet, defense-in-depth gaps).
- **Tertiary**: developer curious about their namespace. Reads once, drills once, leaves.

All three want the SAME first read: red things first, green things second, drilldowns last.

---

## Goals

- Reduce time-to-triage to under 10 seconds for the "is anything on fire?" question.
- Make trust in the numbers obvious: it must be visible at a glance which numbers are cluster-wide (absolute truth) vs filter-scoped (current view).
- Make affordance obvious: clickable filter vs read-only breakdown must be distinguishable without hovering.
- Keep the page one screen tall on a typical operator laptop (~13-14" at reasonable zoom) — scrolling is acceptable for drilldowns, not for the headline.

## Non-goals

- Configuration / editing. This is a read-only surface.
- Historical trends / time-series. That belongs in a metrics tool.
- Multi-cluster comparison. One cluster per page.
- Full workload/policy listings — those live in the Tables view. Status page links out.

---

## Content — what goes on the page

Sections listed in scan order.

### Zone 1 — POSTURE (headline: is it on fire?)

**Headline numbers (5 metrics)**
Purpose: single glance answer to "any red anywhere?"
- `Issues` — count of actionable findings; sub-line names blocking count. Turns red when blocking > 0.
- `Exposed & unpoliced` — workloads facing internet with no workload-level policy; sub-line names affected namespaces. Turns red when > 0.
- `Workloads` — total in scope. Neutral.
- `Policies` — total in scope. Neutral.
- `Namespaces` — selected/total ratio. Neutral.

**Exposed callout** (conditional, only when count > 0)
Purpose: name the worst offenders inline so operator can click straight to detail. Shows up to 4 chips with ingress/egress direction, then "See all" link. This is the ONE always-loud attention block on the page.

**Mesh** (single section, two sub-scopes)
Purpose: mesh enrollment + mTLS posture is a first-class concern equal to policy posture.
- Cluster-wide (ignores filters — source of truth): namespaces enrolled ratio · workloads enrolled ratio · mTLS distribution across enrolled workloads. Bars + legend chips per row.
- Current scope (reflects filters, chips are clickable quick-filters): in-mesh ratio · scope-level chip row (in mesh / out / STRICT / PERMISSIVE / DISABLE / UNSET). Clicking a chip applies the mesh filter and jumps to the graph.

The two sub-scopes coexisting is the trust-critical part. Operator MUST see instantly that the top block is unfiltered truth and the bottom block reflects their filter selections. Different denominators, same labels — miscommunication risk is high.

### Zone 2 — COVERAGE (what does the policy layer look like?)

**Issues**
Chip row per issue type, colored by severity, count per chip. Chips are clickable — toggle the issue-type filter across the app (graph + tables). Zero-count chips stay visible (muted) because "0 policy conflicts" is a positive signal.

**Policies by engine**
Chip row per policy engine (k8s NetworkPolicy, Istio AuthorizationPolicy). Chips are clickable engine filter. Small warning callout below if any workload is covered by only one engine (defense-in-depth gap).

**Rule coverage**
Segmented proportion bar + legend chips for policy classes (restricted / deny-all / allow-all-ns / allow-all / unenforced / audit). Legend chips are read-only (not filters) — informational breakdown of how tight the rules are.

**Node statuses**
Segmented bar of worst-severity per workload (a true partition) + a chip rollup of status keys underneath. Status-key chips are clickable filters.

### Zone 3 — DRILLDOWNS (where do I click first?)

**Top risky workloads**
Ranked table, worst-first. Row click opens node detail; secondary action opens the graph centered on that node.

**Namespaces**
Table sortable by workload count / issue count / coverage. Row click applies namespace filter + jumps to the graph.

---

## Interactions

- **Filter chips** (Issues, Engines, Statuses, Mesh scope) — toggle a store filter. Effect propagates to graph, tables, and this page. Clicked chip on Status page usually also switches to the view where the effect is visible.
- **Section right-side links** — deep-link to a specific Tables tab or the Graph. Present iff the section maps to a route. Absent = no link, don't invent one.
- **Table rows** — primary action opens detail panel, secondary opens graph.
- **Exposed chips** — split action: label side opens detail, arrow side opens graph.

---

## Constraints

- Dark theme (`bg-dark`, `text-light`). Bootstrap + custom utilities. Design tokens live in `ui/src/style/colors.css` and `ui/STYLEGUIDE.md`.
- Data volume: tens of namespaces, hundreds to low thousands of workloads. Chip rows must degrade gracefully when zero-count chips are shown.
- Data freshness: page re-fetches on refresh, no auto-poll. A "refreshing…" line appears briefly.
- Some data (mesh cluster-wide totals) comes from a server-side endpoint and is invariant to client filters; other data (in-scope rollups) is derived client-side from the filter store. Both must render side-by-side without confusion.
- The nav bar above this view already carries the filter scope indicator. This page should not restate the full filter state — just cue where a number is filter-scoped.

---

## What the current design gets wrong (open feedback we've had)

- Card treatment on top (`StatCards`) uses a different visual language than the rest of the page (chromeless sections). Feels like two designs bolted together.
- Zone dividers are hairline rules with an uppercase label — they separate but don't seal. Reader sees a long ribbon of sections rather than three distinct panels.
- Section headers are quiet. Titles feel similar in weight to body labels underneath, so scanning jumps to numbers, not to headers.
- Mesh section has two sub-scopes (cluster-wide vs filtered) that operators keep confusing — same labels, different denominators, adjacent blocks. Multiple attempts at explanatory copy have not made the difference obvious.
- Chip rows across the page look identical whether they're clickable filters or read-only breakdowns. Affordance is not encoded visually.
- Rollup styling (colored border per chip) is consistent within the Rollup family but foreign to the Coverage legend and Mesh scope chips — page has two chip languages.
- Attempts to "elevate zones into panels" made the page look like a boxed enterprise console. Attempts to strip cards made it look like a wireframe. Neither pass felt right.

---

## Questions for the styling teammate

1. What's the one visual system this page should use? Options we've considered:
   - Chromeless with strong typographic hierarchy (auditor's recommendation, tried, still felt flat)
   - Panels-as-zones (tried, felt boxed / enterprise console)
   - Cards-per-section (rejected as too fragmented)
   - Something else — a hybrid?
2. How should headline numbers (Issues, Exposed) visually outrank the rest of the page? Color? Size? Position? Icon?
3. How should we signal "cluster-wide truth" vs "filter-scoped view" for the Mesh block without duplicating copy?
4. How should clickable filter chips visually differ from read-only breakdown chips? Border? Bg? Cursor alone isn't enough (mobile / touchpad users).
5. What's the right density? Current page feels either too dense (walls of text) or too sparse (metric strip looks empty). Where's the middle?
6. Is a 3-zone layout even right? Or should this be one continuous scan without explicit zones?

---

## Reference files (for the teammate)

- `ui/src/components/ClusterStatusView/index.tsx` — page composition, Zone/Section/Metric helpers.
- `ui/src/components/ClusterStatusView/StatCards.tsx` — headline metric strip.
- `ui/src/components/ClusterStatusView/ClusterMeshSummary.tsx` — cluster-wide mesh block (bars + legend).
- `ui/src/components/ClusterStatusView/MeshRollup.tsx` — filter-scope mesh chip row.
- `ui/src/components/ClusterStatusView/CoverageBar.tsx` — rule coverage bar + legend.
- `ui/src/components/ClusterStatusView/ExposedCallout.tsx` — red attention block.
- `ui/src/components/ClusterStatusView/RiskyWorkloadsTable.tsx`, `NamespaceTable.tsx` — drilldown tables.
- `ui/src/components/TablesView/IssueRollup.tsx`, `EngineRollup.tsx`, `StatusRollup.tsx` — shared rollup chip family.
- `ui/src/components/TablesView/Rollup.module.css` — chip styling; `.chip` bordered clickable, `.chipFlat` (present, unused) informational.
- `ui/src/components/ClusterStatusView/Panel.module.css` — zone panel styling (last attempt).
- `ui/STYLEGUIDE.md` — design token reference.
- `ui/src/style/colors.css` — semantic color palette (allow / deny / warn / severity tiers).
