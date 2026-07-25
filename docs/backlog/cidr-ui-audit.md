# CIDR UI audit — branch `44-better-cidr-managment-and-can-reach`

Two-agent audit of new CIDR-peer + reachability UI. Scope split:
- **frontend-ux** — user journey, information hierarchy, interaction patterns
- **ui-design-auditor** — visual craft, styling, hierarchy, color, motion

Both agents read the full unstaged diff and cross-referenced `docs/backlog/cidr-peer-modeling.md` + `ui/STYLEGUIDE.md`.

---

## Section 1 — Frontend UX findings

### 1.1 User journey — CIDR as first-class peer

- `ui/src/components/DetailPanel/views/WorkloadView.tsx:66-73` — CIDR nodes render the "Pin as source" button. Clicking pins a `cidr:...` id as `reachabilitySource`. The next graph click fires `fetchReachability(cidrId, cidrId, dstId, dstNs)` (`graphStore.ts:322-337`) with a CIDR id where a workload id is expected. Backend `PoliciesCanReach` (`internal/store/reachability.go:214`) will look up `NodeRules[cidrId]` (empty bucket) and return a mostly-empty "allow" verdict — user gets a green "can reach" verdict with no evidence, which reads as a broken feature. **Fix:** hide "Pin as source" when `node.type === 'cidr'`. If pinning-a-CIDR is desired long-term, ask backend to add a symmetric egress-only mode; for this branch, cut the button.
- `ui/src/components/DetailPanel/views/WorkloadView.tsx:38-83` — Hero "Healthy / At risk" verdict and StatusBadges strip render on a CIDR node. CIDR has no computed `statuses` (backend never derives status keys for `NodeTypeCIDR`), so `worst` is always `info` → renders bright green "Healthy". A `cidr:0.0.0.0/0` node showing "Healthy" is meaningless at best, misleading at worst. **Fix:** branch on `node.type === 'cidr'` before rendering the hero; show CIDR summary — raw CIDR, classification (internet / LAN / api-server / private), peer count. Kill the Mesh / NodeIssues / per-engine sections for CIDRs.
- `ui/src/store/graphStore.ts:341-344` — Clicking a CIDR node populates `selectedNode` but skips `loadNodeInfo` because `node.namespace === ''`. WorkloadView's "Per-engine evidence" section stays empty; no affordance from a CIDR node to "which workloads touch me" — the exact answer the operator opened it for. **Fix:** synthesize a peer list for CIDR clicks by scanning `allEdges` for `source === cidr.id || target === cidr.id` and rendering a NeighborList-style card locally (no fetch needed).
- `ui/src/components/DetailPanel/shared/edge-composites.tsx:61-72` — Endpoint subtitle for a CIDR reads `— · cidr`. Em-dash from `node.namespace || '—'` fallback. On EdgeView, the SRC or DST card shows a stray em-dash. **Fix:** when `node.type === 'cidr'`, render subtitle as just `cidr` and consider promoting classification word ("internet", "LAN", "cluster") into subtitle slot.

**Click path walk:** operator opens WorkloadView on their workload. CIDRs hidden inside `Per-engine evidence → k8s → Outbound (N)` behind two `<details>` toggles. Friction: CIDR peers indistinguishable from workload peers in `NeighborList`; no summary chip like "3 CIDR peers, 2 with carve-outs." Operator clicks CIDR peer chip → panel switches to CIDR's WorkloadView. **Dead-end:** no back button, no "peers touching this CIDR" list, no way to get to peers on other side.

### 1.2 Except / carve-out clarity

