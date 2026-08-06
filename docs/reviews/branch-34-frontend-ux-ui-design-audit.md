# Branch 34 (Calico Global Policy) — Frontend & UI Design Audit

## Section 1 — Frontend UX (frontend-ux agent)

### Finding 1: Policies tab count badge shows edge count, not row count

- **Severity:** Medium
- **Impact:** The number shown on the "Policies" tab header does not match the number of rows the operator sees in the table. A Calico GlobalNetworkPolicy that has both allow and deny rules, or that covers multiple directions, generates multiple edges but collapses to a single row in `PoliciesTable`. The count will reliably exceed the visible row count whenever any Calico policy exists, which is every cluster this branch targets.
- **Use case:** Operator glances at the tab badge to understand scope before clicking in. Sees "Policies 47" on the tab, opens it, counts 31 rows. No error message, no explanation. They either assume rows are filtered out or distrust the counter permanently.
- **Location:** `ui/src/components/TablesView/index.tsx:154`
- **Alternative:** Drive the badge from the grouped row count, not the raw edge array length. After `tableEdges` is computed, pass it through the same grouping key (`policySource|namespace|policyName`) used in `PoliciesTable` to get a `uniquePolicies` count and display that instead.

This is a pre-existing inconsistency between raw edge count and rendered row count that the Calico rollout meaningfully amplifies. Before this branch, most policies were either pure-allow or pure-deny, so the gap between edge count and row count was small. A Calico GlobalNetworkPolicy that legitimately mixes allow and deny selectors in one object is exactly the case that widens it. On a cluster with a default-deny GNP plus a handful of allow GNPs, the ratio of edges-to-rows can easily be 3:1 or higher, making the badge actively misleading during an audit session.

---

### Finding 2: EngineRollup active+empty chip states conflict visually

- **Severity:** Medium
- **Impact:** When an operator selects Calico as their active engine filter but the current namespace or direction selection yields zero matching Calico policies, the chip simultaneously renders with both `.active` (subtle background highlight) and `.empty` (0.5 opacity). The result is a semi-transparent chip that signals "this filter is active" and "nothing is here" at the same time, two states that contradict each other. An operator scanning the strip cannot tell if zero policies is an expected result of their filter combination or a sign Calico data failed to load.
- **Use case:** Operator has selected only the "gitlab" namespace in the FilterPanel. They click the Calico engine chip to pivot. The namespace has no Calico policies. The chip goes active+opaque, then the `edgesPreEngine` feed returns zero Calico edges, and the chip renders active+receded — visually indeterminate.
- **Location:** `ui/src/components/TablesView/EngineRollup.tsx:73`, `ui/src/components/TablesView/Rollup.module.css:20`
- **Alternative:** When `count === 0 && active`, suppress `.empty` and instead append a zero count with a distinct secondary tint rather than reducing opacity on the whole chip. The chip should clearly read "active, zero results" not "semi-active, maybe here." A simpler path: only apply `.empty` when `!active`. The active+empty combination can never be more informative than active alone, since the count already shows 0.

The decision to keep all engine chips visible at zero count (rather than hiding them) is correct — "calico: 0" is a positive signal that the engine registered but found nothing in this filter scope. The implementation breaks down only when the zero-count engine is also the active filter: the `.empty` opacity was intended for inactive chips that carry information value without being a current pivot target. The fix is one condition change in the `className` expression on line 73.

---

### Finding 3: "narrowed by" label renders without a guard on empty culprits in CIDR scope mismatch row

