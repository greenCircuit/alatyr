# FilterPanel toolbar — UX + SRE review

Two independent agent audits of the current two-row toolbar. Read both, decide.

Current state:
- **Row 1 (chrome):** `[◉ Graph | ▤ Tables | ▦ Status] [Search 180px] [⚙ Display (graph-only)]                  [↻ Refresh]`
- **Row 2 (data filters):** `[Namespace] [Status] [Source] [Action] [Direction] [Issue]`
- Display dropdown groups: Layout + NS view + Connections + Engine icons
- Both rows use `flex-wrap`

---

## Agent 1 — frontend-ux (UX/design lens)

### 1. Holds up 20x/day?

Mostly yes. Two-row split is the right call: chrome vs data. **Top-1 problem still visible: row 2 filters have no "active vs default" summary and no one-click reset.** All six anchors turn amber when narrowed, but after a session of clicking, an operator can't tell at a glance *what* is filtering the graph. Rebuilding trust means opening each dropdown. That is the 3am tax.

### 2. Row 1 coherence

Coherent, one thing misplaced: **Search sits between view-tabs and Display**, which reads as "part of the tab group" on first scan. Search is a global tool; it deserves either `ms-auto` grouping with Refresh, or a visible divider from the tabs. Also — Display is graph-only, so on Tables/Status the row collapses awkwardly and Refresh jumps left. Reserve the slot or anchor Refresh differently so the row doesn't reflow between views.

### 3. Row 2 order

Current: Namespace, Status, Source, Action, Direction, Issue. Two things:
- **Issue at the end is wrong.** It is the loudest signal (red anchor when conflicts exist) and the most common entry point for a debugging session. Should be first, or last with a visible separator.
- **Action + Direction are edge-shape filters** and belong adjacent. Source is also an edge attribute. Group:
  `[Namespace] [Status] | [Source] [Action] [Direction] | [Issue]`
  Node scope, then edge scope, then diagnostics. Current order splits Source from its siblings by nothing.

### 4. First-time-user breakage

- **Anchor labels lie about semantics.** "No engines" / "No direction" / "None" (Action) look like states the user chose, not "you filtered everything out — graph is empty." Needs a danger tint, not warning.
- **Status anchor default is "Filter by status"** while every other anchor defaults to a "showing all" phrasing ("All engines", "Ingress + Egress"). Inconsistent — reads like Status is off vs. others being on.
- **Issue anchor mixes two meanings in one count** (`5 conflicts` = data, `2 conflict types` = filter). Two different nouns behind one number.

### 5. Top 3 changes, ranked

1. Add a **"Filters: N active — Reset"** inline summary at the left of row 2 (or a subtle chip strip under it). Solves the "what is filtering me right now" problem without opening six dropdowns.
2. **Reorder row 2** to put Issue first (or add a `|` gap before Issue). Put the debugging entry point where the eye lands first.
3. **Normalize anchor label grammar and empty-state color.** Every anchor: `All X` when unfiltered, `N / M X` when narrowed, `No X (empty)` in **danger** red when the user zeroed it. Status anchor especially.

### Clarifying question

Is the pain closer to *"I can't tell what's filtering the graph"* (audit rule 1 → do change #1 first) or *"I keep hunting for the right control"* (audit rule 3 → do change #2 first)?

---

## Agent 2 — backend-sre (target-user lens)

### Candidate layouts scored

- **A) Current** — two rows as above
- **B) Reordered row 2** — `[Issue] [Namespace] [Status] | [Source] [Action] [Direction]`
- **C) Single row + Filters mega-dropdown** — every data filter behind one anchor with active count
- **D) Two rows + active-filter chip strip** — current + chip strip below row 2 showing narrowed filters + Reset-all

### 1. Preferred: D

Two rows + chip strip. On-call at 3am, I need to see *what I've narrowed to* without re-reading six dropdown labels. Chips make stale filter state obvious — **the #1 cause of "the graph is lying to me" is a filter left on from the previous investigation.** C hides state behind a popover, which is exactly wrong when tired. B just reshuffles; same cognitive load.

### 2. Per-flow winners

- **"Pod A can't reach pod B, what's blocking?"** → D. Namespace=nsA,nsB → chip strip confirms scope → Direction=egress → Status=isolated. Chips make the "did I actually scope this?" check instant. C costs an extra click per filter change while iterating.
- **"All namespaces with deny-all?"** → any layout — one filter (Status=deny-all-ingress). Tie. C slightly worse (extra click to open).
- **"Any policy conflicts before shipping?"** → B or D. Issue filter as leftmost entry point (B) or visible-as-chip (D) beats hunting the 6th dropdown at 2am.

### 3. Single highest-leverage addition

**Active-filter chip strip with per-chip X + Reset-all.** Not keyboard shortcuts, not saved views. The failure mode I hit is *invisible filter state*, and one glance at chips kills it. Reset-all is the "get me back to ground truth" button every debug tool needs.

### 4. Missing from the whole toolbar

- **Timestamp of last fetch** next to Refresh. "Is this graph 30s old or 30min old?" matters when someone just applied a policy.
- **Namespace filter needs multi-select with search** — tens of namespaces, scrolling a flat list is painful. *(Note: the search input already exists inside NamespaceDropdown — SRE agent didn't spot it.)*
- **No "exclude" mode** on Namespace/Status. "Show me everything *except* kube-system/istio-system" is the daily filter.
- **No permalink/shareable URL** of current filter state. Paging a teammate means screenshots instead of a link.

---

## Convergence — what both agents agree on

- **Active-filter visibility is the biggest gap.** UX calls it "audit rule 1"; SRE calls it "invisible filter state." Same problem, same fix — a chip strip / summary showing narrowed filters + one-click reset. Both rank it top priority.
- **Row 2 order needs rework so Issue is not last.** UX wants grouping by scope (node/edge/diagnostic); SRE wants Issue as leftmost entry point.
- **Two-row layout is fine — do not consolidate into a Filters mega-dropdown.** SRE explicitly rejected option C ("hides state behind a popover — exactly wrong when tired").

## Where they diverge

- UX raises **anchor label grammar** (Status inconsistency, "No X" reading as user choice not empty-state) — SRE doesn't mention it. Real bug but lower blast radius than filter visibility.
- SRE surfaces **operational-context adds** (last-fetch timestamp, exclude-mode, permalink) — UX doesn't. Those are session-productivity features, not toolbar layout, but they'd land here.

## Ranked candidate next steps

1. **Active-filter chip strip + Reset-all** — both agents' #1. Highest leverage.
2. **Reorder row 2** — Issue-first or `[node scope] | [edge scope] | [diagnostics]` grouping with separators.
3. **Normalize anchor labels + empty-state danger tint** — UX rule 4, first-time-user clarity.
4. **Reserve Display slot in row 1** so Tables/Status view doesn't reflow (Refresh jumps left otherwise).
5. **Last-fetch timestamp next to Refresh** — SRE's operational ask.
6. **Exclude-mode + permalink** — bigger scope, defer.

## Open questions before deciding

- Do we want the chip strip *below row 2* (own line, most visible) or *inline at row 2 left* (saves vertical space)?
- Is "Reset" scoped per-chip only, or also a big "Reset all filters" button?
- Do row 2 separators (`|`) between filter groups justify the visual noise, or is order enough?