- `ui/src/components/DetailPanel/shared/badges.tsx:150-162` — `COVERAGE_META` has no `except` entry, so `CoverageBadge` returns `null` for every carve-out. An except-derived deny is a bare deny card everywhere `CoverageBadge` renders (`rows.tsx:404, 494`). **Fix:** add `except` entry: `{ cls: coverageWarn, text: 'except (carve-out)', title: 'Hole in a parent allow — this range is subtracted from the allow policy above.' }`.
- `ui/src/components/DetailPanel/shared/rows.tsx:387` — `NeighborGroup` renders ActionIcon as red `✗ deny` for `coverage === 'except'`. Row shows a red-stripe deny card, red ✗ icon, no coverage chip. Semantically wrong: this is a carve-out from an allow the operator wrote intentionally. Operator will think another policy is blocking them and go hunting. **Fix:** when `rule.coverage === 'except'`, use distinct icon (amber `⊖` or scissor), warn stripe (not deny stripe), force coverage badge to render. Card should read "carve-out from <parent policy>".
- `ui/src/components/DetailPanel/shared/rows.tsx:13-44` — `PolicyRow` in `EdgeView` renders no CoverageBadge at all. An except-rule edge, clicked from graph, shows as plain red deny card with no cue this is a carve-out. **Fix:** add `<CoverageBadge coverage={p.coverage} />` to PolicyRow header. Same one-liner fix as `NeighborGroup` at line 404.
- `ui/src/style/edgeStyles.ts:99-115` — Graph edge styling is the one place except is distinguished (amber + dotted). Good. But DetailPanel never learns from that vocabulary. **Fix:** carry amber-dotted metaphor into DetailPanel — on any card representing an except rule, add `border-left: dotted amber` stripe to reinforce "same thing you clicked on the graph."
- `ui/src/style/edgeStyles.ts:87-115` — Order-of-precedence risk in Cytoscape stylesheet. `edge[action = 1]` selector applies to every deny edge; `edge[coverage = "except"]` applies immediately after. Except edges have `action = 1` too, so both selectors match. Cascade means later `coverage = "except"` block wins for named properties, but `line-color` and `target-arrow-color` from `action = 1` only overridden because re-declared. Stray property added to `action = 1` later could quietly flip an except edge back to red. **Fix:** define selector as `edge[action = 1][coverage = "except"]` explicitly, mirror every property `action = 1` sets.

### 1.3 Reachability view UX

- `ui/src/components/DetailPanel/views/ReachabilityView.tsx` — View doesn't recognize when DST is a CIDR. Backend returns `ingressEval.Reason === 'no-opinion'` for CIDR destinations. Lands in `DirectionBlock` (line 250) as amber "no opinion" reason on ingress side of the grid — reads like a data gap. **Fix:** when DST is CIDR, replace two-column egress/ingress grid with single-column "egress to CIDR" panel. Ingress-side answer is nonsensical for CIDR target.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:130-180` — `PortsSummary` treats DST as workload with ingress ports. For CIDR DST, "DST ingress" column always renders "none" — pairing with SRC's real egress ports produces confusing "SRC opens 443 → DST accepts none" chip pair. **Fix:** for CIDR DST, render one row: SRC egress ports. Drop arrow and DST side.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:113-122` — SubsystemChip strip lists Istio + k8s equally when DST is CIDR, but Istio never emits CIDR rules today (per design doc §10-11 — parity out of scope). Users will see "istio: not enforced" alongside "k8s: allow" and try to correlate. **Fix:** hide Istio subsystem chip on CIDR reachability queries, or replace with "n/a for CIDR peer" tooltip.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:511-547` — Empty-state copy doesn't acknowledge CIDR targets are valid. Placeholder reads "click a **workload**", teaching wrong mental model. **Fix:** update copy to "Click a workload or CIDR peer to check reachability."

### 1.4 CIDR node in graph

- `ui/src/components/PolicyGraph/parts/styles.ts:101-118` — Barrel shape + dashed-blue border distinguishable from workloads at scan speed. Silhouette check passes: nothing else uses barrel. Good.
- `ui/src/components/PolicyGraph/parts/styles.ts:107-117` — `width: label` + `padding: 14` means `cidr:169.254.169.254/32` node is ~2× wider than workload with short label like `redis`. Multiple CIDR peers stack vertically (LR dagre) on right edge, each variable-width. With 5+ CIDRs (common: internet + api-server + metadata + LAN + LAN carveout) right side becomes wall of variable-width barrels. **Fix:** cap width or wrap long CIDRs. Alternatively cluster CIDRs into compound "CIDR peers" pseudo-namespace.
- `ui/src/components/PolicyGraph/parts/bundling.ts:26-52` — Bundling keys by `(source, target, action, coverage)` — no bundling by target alone. Shared internet CIDR touched by 30 workloads produces 30 separate arrows converging on one CIDR node. Bezier curves overlap, arrowheads pile up. **Fix:** for CIDR targets, "aggregate fan-in" mode drawing single thick edge from CIDR-side ns-box to CIDR, labelled "30 workloads → 10.0.0.0/8". Alternatively add opt-in "hide CIDRs, show as status badge on workload" toggle.
- `ui/src/components/PolicyGraph/parts/elements.ts:69` — CIDR nodes never enter a compound parent (empty namespace, no `parent` assignment). Float at graph top-level alongside `nsbox-*` compounds. Visually compete with namespaces for "outside world" mental model. **Fix:** synthesize compound parent `cidr-peers-box` (labelled "External CIDRs") owning all CIDR nodes.
- `ui/src/components/PolicyGraph/parts/elements.ts:100-190` — Aggregate-by-namespace mode leaves CIDRs ungrouped. Every unique CIDR stays separate node. If user aggregated to reduce noise, seeing 8 CIDR nodes untouched defeats the purpose. **Fix:** in aggregated mode, collapse CIDRs into synthetic "CIDR peers (N)" node, or better, into "internet CIDRs" and "private CIDRs" using backend's `policy/networkPolicyHelpers.go` classification helpers.