- **Severity:** Low
- **Impact:** In `CulpritActions`, the `cidrScope` branch renders the static text "narrowed by" unconditionally, then maps over `culprits`. If `culprits` is empty, the label hangs alone with no policy chips following it. The backend currently always populates `egressCulprits` or `ingressCulprits` for this issue type (the issue only fires when `narrow != ""`), but the frontend offers no protection if that invariant relaxes or if the issue arrives through a future code path that omits culprits.
- **Use case:** Operator opens the Issues table for a CIDR scope mismatch, expecting to see "narrowed by [chip]." Sees only the orphaned label with nothing after it. No crash, but the row is unreadable — "narrowed by" with no target looks like a rendering bug.
- **Location:** `ui/src/components/TablesView/CulpritActions.tsx:224`
- **Alternative:** Gate the "narrowed by" label exactly as "range allowed by" is gated: `{culprits.length > 0 && (<><span ...>narrowed by</span>{culprits.map(...)}</>)}`. One line change, defensive, matches the `allowed` guard pattern already present on line 216 in the same branch.

---

### Finding 4: Action column sort mixes lexicographic and numeric ordering for multi-action rows

- **Severity:** Low
- **Impact:** Sorting the Policies table by the Action column produces `allow` (0) → `allow+deny` (0,1) → `deny` (1) when sorted ascending. This is alphabetically reasonable but semantically surprising: operators who sort by action to group allow policies together will find Calico GNPs (which emit both actions) interleaved between pure-allow and pure-deny rows rather than grouped with either. Descending sort gives the inverse: deny-only, then mixed, then allow-only — which is actually useful for finding the most restrictive policies, but the column header "Action" does not communicate that ascending means "least restrictive first."
- **Use case:** Operator wants to see all deny policies in one block to audit what Calico is blocking. They click Action to sort descending. Mixed-action rows land in the middle, not with the denies.
- **Location:** `ui/src/components/TablesView/PoliciesTable.tsx:94`
- **Alternative:** For sort purposes, classify a multi-action row by its most restrictive action (max of its `actions` array). A policy with both allow and deny is effectively a deny-capable policy. Changing the sort comparator to `sign * (Math.max(...a.actions) - Math.max(...b.actions))` groups mixed-action Calico policies with deny rows on descending sort, which matches how operators reason about risk.

This is a minor triage inconvenience rather than a data integrity issue — the rendered badge list correctly shows both actions. The sort behavior only matters during audit sessions where the operator is using column sort as a quick filter substitute.

---

### Finding 5: Aggregated edge banner in EdgeView uses passive prose where the operator needs to act

- **Severity:** Low
- **Impact:** When clicking an aggregated Calico arrow (one that stands for N per-workload rules), the EdgeView panel renders: "Stands for N per-workload rules — the policy selects every workload in this namespace, and all N resolved this same verdict." This is accurate but written as an explanation of the rendering decision, not as guidance on what the operator is looking at or what they should check. An operator landing here at 2am after a reachability failure needs to know immediately whether this single displayed verdict represents all workloads faithfully or whether some workloads in the namespace have non-uniform rules that were excluded from this aggregated view.
- **Use case:** Operator clicks an arrow from namespace A to a CIDR block. They see the aggregated-edge banner. They do not know whether there are workloads in namespace A whose egress to that CIDR resolved differently and are therefore NOT represented by this arrow.
- **Location:** `ui/src/components/DetailPanel/views/EdgeView.tsx:35-41`
- **Alternative:** Reframe the banner text to address the operator's actual question: "All N workloads in this namespace resolved this verdict identically. Workloads with non-uniform rules appear as separate arrows." This makes the aggregation contract explicit — if the operator sees only one arrow and wonders about outliers, the banner answers it directly.

---

### Finding 6: ManifestModal header uses "cluster-scoped" as a namespace substitute but it is not a namespace

- **Severity:** Nit
- **Impact:** The YAML drawer header for a Calico GlobalNetworkPolicy renders `cluster-scoped/policy-name` in the monospace subtitle line, mimicking the `namespace/name` pattern used for namespaced objects. This is intended to be informative, but "cluster-scoped" is not a namespace — presenting it in namespace position trains operators to read it as one. An operator unfamiliar with GNPs might try to `kubectl get` the policy by appending `-n cluster-scoped` and fail.
- **Use case:** Operator opens the YAML drawer for a GNP, reads `cluster-scoped/egress-default-deny`, copies the name portion for a `kubectl` invocation.
- **Location:** `ui/src/components/DetailPanel/shared/ManifestModal.tsx:206`
- **Alternative:** Use a distinct layout for cluster-scoped objects: show the name on its own line with a `(cluster-scoped)` qualifier badge rather than borrowing the namespace slot. For example: `egress-default-deny` as the primary mono text, and a secondary small badge `cluster-scoped · GlobalNetworkPolicy` below it. This matches what `kubectl describe` outputs and avoids borrowing the namespace position for a non-namespace value.

---

## Nits

- `ui/src/components/TablesView/index.tsx:21` — a blank line was added between the import block and the `EMPTY_ISSUE_TYPES` constant. No semantic impact, minor style inconsistency.

- `ui/src/components/PolicyGraph/parts/bundling.ts:75` — the edge label suffix `all ${aggregatedFrom} workloads` appears in the Cytoscape arrow label on the graph canvas. At the font size graph labels render (~10px), the number followed by "workloads" will be truncated on most bundled arrows. Consider abbreviating to `×${aggregatedFrom}` for the canvas label, reserving the full phrase for the EdgeView panel where space exists.

- `ui/src/components/TablesView/PoliciesTable.tsx:143` — Calico GlobalNetworkPolicy rows render `—` in the Namespace column. This is correct, but operators sorting by namespace will find GNPs sorted with rows that have no namespace set (blanket-rule edges from k8s default-denies also emit empty namespace). The column header alone does not tell the operator that `—` could mean "cluster-scoped GNP" or "blanket rule with no peer." The engine badge adjacent to the row disambiguates this, but only if the operator knows what to look for. No change required — just documented here as a reading-context note for future work if the table grows a scope column.

---

## Section 2 — UI Design (ui-design-auditor agent)

This audit covers the visual and craft dimensions of the Calico engine addition: iconography fidelity, color token discipline, component-state differentiation, information hierarchy in the modified table surfaces, and the new `cidr scope mismatch` issue presentation. Files examined via `git diff main...HEAD` plus full-file reads where context required it. The styleguide at `ui/STYLEGUIDE.md` was used as the authoritative baseline throughout.

---

### Finding 1: Calico brand color bypasses the engine token system

- **Severity:** High
- **Impact:** The Calico orange (`#ff7b1c`) lives only in `engines.ts:19` as a hardcoded hex. It is never registered as a `--color-engine-calico` CSS custom property in `colors.css`. The k8s and istio engine hues each have corresponding `--color-engine-k8s` / `--color-engine-istio` tokens — plus `.text-engine-*` and `.bg-engine-*` utility classes — that components reach for when they need the hue for graph edge strokes, text coloring, or border tints. Calico has none. Any future component that references the Calico brand hue has no token to reach for: it will either hardcode the hex again (creating token drift) or silently fall back to the unknown-engine grey `#6c757d`.
- **Use case:** Any surface outside `EngineBadge` that needs Calico's brand hue — graph edge stroke, per-engine verdict stripe in a future detail card, or a `text-engine-calico` utility in panel prose.
- **Location:** `ui/src/style/colors.css:19–21`, `ui/src/data/engines.ts:19`
- **Alternative:** Add `--color-engine-calico: #ff7b1c;` to the `:root` block directly below `--color-engine-mesh`, then add the three utility classes (`.bg-engine-calico`, `.text-engine-calico`) that match the k8s and istio entries. The `engines.ts` record can continue to hold the literal for `color-mix` tinting inside `engineIcons.module.css` — the token and the data value can coexist — but the token must exist for the system to be internally consistent.

The styleguide states at section 1: "Every color goes through a `--color-*` var — never inline hex outside this file." The Calico brand color currently exists only as a string literal in a TypeScript record. This is a system-integrity failure: the engine token vocabulary has a gap shaped exactly like the third engine that was just added.

---

### Finding 2: Zero-count chip and dimmed chip are visually indistinguishable