### 1.5 Filter / selection story

- `ui/src/store/graphStore.ts:108` — `cidr` added to `ALL_TYPES`, but no UI surface toggles node types. `toggleNodeType` and `selectedNodeTypes` exposed on store, zero components call `toggleNodeType`. Filter dropdowns in `ui/src/components/FilterPanel/parts/filter-dropdowns.tsx` cover namespaces, statuses, policy sources, actions, directions, mesh, issues — no node-type dropdown. **Users cannot hide CIDR nodes.** CIDRs push display density much higher. **Fix:** add node-type dropdown. Minimum for this branch: single "Show CIDR peers" toggle in display dropdown.
- `ui/src/store/filters.ts:48-77` — No filter for internet-CIDR vs LAN-CIDR vs api-server-CIDR. Backend has classification helpers; frontend never uses them. User asking "what internet-facing egress do I have?" has to eyeball every CIDR node. **Fix:** compute classification client-side or ask backend to stamp `cidrClass: 'internet' | 'lan' | 'api-server' | 'other'` field on CIDR workload nodes, add display dropdown "CIDR peers: [ ] internet [ ] private [ ] api-server".

### 1.6 Empty / edge states

- **One CIDR with 10 excepts** (`10.0.0.0/8, except: [10.1.0.0/16, ..., 10.10.0.0/16]`) — backend emits 1 allow rule + 10 except deny rules, plus 11 CIDR nodes. Graph shows 1 amber allow arrow to `10.0.0.0/8` **plus 10 amber except arrows to 10 separate CIDR nodes**. Visually catastrophic — operator sees 11 nodes instead of "one CIDR minus 10 holes." **Fix:** except relationship is semantic parent-child. Render as single visual: parent CIDR node with nested list of excepts (on-node badge "−10 excepts" or compound-child cluster). Highest-impact CIDR presentation bug.
- **`0.0.0.0/0` allow with `169.254.169.254/32` except** (metadata service carve-out — extremely common). Backend emits: 1 amber `→ cidr:0.0.0.0/0` allow, 1 amber `→ cidr:169.254.169.254/32` except-deny. UI shows two separate barrel nodes side by side, both connected with amber arrows. Semantically operator wrote "allow world, block metadata" — UI presents as "two unrelated CIDR peers with equal weight." Relationship invisible. **Fix:** dashed-red outline on except-only CIDR nodes, or group excepts under parent policy in outbound list.
- **Except CIDR equals parent** (`10.0.0.0/8 except [10.0.0.0/8]`) — backend emits allow + deny on same node id. Bundling keys `(src, target, action, coverage)` differ so two edges to same barrel render. Operator sees `→ 10.0.0.0/8` (amber allow) plus `→ 10.0.0.0/8` (amber except) on same node. Redundancy on one node is a visual hint the policy is dead. Downstream issue-detector rule "policy nullified by self-except" would be strong.

### 1.7 Top 3 UX gaps

1. **`except` renders as red standalone deny in every DetailPanel view** (`badges.tsx:150-162`, `rows.tsx:13-44`, `rows.tsx:387`). Operator who wrote "allow world except metadata" sees red deny card on metadata CIDR, concludes another policy is blocking them. This is the "quietly lies" trust bug — graph edge amber-dotted, panel red — same rule, two stories. **Ship-blocker.**
2. **CIDR nodes trigger `WorkloadView` unmodified**, producing "Healthy" hero, empty per-engine section, Pin button that leads into broken reachability flow (`WorkloadView.tsx:38-83`, `:66-73`, `graphStore.ts:341-344`). CIDR click has no useful destination. Either add dedicated `CIDRView` or gate workload sections behind `node.type !== 'cidr'`.
3. **Except carve-outs and parent allow render as unrelated peer arrows** (`elements.ts` + `bundling.ts`). `0.0.0.0/0 except 169.254.169.254/32` — canonical shape — produces two barrel nodes with equally-weighted amber arrows, no visual link. Operators cannot see hole-in-allow structure they authored.

**Open question:** Do you want CIDR nodes to be first-class navigation destination (own view with peer list, classification, coverage summary) or terminal "end of map" endpoint (click-through leads only to reachability grid)? Rest of the fixes fall out cleanly once that fork is decided.

---

## Section 2 — UI Design Auditor findings

### 2.1 CIDR node visual — barrel + dashed blue