- **Severity:** Medium
- **Impact:** `Rollup.module.css` defines `.chip.empty { opacity: 0.5; }` and `.chip.dimmed { opacity: 0.45; }`. The visual distance between these two states is 5 percentage points of opacity — imperceptible in practice, and certainly imperceptible under the stress of an incident. An operator who has never seen the EngineRollup with a zero-count Calico chip will not distinguish "calico exists but zero policies match this filter" (`.empty`, data signal) from "calico exists and I have selected a different engine" (`.dimmed`, filter artifact). Those two states carry meaningfully different information. They need distinct visual treatments, not near-identical opacity values.
- **Use case:** TablesView EngineRollup strip when the user filters to a namespace that has no Calico policies while other engines have matches. The `.empty` chip and the `.dimmed` chip sit side by side at 0.5 vs 0.45 opacity.
- **Location:** `ui/src/components/TablesView/Rollup.module.css:17–20`, `ui/src/components/TablesView/EngineRollup.tsx:73`
- **Alternative:** Keep `.dimmed` at `opacity: 0.45` for the filter-driven case. Give `.empty` a distinct axis: `filter: grayscale(55%)` at full opacity, or `border-style: dashed` instead of solid. Desaturation reads as "no data" while dimming reads as "deprioritized" — exactly the semantic split needed. The existing opacity collapse conflates them.

The decision to keep zero-count chips present rather than hiding them is correct and well-reasoned in the inline comment. The execution undermines it: receeding to 50% opacity when `dimmed` is already 45% does not recede, it almost duplicates.

---

### Finding 3: Calico paw icon loses definition at `size=10` — lighter visual weight than siblings

- **Severity:** Medium
- **Impact:** The paw print icon at `size=10` (its use in EngineRollup chip buttons) has four ellipses (rx=1.7, ry=2.1) spread across a 16×16 viewBox plus a central pad shape. At 10px rendered size, the four ellipses become sub-pixel specks and the mark's mass fragments. By contrast the k8s heptagon (solid polygon fill plus radiating spokes) and the Istio sail (two overlapping filled paths sharing the full height of the viewBox) both read as single dense units at `size=10`. The paw distributes area into five disconnected shapes with significant dead space in the mid-left and mid-right quadrants of the viewBox. Three engines in the same strip should read at equal optical weight so the operator's eye does not pre-weight any engine.
- **Use case:** EngineRollup chip row (`size=10`), EngineBadge in PoliciesTable Engine column (`size=12`), graph edge overlay label icons.
- **Location:** `ui/src/data/engineIcons.tsx:42–57`
- **Alternative:** Two options. Option A: Reduce toe spread — move outer toe cx values from 4.0/12.3 to approximately 4.8/11.5 and increase the pad shape width, so the mark reads as one dense cluster rather than four dots above a blob. Option B: If `size=10` is a confirmed production case, shift to a simpler 2-shape abstraction at small scale. A compact filled shield or rounded-square with a contrasting "C" cutout renders with more mass per pixel than organic curves at 10px. The paw concept is architecturally sound — the Calico brand's cat imagery makes it the right metaphor. The issue is purely photometric: the mark needs more mass-per-unit-area to hold equal weight against its siblings at the smallest rendered size.

---

### Finding 4: `cluster-scoped/name` in ManifestModal header borrows namespace position for a non-namespace value

- **Severity:** Medium
- **Impact:** When a Calico GlobalNetworkPolicy has no namespace, the manifest drawer subtitle renders `cluster-scoped/name`. This string mimics the `namespace/name` path format but "cluster-scoped" is a scope adjective used as a namespace placeholder. An operator who reads `cluster-scoped/allow-egress-dns` will parse the left side of the slash as a namespace named "cluster-scoped". They will attempt `kubectl get globalnetworkpolicy -n cluster-scoped allow-egress-dns` and fail. The existing `namespace/name` header format's readability is built on the invariant that the left side of the slash is always a real namespace string. Injecting a synthetic prefix corrupts that pattern.
- **Use case:** Manifest drawer header when the user clicks the YAML button on any Calico GlobalNetworkPolicy row in PoliciesTable.
- **Location:** `ui/src/components/DetailPanel/shared/ManifestModal.tsx:206`
- **Alternative:** Render the name on its own line and append scope as a qualifier, not a path prefix:

```
GlobalNetworkPolicy            ← headerKind (.section class, unchanged)
allow-egress-dns               ← name only, no slash
(cluster-scoped)               ← small .dim chip below, or inline after name
```

This matches `kubectl describe` output — which operators use as their mental model for Kubernetes object identity — and stops borrowing the namespace slot for a non-namespace value.

---

### Finding 5: CIDR range text rendered in `dim + smallText` — quieter than the chip it precedes

- **Severity:** Medium
- **Impact:** For `cidr scope mismatch` issues, `CulpritActions` renders `issue.message` (which contains the two CIDR masks — the substance of the finding) in `${s.smallText} ${s.dim}` above the culprit group rows. The comment in the code correctly notes "the exact masks ARE the finding here." Yet the `reasonWarn` chip that follows ("Scope mismatch") is rendered with a colored background tint and higher contrast. Information hierarchy is inverted: the chip is louder than the content it summarizes. The operator's eye lands on the tinted chip label first, then has to search upward in reduced contrast to find the actual masks.
- **Use case:** Issues table, expanded `cidr scope mismatch` row showing CulpritActions.
- **Location:** `ui/src/components/TablesView/CulpritActions.tsx:376`
- **Alternative:** Render the message in `${s.body}` at full contrast. Where the message contains CIDR notation (`10.0.0.0/8`), wrap those tokens in `<code className="chip-mini">` so the masks render in monospace and are scannable at a glance. The warn chip below is still correct as a condition label; the message above it is the value — the value should be louder than the label.

---

### Finding 6: `cidr scope mismatch` chip in IssueRollup lacks the Calico engine glyph

- **Severity:** Low
- **Impact:** The `ISSUE_ENGINE` map in `IssueRollup.tsx` assigns engine logos to mesh-family issue types (`mesh conflict` → Istio sail, `mesh transport blocked` → Istio sail, `mesh policy` → Istio sail) so operators can identify which system layer is responsible by scanning the chip row. `cidr scope mismatch` is a Calico-engine finding — it arises specifically from Calico GlobalNetworkPolicy CIDR ranges overlapping another engine's ipBlock. The chip currently renders with a plain colored dot (the else branch at line 85), losing the engine attribution that makes the mesh-family chips scannable by shape, not just color.
- **Use case:** IssueRollup strip on the Issues tab when Calico is installed and produces CIDR-overlap findings.
- **Location:** `ui/src/components/TablesView/IssueRollup.tsx:18–22`
- **Alternative:** Add `'cidr scope mismatch': 'calico'` to the `ISSUE_ENGINE` partial record. One line. The chip then shows the Calico paw alongside the warning-hue border, consistent with how mesh-family chips carry the Istio sail.

---

### Finding 7: Two `fontWeight: 600` inline styles on durable permitted-direction spans

- **Severity:** Nit
- **Impact:** Lines 177 and 199 in `CulpritActions.tsx` apply `style={{ fontWeight: 600 }}` on spans whose content never changes at runtime. `fontWeight: 600` is not data-driven — it is a fixed presentational value that belongs in a CSS module class. The styleguide at section 8 is unambiguous: "Never `style={{}}` for anything durable."
- **Use case:** The green "✓ allowed" and "✓ ns-level allow verified at pod level" lines in `CulpritGroup` permitted and layering branches.
- **Location:** `ui/src/components/TablesView/CulpritActions.tsx:177`, `ui/src/components/TablesView/CulpritActions.tsx:199`
- **Alternative:** Add `.allowVerdict { font-weight: 600; }` to `DetailPanel.module.css` and apply `${s.allowVerdict}` alongside the existing `text-allow` class. Two inline overrides become one reusable token entry.