- `ui/src/components/PolicyGraph/parts/styles.ts:107-118` — border `#4dabf7` is same hex as egress edge stroke at `edgeStyles.ts:36`. Blue barrel with blue-dashed border sitting at far end of a blue egress edge visually fuses with arrow — node reads as inflated arrowhead, not distinct entity. **Fix:** shift CIDR border to colder, desaturated network hue (`#7aa2c8` or slate-cyan not in edge palette), or lean on barrel silhouette + neutral border, drop the color echo.
- `ui/src/components/PolicyGraph/parts/styles.ts:110-111` — hardcoded `#0f1a24` and `#4dabf7` bypass `colors.css`. Every other palette entry goes through a token. **Fix:** add `--color-cidr-fill` / `--color-cidr-stroke` to `colors.css`.
- `ui/src/components/PolicyGraph/parts/styles.ts:114` — `font-weight: bold` on CIDR label alone. No other node type bolds its label. Bold CIDR reads louder than workload it neighbors, inverting hierarchy (pod is subject, CIDR is peer). **Fix:** drop the bold, hold rank with shape.
- `ui/src/components/PolicyGraph/parts/styles.ts:113` — CIDR label `#4dabf7` (bright info-blue) as text on `#0f1a24` fill. Bright colored text inside shape competes with border color at scan speed; workload nodes use `#e9ecef` neutral text so border carries the semantic. **Fix:** neutralize label to `#cde4ff` or `#e9ecef`.
- `ui/src/components/PolicyGraph/parts/styles.ts:108` — `padding: 14` breaks density rhythm. Workload is `12`, namespace-box is `32`. 14 is off-scale (STYLEGUIDE §1: "4/8/12/16 only"). **Fix:** `padding: 12` to match workloads.
- `ui/src/components/PolicyGraph/parts/styles.ts:105-118` — no `border-width` change on `:selected`, no hover cue anywhere. Barrel clickable but nothing acknowledges it. **Fix:** at minimum add `node:selected` rule with `border-width: 3` and brightened stroke.
- `ui/src/components/PolicyGraph/parts/styles.ts:108` — barrel shape only reads as "pipe / segment" at mid-to-high zoom. At zoom-out barrel collapses to fat rectangle indistinguishable from `deployment`. **Fix:** dashed border survives zoom better than shape; consider horizontal-stripe fill pattern so density = "range" reads at any zoom.

### 2.2 Except edge — amber dotted vs coverage amber

- **[FIXED — stroke now `#fab005` (caution token), label `#ffe08a` (ink-tinted-caution).]** `ui/src/style/edgeStyles.ts:108` — Except stroke `#ffa94d`. DST role token is `#ffb54e`. Two amber tones separated by ~5 hue points read as same swatch on a graph; operator will subconsciously map amber-dotted to "DST-adjacent" (spatial role) rather than "carve-out" (semantic). **Fix:** move Except to `--color-caution` (`#fab005`, yellow-gold) — clearly amber-family but visibly distinct from DST-amber.
- **[FIXED — except now `line-style: dashed` + `line-dash-pattern: [2, 6]` (whisper vs ns dashed `[8, 4]`).]** `ui/src/style/edgeStyles.ts:104-115` vs `styles.ts:33-47` — dashed egress (ns) + dotted-except overlap on same egress path. Dashed-blue (ns) + dotted-amber (except) on same lane produce scanline where eye can't tell where one ends and next begins. **Fix:** widen except dash pattern (`line-dash-pattern: [2, 6]`) so it reads as a whisper vs ns dashed's `[8, 4]`.
- `ui/src/style/edgeStyles.ts:110` — Except reuses `triangle-cross` (matches `.deny` at `:92`). Same shape + amber-not-red = "deny lite" at scan speed. That's not what except means — doesn't block traffic, shrinks the allow set. **Fix:** plain `triangle` and encode carve-out via `source-arrow-shape: 'tee'` (bar at source end) so visual reads "starts constrained" rather than "ends blocked." Alternative: `triangle-tee` at target.
- `ui/src/style/edgeStyles.ts:113` — label color `#ffd8a8` off-token; nothing in `colors.css` uses this hex. Line hard-codes four hex values. **Fix:** add `--color-except` + `--color-except-ink` to `colors.css`.
- `ui/src/style/edgeStyles.ts:114` — width `2.5`. Deny is `3`, base is `1.5`. 2.5 sits in dead space. **Fix:** `width: 2` so except reads clearly quieter than red-dashed deny.

### 2.3 DetailPanel presentation of CIDR / Except