---

### Cut list

1. `cluster-scoped/` namespace-borrowing prefix in `ManifestModal.tsx:206` — replace with name-only plus a scope qualifier; the slash asserts a hierarchy that does not exist.
2. The two `style={{ fontWeight: 600 }}` inline overrides in `CulpritActions.tsx:177,199` — move to a module class, consistent with existing module discipline.
3. The `--color-engine-calico` token absence — add to `colors.css` before the next engine surface is built on top of a missing foundation.

---

### Crit closer

The Calico engine addition is structurally sound — icon, badge, rollup chip, issue type, and culprit renderer all follow the established patterns with genuine craft — but the one color token that would make Calico a first-class member of the engine token vocabulary (`--color-engine-calico`) was never registered, which means the visual system has a hole at the exact location the new engine should anchor.

---

## Conclusion

The branch successfully introduces Calico GlobalNetworkPolicy as a third policy engine alongside `k8spolicy` and `istio`, and the integration is structurally faithful to the existing multi-engine patterns: a dedicated engine identifier, an icon in `engineIcons.tsx`, a filter row wired through `FilterPanel/parts/constants.ts`, and full participation in the TablesView rollup pipeline. There are no critical regressions in the diff, no broken flows, and no data-integrity confusions that would mislead an operator about whether traffic is allowed or denied. The frontend-ux and ui-design audits converged on the same overall verdict: the change is safe to merge, but a small cluster of near-miss issues deserves attention before Calico is presented as a first-class engine in a customer-facing demo.

The highest-leverage items across both sections are the missing `--color-engine-calico` design token in `colors.css` (UI-design Finding 1) and the `cluster-scoped/name` namespace-slot reuse in `ManifestModal.tsx:206` (both audits, independently). The color token gap is the kind of foundation crack that will be paid for every time a future engine surface is built — a badge tint, a filter chip background, a graph edge color — because each site will hardcode `#ff7b1c` again rather than referencing a variable, and the first time design wants to shift the Calico hue the change will require a repository-wide grep. The namespace-slot issue is the only finding with real risk of an operator misusing the output: an engineer copying `cluster-scoped/policy-name` into `kubectl get -n cluster-scoped policy-name` will get a "namespace not found" error and briefly wonder if the tool is broken. Both are cheap to fix and should ship in the same follow-up.

The medium-severity findings cluster around two themes. First, the TablesView counting and sorting logic (frontend-ux Findings 1 and 4) was written when every engine emitted one row per policy per action; Calico GlobalNetworkPolicy routinely emits both allow and deny rules from a single policy object, which breaks the assumption. The badge count in `index.tsx:154` and the action sort in `PoliciesTable.tsx:94` both need to become row-aware rather than edge-aware. Second, the EngineRollup chip states (frontend-ux Finding 2 and UI-design Finding 2) have overlapping opacity semantics — `active + empty` renders as a visually broken chip, and `empty` at `0.5` versus `dimmed` at `0.45` is imperceptible — which will be more visible now that most clusters will have three engines with genuinely different presence patterns (e.g. Istio installed but no policies yet, Calico installed and populated, k8spolicy sparse).

The low-severity and nit findings are polish rather than function: an unguarded label in `CulpritActions.tsx:224`, an aggregated-edge banner that describes rather than acts, dim CIDR mask text that inverts the intended hierarchy, a missing engine glyph on the new issue type in `IssueRollup.tsx`, and two inline `fontWeight: 600` overrides that belong in a module class. None of these will bother an operator during a bug hunt, but together they are the difference between a third engine that feels native and one that feels bolted on. Recommended sequence: land the color token and the namespace-slot fix immediately, address the TablesView row/edge counting in the same PR since it touches related code, and treat the remaining chip-state and polish items as a single visual-consistency follow-up before the branch is presented externally.