- **[FIXED — CIDR nodes now render `CIDR range` as subtitle; namespace concatenation dropped.]** `ui/src/components/DetailPanel/views/WorkloadView.tsx:63` — CIDR node produces `"kube-system · cidr"` (or `"cidr"` if no ns). Dim subtitle lies-by-omission — operator sees `10.0.0.0/8` label in hero, no acknowledgment this is network range not pod. **Fix:** branch on `node.type === 'cidr'`: render `CIDR range` (or `IP block`) as subtitle, drop namespace concatenation.
- `ui/src/components/DetailPanel/views/WorkloadView.tsx:38-50` — `worstSeverity` folds over `node.statuses`; CIDR has none. Every CIDR click renders "Healthy" green in hero. Silent lie (STYLEGUIDE §5) — panel claims verdict for entity with no verdict semantics. **Fix:** short-circuit for `type === 'cidr'`; replace with neutral "IP block · N.N.N.N/M · used by N policies" identity block.
- `ui/src/components/DetailPanel/views/WorkloadView.tsx:75` — `LabelStrip` renders nothing for CIDR (no labels); `LabelStrip null-return` at `badges.tsx:94` leaves visible empty vertical gap. CIDR node top of panel = false-healthy hero + empty gap = says nothing. **Fix:** in CIDR branch, replace label-strip slot with `except` count + rules-that-include-this-block count.
- `ui/src/components/DetailPanel/shared/badges.tsx` (whole file) — no `CIDR` or `IPBlock` badge/chip exists. When CIDR peer shows in `SelectingPolicyChip`, `PolicyRefList`, or `NeighborList`, falls back to `node?.label ?? role` (`edge-composites.tsx:69, 196`). Raw string like `10.0.0.0/8` in same section-weight font as workload name. Reads as text stub, not first-class. **Fix:** introduce `CidrPeerChip` — dashed-border mini-pill in slate/cyan token, mono font. One primitive, drop wherever peer name renders.
- `ui/src/components/DetailPanel/shared/edge-composites.tsx:61,72` — `EndpointCard` treats CIDR as workload: RolePill · label · subtitle `"— · cidr"` or `"kube-system · cidr"`. No visual echo of graph's dashed-blue barrel. Click CIDR node in graph → open panel → "same" entity now looks like every other workload endpoint. **Biggest missed opportunity on this branch:** graph's careful CIDR silhouette thrown away at panel boundary. **Fix:** `EndpointCard` sniffs `node.type === 'cidr'`, renders with `cardDashed` + graph node's stroke token + mono label.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:167-179` (PortsSummary rows) — same fallback: CIDR peer ports render in standard SidePortBadges layout with workload's `RolePill + EngineBadge + DirectionBadge`. No indication DST is range vs pod. **Fix:** in DST slot when `dst.type === 'cidr'`, render CIDR chip primitive above instead of RolePill+label.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:221-227` — Except carve-out has no dedicated rendering in `BlockerRow`. If except-generated Deny rule reaches this branch, prints under `.deny` label ("Deny rules — delete to allow") next to red-red-red block. Since branch was designed to differentiate except from Istio DENY, this row should stripe or badge amber/except vocab back — otherwise operator's deletion instinct fires on wrong artifact. **Fix:** `RuleGroupList` picks up `coverage === 'except'` per-rule badge with caution-amber chip; header becomes "Blocked by allow-list gap — widen the allow (not delete)."
- `ui/src/components/DetailPanel/shared/status.tsx:10` — `StatusBadges` returning `null` on empty is correct, but combined with WorkloadView forced hero, CIDR shows empty verdict callout — green-tinted bar with word "Healthy." Two vestigial elements stacked. **Fix:** covered by CIDR-branch hero short-circuit above.

### 2.4 Reachability grid — density and anchoring

- `ui/src/components/DetailPanel/DetailPanel.module.css:50-55` — `grid-template-columns: 1fr 1fr 1.3fr` gives verdict column only 30% more room than identity columns. On `70vw` panel at 1440px that's ~340px for verdict — narrower than two SRC/DST columns which carry less-critical evidence. Operator's eye should land on verdict, but grid tells it "verdict is third column, slightly wider." **Fix:** invert weights — `1fr 1fr 2fr` at minimum, or promote verdict column to full-width row above identity columns.
- `ui/src/components/DetailPanel/DetailPanel.module.css:61-66` — `.reachColHeader` uses `border-bottom: 1px solid #444` (raw hex, off-token). Every other section boundary uses eyebrow text + `mb-1`. `#444` hairline is legacy admin-console chrome — STYLEGUIDE §8 kills exactly this pattern. **Fix:** replace with `.eyebrow` + no border.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:414-422` — reach-column header stacks RolePill + label + subtitle. Fine on SRC/DST. On `ResultColumn` (`:482`) header is plain `"verdict detail"` string with same `.reachColHeader` — no visual promotion, same weight as identity headers. One column that carries answer looks like peer of columns that carry setup. **Fix:** verdict column header gets different register — colored `.eyebrow` matching verdict tone, or drop entirely (callout above already names it).
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:99-127` — verdict callout stacks four rows: verdict text, subsystem strip, reason line. Gap 8px. Reason line at `:123` conditionally applies `s.body + textTone` on deny (bold colored) but `s.dim + s.smallText` on allow (small grey). On allow reason line reads as caption, on deny as body. Operator flipping between allow/deny sees different information architecture, not different verdict. **Fix:** hold reason line at one register (`.body`, dim on allow, `textTone` on deny), don't flip type sizes with state.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:100-112` — inside callout the direction arrow `→` between RolePill+label pairs is `s.dim`. Dim arrows against colored role pills create two-tone rhythm where pills feel disconnected. In graph arrow is first-class semantic. **Fix:** arrow inherits verdict tone, not `.dim`.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:161-179` — `PortsSummary` per-engine row packs `EngineBadge + RolePill + DirectionBadge + ports + arrow + RolePill + DirectionBadge + ports` on one flex-wrap line. 8 elements with 4 tone systems competing. On narrow panel wraps unpredictably, SRC→DST relationship blurs. **Fix:** two-line layout per engine — engine on line 1, `SRC ports  →  DST ports` on line 2, mono monospaced so numbers align optically.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:196-206` — `BlockerRow` header stacks EngineBadge + direction-outline + RolePill + label + reason-chip. Five chips varying tint before the actual "edit these policies" call. Reason chip on right, `justify-content-between`, puts *answer* farthest from eye's landing zone. **Fix:** reason chip promoted to left adjacent to engine — reads "istio-explicit-deny" as one atomic thought.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:223` — `style={{ color: 'var(--color-deny)' }}` inline. Class `eyebrowDeny` already exists at `.module.css:141`. **Fix:** use the class.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:451-473` — `MeshCardReach` uses `style={{ borderLeftColor }}` and `style={{ background }}` inline for verdict tone. `.semanticDeny/Allow/Warn` primitives at `.module.css:321-334` already carry this. **Fix:** switch to `semanticDeny/Allow/Warn` shell; drop inline styles.
- `ui/src/components/DetailPanel/DetailPanel.module.css:50-52` — grid gap `12px`. `.reachCol > *` inner cards use own `margin-bottom: 8px`. Column-to-column gap 12, card-to-card gap 8; eye reads columns as tighter than intra-column rhythm. **Fix:** grid gap 16 or column margin 12; keep 4/8/12/16 scale honest across axes.

### 2.5 Color / token hygiene

- **[FIXED — except styles now consume `GRAPH_TOKENS.except` / `.exceptInk` from `ui/src/style/graphTokens.ts` (JS mirror of colors.css since Cytoscape can't resolve CSS vars).]** `ui/src/style/edgeStyles.ts:108,109,112,113` — four hex literals for except styling. Zero tokens.
- **[FIXED — CIDR node now uses `GRAPH_TOKENS.cidr` / `.cidrInk` / `.cidrFill`.]** `ui/src/components/PolicyGraph/parts/styles.ts:110,111,113` — three hex literals for CIDR node.
- **[FIXED — replaced `#444` border with eyebrow-style typography (uppercase, 10px, `#7a848e`); dropped border entirely.]** `ui/src/components/DetailPanel/DetailPanel.module.css:63` — `#444` in `.reachColHeader`. Off-palette.
- **[FIXED — promoted to `.subsystemChip` in DetailPanel.module.css.]** `ui/src/components/DetailPanel/views/ReachabilityView.tsx:40` — inline `style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}` on `SubsystemChip`. STYLEGUIDE §6: never `style={{}}` for anything durable. **Fix:** promote to `.subsystemChip` class.
- **[FIXED — promoted to `.reasonEmph` class.]** `ui/src/components/DetailPanel/views/ReachabilityView.tsx:123` — inline `style={deny ? { fontWeight: 600 } : undefined}`. Class it.
- **[FIXED — already gone in current tree; verified no `marginTop: 8` inline remains in file.]** `ui/src/components/DetailPanel/views/ReachabilityView.tsx:146` — inline `style={{ marginTop: 8 }}`. Parent `d-flex flex-column gap-2` already handles rhythm; use token spacer if needed.
- **[FIXED — swapped to `.eyebrowDeny`.]** `ui/src/components/DetailPanel/views/ReachabilityView.tsx:225` — `style={{ color: 'var(--color-deny)' }}` — reuse `.eyebrowDeny`.
- **[FIXED — added `.egressSection` / `.ingressSection` classes; JSX picks one based on direction.]** `ui/src/components/DetailPanel/views/ReachabilityView.tsx:277` — `style={{ color: tintVar }}`. `direction` is two-value enum → two classes (`.egressSection` / `.ingressSection`). Class it.
- **[FIXED — added `--color-cidr` `#7aa2c8`, `--color-cidr-ink` `#cde4ff`, `--color-except` `#fab005`, `--color-except-ink` `#ffe08a` in `colors.css`; Cytoscape mirror in `graphTokens.ts`.]** **Two new visual dialects (CIDR, except) introduced, zero new tokens in `colors.css`.** Right now CIDR blue collides with `--color-info` (`#4dabf7` — literally same hex) and except amber collides with DST-amber. **Fix:** add `--color-cidr` (slate-cyan), `--color-except` (yellow-gold, aligned to `--color-caution`), `--color-except-ink` for label contrast.

### 2.6 Motion / affordance

- `ui/src/components/PolicyGraph/parts/styles.ts:105-118` — no hover, no `:selected`, no transition on CIDR node. Not CIDR-specific regression (workload/ns share the gap), but CIDR is *newest* node type — moment to establish pattern. **Fix:** `node:selected` block (border-width 3, stroke brightened) and `node.hover` if Cytoscape's built-in hover class is bound.
- `ui/src/style/edgeStyles.ts:104-115` — except edge no hover/selected treatment. If bundling collapses multiple except carve-outs onto one arrow (which it does per `bundling.ts:32-34`), operator has no scan-speed cue that hovering will reveal individual carve-outs.
- `ui/src/components/DetailPanel/views/ReachabilityView.tsx:527-536` — swap button (`⇄`) uses `.iconButton`; inherits STYLEGUIDE motion. Good. But "click to check reachability" empty state at `:541-546` uses `.card .cardFlush .cardDashed` — static dashed card, no hover cue despite being adjacent to active canvas. Reads as inert. **Fix:** soften border further so it doesn't read as broken input.

### 2.7 Cut list

- `WorkloadView.tsx:82` — hero verdict callout on CIDR nodes. Remove for `type === 'cidr'`.
- `edgeStyles.ts:110` — `triangle-cross` on except. Wrong semantic.
- `DetailPanel.module.css:63` — `border-bottom: 1px solid #444` on `.reachColHeader`. Replace with eyebrow.
- `styles.ts:114` — `font-weight: bold` on CIDR label. Silhouette holds rank.
- `styles.ts:113` — bright blue label text on CIDR node. Neutralize.
- `ReachabilityView.tsx:40, 123, 146, 225, 277` — all inline `style={{}}`. Promote to classes.
- `ReachabilityView.tsx:452-472` — inline border/background styling on MeshCardReach. Reuse `.semanticDeny/Allow/Warn`.

### 2.8 Top 3 craft gaps (visual impact, high → low)

1. **CIDR node visual dies at panel boundary.** Graph invests in barrel + dashed border + custom stroke, then `WorkloadView` and `EndpointCard` render the CIDR peer as generic workload rows. Continuity graph↔panel is single biggest craft signal in an operator tool — click the barrel, get the barrel. Fix `WorkloadView.tsx:52-83` + `edge-composites.tsx:34-75` to branch on `type === 'cidr'` and render dashed identity block echoing the graph node.
2. **Two amber tones + shared `triangle-cross` conflate "except" with "DST-adjacent" and "deny."** Amber `#ffa94d` vs DST amber `#ffb54e`, plus `triangle-cross` shared with `.deny` at `edgeStyles.ts:92,110`, collapses three orthogonal semantics into one visual bucket at scan speed. Shift except to caution-yellow token and swap arrowhead to source-tee shape.
3. **Reachability grid buries answer in third column at 1.3× width.** Verdict is reason panel opens, but `.module.css:52` gives it 30% more room than each setup column. Either invert ratio (`1fr 1fr 2fr`) or promote verdict to full-width top band.

**Open question:** Is CIDR node intended to ever carry own policy verdict (someone stamps status key on range because "this range is internet"), or purely peer placeholder inheriting meaning from edges? Changes whether `WorkloadView` should short-circuit hero entirely (peer-only) or render scoped verdict (first-class node with own status semantics).

---

## Conclusion

Both agents converge on the same core diagnosis from opposite angles: **CIDR-as-first-class-peer is well-modeled in the graph layer and then discarded at every downstream surface.** The barrel node + dashed-blue border + amber-dotted except edge are legible visual work; the DetailPanel, ReachabilityView, and filter surfaces treat CIDR as an afterthought.

### Convergent findings (both agents flagged)

1. **`WorkloadView` on a CIDR node is broken end-to-end.** UX agent: renders "Healthy" hero, empty per-engine section, Pin button leading to broken reachability. Design agent: false-healthy verdict is silent lie (STYLEGUIDE §5), label-strip leaves visible gap, panel says nothing. **Both prescribe the same fix:** branch on `type === 'cidr'` in `WorkloadView.tsx:38-83`.
2. **Except carve-out semantics are lost in the DetailPanel.** UX agent: `COVERAGE_META` missing `except` entry, red ✗ deny icon, no coverage badge in `PolicyRow`. Design agent: no visual echo of graph's amber-dotted vocabulary, `BlockerRow` renders except under "delete to allow" header (wrong action). **Both prescribe:** add `except` to `COVERAGE_META`, distinct icon/stripe/wording, carry amber-dotted metaphor into panel.
3. **Continuity graph↔panel breaks at the CIDR boundary.** UX agent: EndpointCard subtitle reads `"— · cidr"`. Design agent: EndpointCard treats CIDR as workload with RolePill+label — biggest missed opportunity. **Both prescribe:** dedicated `CidrPeerChip` primitive with dashed border echoing graph node.
4. **Reachability view doesn't understand CIDR DST.** UX agent: PortsSummary produces confusing "SRC opens 443 → DST accepts none" pair, Istio subsystem chip nonsensical. Design agent: verdict column under-weighted, per-engine row packs 8 competing chips. **Both prescribe:** CIDR DST needs asymmetric single-column layout; verdict column needs promotion.

### Divergent findings (one agent, high-value)

- **UX-only:** except carve-out and parent allow render as unrelated peer arrows in the graph (canonical `0.0.0.0/0 except metadata` shape produces two barrels). Requires new visual grouping concept.
- **UX-only:** users cannot hide CIDR nodes — `toggleNodeType` exposed on store, no FilterPanel surface. Zero classification filter (internet/LAN/api-server) despite backend having helpers.
- **UX-only:** CIDR node is dead-end — no peer list, no back navigation, no "who touches me."
- **Design-only:** CIDR border `#4dabf7` collides literally with `--color-info` and egress edge stroke — CIDR node visually fuses with arrow into "inflated arrowhead." Requires new `--color-cidr` token.
- **Design-only:** except amber `#ffa94d` vs DST amber `#ffb54e` — 5 hue points apart, reads as same swatch, teaches wrong semantic mapping.
- **Design-only:** except reuses `triangle-cross` with `.deny` — encodes "ends blocked" instead of "starts constrained." Wrong arrowhead grammar.
- **Design-only:** 8+ new hex literals introduced this branch, zero new tokens in `colors.css`.

### Recommended ship order

**Ship-blocker (data-truth bugs — will actively mislead operators):**
1. `WorkloadView.tsx` CIDR branch — no more false "Healthy" hero, no broken Pin button.
2. `COVERAGE_META` add `except` + `NeighborGroup` + `PolicyRow` render coverage badge and non-red styling — no more "delete this deny" instinct on carve-outs.
3. `edgeStyles.ts` swap `triangle-cross` for `triangle` + source-tee on except; shift stroke to `--color-caution` (yellow-gold).

**Craft pass (visual continuity + STYLEGUIDE hygiene):**
4. `CidrPeerChip` primitive + apply to `EndpointCard`, `NeighborList`, `PortsSummary` DST slot.
5. `colors.css` new tokens `--color-cidr`, `--color-except`, `--color-except-ink`; delete all hex literals in `styles.ts` + `edgeStyles.ts` for these dialects.
6. `ReachabilityView.tsx` inline `style={{}}` sweep → classes. `MeshCardReach` → `semanticDeny/Allow/Warn` primitives.
7. Reachability grid — invert column weights or promote verdict to top band.

**Follow-on (needs product decision):**
8. CIDR node navigation destiny — first-class view or terminal endpoint? Both agents surfaced this as the fork the rest of the fixes cascade from.
9. Except-parent visual grouping in graph (`0.0.0.0/0 except metadata` shape).
10. FilterPanel node-type dropdown + CIDR classification filter.

### Files touching this audit

Graph: `PolicyGraph/parts/{styles,elements,bundling}.ts`, `style/{edgeStyles,colors}.css`.
Panel: `DetailPanel/views/{WorkloadView,ReachabilityView,EdgeView}.tsx`, `DetailPanel/shared/{badges,status,rows,edge-composites}.tsx`, `DetailPanel/DetailPanel.module.css`.
Store: `store/{graphStore,filters}.ts`.
Backend context (no changes needed): `internal/store/reachability.go`, `internal/policy/k8spolicy/buildRules.go`, `docs/backlog/cidr-peer-modeling.md`.
