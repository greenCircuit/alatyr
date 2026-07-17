# DetailPanel Redesign — Agent Consensus

Findings from 3 agents (backend-sre / frontend-ux / ui-design-auditor) reviewing Node, Edge, Compare panels. Use as checklist. Mark items as done inline.

**Pre-work — locked decisions:**
- [x] **Audience: expert operator** — density, minimal chrome, tint-only tier-1 surfaces, borders only when semantic. Assume reader knows SRC/DST, coverage terms, engine encoding.
- [x] **Edge endpoints: side-by-side** w/ arrow between. Shows directional relationship, mirrors Compare panel headline. Panel min-width must support ≥ 480px.

---

## Priority stack (highest leverage first)

1. [x] **Two-tier surface system** — kill reflexive `border border-secondary rounded p-2`. Tier-1 = subtle bg tint (`#ffffff06`) + no border + 12–16px padding. Tier-2 = borders only when semantic (deny / verdict / issue). Strips ~40% chrome. All 3 agents agree. **Done in V2 preview** (`tokens.module.css` `.tier1` / `.tier2` / `.semanticDeny` etc).
2. [x] **Unified verdict-callout component** reused across all 3 panels. Currently 3 shapes: Workload StatusBadges chip strip, Edge accent-bar banner, Compare headline w/ subsystem chips. **Done** — `.verdictCallout` shell used on Node, Edge, Compare.
3. [x] **Compare per-engine grid restructure** — engine-major rows (row per engine spanning 3 cols) + hide PortsSummary on deny verdicts. **Done** (`.engineGrid`, ports gated on `!isDeny`).
4. [x] **Node per-engine collapse** — summary row per engine w/ counts, default-open only engines w/ deny/issue. **Done** — plus counts now colored capsules (see cross-panel §).
5. [x] **Edge banner promote "blocked by"** — name blocking policy + ManifestButton in banner. **Done** (verdict callout renders `Blocked by: <ns/name> <EngineChip> <YAML>` line).
6. [x] **Silent-lie fixes** — empty status label, listen-port disclaimer, dead-letter port hint. **Partial** — dead-letter wording softened + tooltip explains containerPorts advisory; bidirectional-blocker reverse disclaimer added; StatusBadges empty state + `all ports` disclaimer still TODO in live app.

---

## Node / Workload panel — `views/WorkloadView.tsx`

### Keep
- [x] Identity header + "Pin as reachability source" promoted to title area (`WorkloadView.tsx:28-44`). **Done in V2**.
- [x] Effective-status hero above per-engine evidence — verdict/proof split. **Done**.
- [ ] Cross-ns policy warning badge (`rows.tsx:392, 401-407`). Not yet in V2 — needs live-store wire.

### Change
- [x] **Per-engine cards too heavy** (`WorkloadView.tsx:100-115`). Collapse each engine to summary row w/ in/out counts + status badges. Default-open only engines w/ deny/issue. Drop outer `border border-secondary` per engine card. **Done + upgraded** — counts render as colored capsules (in=blue, out=amber) instead of `in 12 · out 3` text.
- [x] **Two issue channels stacked** (`WorkloadView.tsx:62-79`). `nodeInfo.issues` inline list + `NodeIssues` cards. Merge into single Issues section OR make one subordinate. **Done** — single `.semanticWarn` block, count in title.
- [x] ~~**Labels above verdict wastes prime real estate** (`WorkloadView.tsx:46-54`). Move below Effective status OR fold into `<details>`.~~ **REVERSED.** Kept labels inline in identity tier-1 (line-3 of Node panel, line-2 of Edge/Compare EndpointCard). **Reason:** operator + backend-sre pushback — labels = selector currency, needed at glance for policy-match diff + `kubectl -l` workflow. Folding buried the join key. Compromise = filter system labels (`app.kubernetes.io/*`, `helm.sh/*`, `meta.helm.sh/*`, `kubernetes.io/*`, `k8s.io/*`, `argocd.argoproj.io/*`) behind `Show N system labels` disclosure; keep `app`, `tier`, `version` etc inline. Chip = click-to-copy `key=value`; strip has `copy selector` button for full `k1=v1,k2=v2`.
- [x] **Effective-status hero not visually a hero** (`WorkloadView.tsx:58-61`). Same 11px badges as everything else. Bump to 13–14px, heavier weight, `calloutAccent` box tinted by worst-severity color. **Done** — `.verdictCallout` tinted by worst severity + eyebrow prose `Effective status` dropped per SRE consult (tint + pill weight carry the meaning without a label).
- [ ] **`⚠` emoji glyph inside uppercase heading** (`WorkloadView.tsx:68`). Extract to SVG/icon component, gap-1, drop emoji. Not addressed in V2 preview — WorkloadView-live scope.

---

## Edge panel — `views/EdgeView.tsx`

### Keep
- [x] `EdgeReachabilityBanner` above endpoints (`EdgeView.tsx:25`) — cross-engine truth vs arrow's single-engine opinion. **Done** as `.verdictCallout` at top of Edge panel V2.
- [x] `PairIssues` between endpoints and policy list (`EdgeView.tsx:32`). **Done** — `.semanticWarn` block titled `Findings` between endpoints and policies.
- [ ] Wildcard-aware `EndpointCard` (`edge-composites.tsx:51-63`) — honest "any peer". Not exercised in V2 mock (no wildcard fixture).

### Change
- [x] **PolicyRow field-row noise** (`rows.tsx:28-53`). Engine/Namespace/Direction repeated as 3 label-value rows per policy card. Collapse to badge meta-strip in header, inline ns as `ns/name`, keep fieldRow only for Ports. **Done** — header renders name · ns + meta strip w/ engine + direction arrow + YAML link.
- [x] **Banner needs dominance** (`edge-composites.tsx:117-144`). Swap to top w/ tinted bg (not just left-stripe), verdict badge fs-13 bold. Promote blocking policy names as "blocked by: <policy>" line + ManifestButton — currently requires scrolling PolicyRow list and inferring from `action === 1`. **Done** — verdict text at 18px + `Blocked by` line + `<YAML>`.
- [x] **Banner mixes inline-label pattern (`Engines: <chips>`) w/ panel's eyebrow convention** (`edge-composites.tsx:128-144`). Convert to eyebrow labels. **Done** — meta strip pattern only.
- [x] **Pair-issues section title-less** (`EdgeView.tsx:31-33`). Give section header consistent w/ WorkloadView's "Detected issues". **Done** — `Findings` title (matches N-findings shape used on Node panel).
- [x] **SRC/DST layout** — pick stacked vs side-by-side per pre-work decision. **Done** — side-by-side via `.endpointRow` grid + arrow.

### Silent-lie gap
- [x] `PolicyRow` shows allow on port 8080; workload listens on 9090 → renders identical to working allow. No dead-letter indicator. Add "listens on: <ports>" hint or dim when no intersection. **Done, softer wording per SRE consult.** V2 renders per-endpoint `listens: 9090/TCP` on EndpointCard AND policy card gets `⚠ no declared listener on this port` (was `⚠ dead-letter`). Tooltip explains containerPorts are declarative — workload may still bind other ports at runtime. Reason for softening: containerPorts advisory-only; naming it "dead-letter" pages people about working rules.

---

## Compare / Reachability panel — `views/ReachabilityView.tsx`

### Keep
- [x] Tiered hierarchy: verdict → blockers → per-engine collapsed (`ReachabilityView.tsx:570-583`). **Done**.
- [x] Blocker rows w/ endpoint pill + culprit + delete-target deny rules (`ReachabilityView.tsx:194-236`) — pager answer, top of panel. **Done + wording softened** — `Delete to allow` renamed `Matching subrule` w/ clarifier "Removing this subrule unblocks the pair. Does not delete the surrounding policy". Reason: tired operator was one `kubectl edit` away from nuking an entire AuthorizationPolicy.
- [x] Bidirectional chip w/ explicit reverse-unavailable failure (`ReachabilityView.tsx:57-91`). **Done + reverse-blocker disclaimer added** — bidirectional label now colored per reverse verdict + inline note "Blockers below are for SRC → DST. Reverse direction is also blocked, possibly for different reasons — pin DST as source to inspect." Reason: original "blocked both ways" label silently implied same reason both directions.
- [ ] AND-semantics header "Blocked on N axes · all must clear" (`ReachabilityView.tsx:249`). Present via `cmp.reason` string in mock, not yet a semantic header component.

### Change
- [x] **PortsSummary wrong for deny** (`ReachabilityView.tsx:165, 574`). Hide on deny (theoretical, hides blockers below fold). On allow, promote next to verdict headline as "reachable on: <ports>". **Done** — `!isDeny` gate + inline `reachable on:` line inside verdict callout.
- [x] **Per-engine grid too heavy in single `<details>`** (`ReachabilityView.tsx:575-582`). Split disclosure per engine, default-open engines named in blockers. OR restructure to engine-major grid: row per engine spanning 3 cols (src selecting | dst selecting | verdict detail) so eye sweeps left-right per engine. **Done — engine-major grid chosen** (`.engineGrid` w/ header row + one row per engine, 3 cols).
- [x] **VerdictHeadline chaotic flex-wrap** (`ReachabilityView.tsx:110-118`). Two-line composition: line 1 = verdict badge + `SRC → DST` sentence at fs-5 bold; line 2 = subsystem chips + bidirectional. Names as text w/ subtle SRC/DST role dots, not full `bg-info/bg-warning` competing w/ verdict badge. **Done** — 2-line composition + role dots (`.roleDotSrc` / `.roleDotDst`).
- [x] ~~**SRC/DST columns duplicate the pinned banner** (`ReachabilityView.tsx:407-423`). Role badge / label / ns / type / k8s labels repeated. Cut labels from columns OR make banner collapsible when Tier 2 opens.~~ **REVERSED.** V2 keeps SRC/DST columns w/ labels + `EndpointCard` shape. **Reason:** operator + backend-sre — labels above fold win over dedup. Header is a compact one-line sentence w/ role dots; the endpoint cards below carry labels + listens + ns/type. Redundancy with the sentence is minor and buys the scan-value.

### Silent-lie gap
- [ ] `SidePortBadges` "all ports" for `side.allPorts` (`ReachabilityView.tsx:145-151`). Means *policy* unrestricted, not workload listens all ports. Effective reachability = policy ∩ listening. Add listen-port hint or explicit disclaimer. Not addressed in V2 preview — live-app scope.

---

## Cross-panel visual system

- [x] **Verdict rendering diverges 3 shapes** (Workload StatusBadges / Edge banner / Compare headline). Unify — pick accent-bar callout as canonical shell. **Done** — `.verdictCallout` on all 3.
- [x] **Border-first design.** Nested cards stack 3 concentric `border-secondary` inside 24px. Replace w/ tier-1 bg tint + tier-2 semantic border only. **Done** — `.tier1` / `.tier2` / `.semanticX`.
- [x] **Color role overload.** `bg-info` = SRC + engine badge + port badge. `bg-warning` = DST + ns-level + allow-all. Introduce role tokens `--role-src` / `--role-dst` distinct from semantic severity palette. **Done** — role tokens + per-engine hue tokens (`--engine-k8s` / `--engine-istio` / `--engine-mesh`) so engine chip is no longer neutral grey. Reversed the original "neutral engine chip" plan per SRE consult: operators anchor to hue faster than to text; same tokens can drive edge stroke color on the graph so panel + canvas share vocabulary.
- [ ] **Manifest button placement inconsistent** (`PolicyRow` header-right vs `BlockerRow`/`MeshCardReach` inline "Forced by:"). Pick header-right always. Partial — V2 keeps YAML at header-right on PolicyRow, inline on blockers (matches "Forced by:" pattern for narrative flow). Revisit if inconsistency bites.
- [x] **Issue callouts diverge** — WorkloadView renders `nodeInfo.issues` inline list + `NodeIssues` cards; EdgeView renders only `PairIssues`. Same finding renders two ways per panel. **Done** — single `.semanticWarn` shell on Node + Edge.
- [x] **SRC/DST casing diverges** — Compare upper, Edge (`edge-composites.tsx:44`) lower. **Done** — uppercase everywhere in V2.
- [ ] **Coverage/posture badges missing on `SelectingPolicyChip`** (`ReachabilityView.tsx:377-396`). Unenforced policy looks identical to actively denying. Reuse `CoverageBadge`. Not addressed in V2.
- [x] **Padding scale flat** — `p-2` (8px) for everything from top-level callouts to inner rule sub-rows. No density signal. **Done** — `--pad-tier1` 14px, `--pad-tier2` 10px.
- [x] **Type scale flat** — `fs-5` once (workload name), rest default/small/11px. Add 3-step scale: 13 body / 15 section / 18 hero. **Done** — `--fs-body` / `--fs-section` / `--fs-hero`.

### Silent-lie gap (system-wide)
- [ ] `StatusBadges` renders `—` for empty (`status.tsx:9`). Means "no L3 engine matched this workload" (default-allow signal) OR "not evaluated" (bug). Same glyph, opposite meaning. **Label empty state explicitly.** Live-app scope — not exercised in V2 mock.

---

## Mock page for iteration

- [x] Add `#design-preview-panels` hash route (parallel to existing `#design-preview` in `App.tsx:19`) rendering Node + Edge + Compare side-by-side w/ mock data. Iterate visual system w/o touching live graph state. **Done**.

---

## Second-round tri-agent audit (backend-sre + frontend-ux + ui-design-auditor)

Fresh audit after PostureRow / unenforced / MeshBlock landed. Three agents re-reviewed the V2 preview in parallel. Findings ranked P0 (ship-blocker) / P1 (vocabulary + interaction) / P2 (craft debt) / P3 (SRE data guards).

**Triple-consensus (all three flagged the same wound from a different angle):**
- Node verdict callout has no headline: sre says the *computation* is wrong (silent lie — worst severity ignores `perEngine.hasDeny`), ux says the *display* is missing (Edge/Compare have hero verdict text, Node doesn't), design says the *hierarchy* inverts (hero name above verdict callout, but verdict is the answer).
- Vocabulary drift across panels: sre wants `Blocking` array w/ `PolicyRef`s; ux wants `PolicyRef` reused in Edge banner `Blocked by` (currently bespoke) + `ActionIcon` in meta-strip `allow`/`deny` text; design wants one icon family across all glyphs. Same convergence.

---

### P0 — ship-blockers

- [x] **Verdict derivation bug** (`panels.tsx:443-449`). `worst` reduced over `node.statuses` only. `perEngine[].hasDeny=true` (istio in mock) does not influence the callout. Callout renders warn/allow while istio deny is right below. Silent lie. Fix: compute effective severity by folding `perEngine.hasDeny` + `perEngine.selectingPolicies.some(state === 'blocks')` into the reducer. **Done** — added `denyEngines` fold + `anyEngineBlocks` gate; any engine deny lifts `effectiveWorst` to `critical` regardless of status-chip max.
- [x] **`engine.denyPolicy` singular** (`mockData.ts:160`, `panels.tsx:536-543`). Real clusters stack multiple denies (root-default-deny + ns-scoped deny + tenant deny). One-culprit shape names one while two others also block. Array-ify to `denyPolicies` + render all. **Done** — mock istio engine now has `denyPolicies: [root-default-deny, block-legacy-api]`; Blocking block renders as `Blocking (N):` + stacked `PolicyRef` list.
- [x] **Catch-all peer variant missing** (`panels.tsx:314`). `podSelector: {}` + `namespaceSelector: {}` = "everyone everywhere". Fixture doesn't exercise. Real k8s NetworkPolicy w/ empty `from:` falls through the `peers.length > 0` gate and renders nothing. The most permissive rule in the file renders as the quietest. Add explicit "any peer" row that reuses `SelectorBlock`'s catch-all copy. **Done** — added `wildcard` flag on `PolicyPeer` + `kind: 'wildcard'`. Mock exercises via `k8s/allow-health-checks` policy w/ `from: []` peer. `PeerRow` renders wildcard as explicit warn-colored "any peer everywhere" row + italic explainer of k8s semantics, so the most permissive rule is now the most visible instead of the emptiest.
- [x] **`deadLetter` off-by-role bug** (`panels.tsx:691-694`). Egress branch compares against `edge.src.listensOn` (source's own listens — irrelevant to what it can reach). Port intersection is always vs DST. Simplify to `edge.dst.listensOn` regardless of policy.direction. **Done** — dropped the direction branch; always intersect vs `edge.dst.listensOn`.
- [x] **Node verdict callout has no hero verdict text** (`panels.tsx:484-488`). Edge + Compare open with `verdictText` ("✗ blocked" / "✓ reachable"). Node just renders StatusPills. Cross-panel consistency regression: operator scanning Node → Edge → Compare re-learns the callout shape each panel. Add computed one-line summary (e.g. "✗ Blocked by istio · 2 findings" / "⚠ Exposed to internet · 2 findings" / "✓ Healthy") above the pill row. **Done** — `heroText` composed off `denyEngines` + `worstStatus` + `findingCount`; renders as `verdictText` line above the pill row. Cases: any-engine-blocks = `✗ Blocked by {engines} · N findings`; high/critical status = `⚠ At risk · N findings`; warning-or-findings = `⚠ N findings`; else `✓ Healthy`.

### P1 — vocabulary unification + interaction polish

- [x] **Reuse `PolicyRef` in Edge banner `Blocked by` line** (`panels.tsx:644-650`). Currently bespoke composition; Node + Compare use `PolicyRef`. One component everywhere = one vocabulary. **Done** — Edge `Blocked by:` swapped from name+mono+EngineChip+ManifestLink composition to single `<PolicyRef ... action="deny" />`.
- [x] **`ActionIcon` in meta-strip** — engine allow/deny text (`panels.tsx:637, 766`) currently plain colored word. Every other allow/deny site uses ✓/✗ square. Reuse `ActionIcon`. **Done** — Edge + Compare metaStrip render `<EngineChip> + <ActionIcon>`; colored-word `allow`/`deny` text gone.
- [x] **`FindingKind` chip on findings** (`panels.tsx:507`). Free-form English text ("Cross-namespace ALLOW rule references deleted workload…"). Add a kind chip (`cross-ns-dangling` / `port-conflict` / etc) + short structured predicate so findings mirror StatusPill scan-value. **Done** — `FindingKindChip` component + `.findingKind` neutral-surface mono chip. Mock issues carry `kind: 'cross-ns-dangling' | 'port-conflict' | 'port-not-declared'`; rendered inline between severity dot and text on Node + Edge findings.
- [x] **Per-engine disclosure `open` fallback** (`panels.tsx:524`). `open={engine.hasDeny}` — on healthy workload every engine closes, panel reads empty below verdict. Fallback: open top-by-peer-count when no engine has deny. **Done** — `topByPeersEngine` computed when `!anyEngineBlocks`; per-engine `<details>` `open` = `engine.hasDeny || engine === topByPeersEngine`.
- [x] **Compare per-engine grid `open={false}`** (`panels.tsx:847`). Default-open on deny so the "which side to fix" map is visible without a click. **Done** — `open={isDeny}`.
- [x] **Click-to-copy affordance** — `title` on the label strip educates once ("click any chip to copy `k=v`") instead of tagging every chip with a copy glyph. **Done** — `LabelStrip` container carries `title="Click any chip to copy 'key=value' · click 'copy selector' to copy the full 'k1=v1,k2=v2' selector for 'kubectl -l'"`. First-time hover teaches; no per-chip chrome.
- [x] **PostureRow + PolicyCard deny-tint stacking** blurs the posture-vs-restricted boundary (`panels.tsx:565-581`). Add "Rules matching specific peers" eyebrow between OR dim restricted tint when posture already carries direction verdict. **Done** — eyebrow between when both posture and restricted exist: `Rules matching specific peers (N)`.
- [x] **`MeshBlock` mtls chip needs tooltip on STRICT** (`panels.tsx:392`). DISABLE reads deny-red — operator wants to know why. Add `title` explaining STRICT/PERMISSIVE/DISABLE at the workload level. **Done** — mtls mode span carries `title` per mode: STRICT explains zero-trust posture, PERMISSIVE calls out migration mode risk, DISABLE explains why AuthorizationPolicy matching principals fails, UNSET points to parent PeerAuthentication resolution.
- [x] **Compare bidirectional label buried in metaStrip** (`panels.tsx:773-779`). Reverse direction is a distinct concept, currently visually equal to `k8s deny · istio deny`. Promote to own line above endpoints OR gate behind `Reverse:` eyebrow. **Done** — bidirectional lifted out of metaStrip to its own row w/ `Reverse` eyebrow + colored label + inline disclaimer text on split.

### P2 — craft debt (real but non-blocking)

- [ ] **Icon family mishmash** (loudest craft tell). Unicode glyphs from 4 sources: `▸/▾` triangles, `↑/↓ →/←` arrows, `✓/✗` ballots, `●` bullet, `⚠` warning, `×` close. Four stroke weights, four optical sizes. Fix: adopt Lucide (or Radix Icons) at 12/14/16px, replace every glyph. If icon lib off-limits, pick ONE Unicode weight and stick to it. **Deferred** — Lucide/Radix is a new dep decision. `×` close was the only ship-quality outlier; replaced with an SVG-stroke close glyph (`IconClose`). Remaining Unicode glyphs (arrows, ballots, chevrons) stay for now; full icon-lib pass is a follow-up ticket.
- [x] **Border creep returned.** Six left-bars stack in Node panel (verdictCallout 4px + semantic 3px + policyCard bars + postureRow bars + peerArrow color + dashed dividers). Reserve the 3px semantic bar for `.verdictCallout` and Compare blockers only. Drop from `.policyCard*` + `.postureRow*` — keep tint, drop bar. **Done** — dropped left-bars from `.policyCardBlocks/Allows/Inert/Unenforced` + `.postureDenyAll/AllowAll/Unenforced` (tint alone carries state); dropped `.peerRow` dashed border (gap now separates) and `.engineGridDivider`. Left-bar reserved for `.verdictCallout` (4px) and `.semanticDeny/Allow/Warn` (3px on findings + Compare blockers).
- [x] **Spacing scale off-rhythm.** Gap values 6/8/12/14 mixed inside one panel. Commit to 4/8/12/16, delete 6 and 14. `--pad-tier1` 14 → 12. **Done** — `--gap-xs 4 / --gap-sm 8 / --gap-md 12 / --gap-lg 16` tokens introduced. All ad-hoc gaps rewired. `--pad-tier1` 14→12, `--pad-tier2` 10→8.
- [x] **Type hierarchy inverts on verdict callout** (`panels.tsx` hero name at 18/700 sits above verdict band). Verdict is the answer, name is the label. Either flip order (verdict first) or demote name to `--fs-section` (15px). **Done** — Node identity name demoted from `.hero` (18/700) to `.section` (15/600). Verdict callout heroText (18px) is now the loudest element on the panel; name reads as label.
- [x] **Three panels don't read as siblings.** Widths 460 / 520 / 780. Lock to shared unit (say 480px), Compare = 2× (960). Currently look like three windows from different apps. **Done** — Node 460→480, Edge 520→480, Compare 780→960. Base unit 480 shared; Compare is exact 2×.
- [ ] **Inline `style={{}}` 30+ occurrences** (`panels.tsx`). Promote recurring values to `tokens.module.css` classes; delete one-offs. **Partial** — several one-off `paddingLeft: 14` / `fontStyle: italic` / `marginTop: 2` etc still inline. Recurring `policyMetaRow` / `labelStrip` / `frameHeaderMeta` promoted. Full inline-purge deferred.
- [x] **Motion absent on `<details>`, chevron, hover.** Add 120ms ease-out on `max-height`, `opacity`, chevron rotate, ghost-button hover. **Done** — introduced `--motion: 120ms cubic-bezier(...)` token. Transitions on `.verdictCallout` (background), `.disclosureSummary::before` (chevron rotate), `.ghostButton` / `.iconButton` / `.primaryButton` / `.labelChip` / `.labelCopyAll` / `.manifestLink` / `.policyCard`. `<details>` content itself doesn't transition (browser limitation) but the chevron carries the state-change signal.
- [x] **Radii chaos**: statusPill 4, engineChip 3, portChip 3, countCapsule 10, ghostButton 4, labelChip 3, verdictCallout 8, policyCard 6. Consolidate to: chip 3, card 6, panel 8. Countcapsule 10px = over-rounded pill on square card (designer's tell). **Done** — `--r-chip: 3 / --r-card: 6 / --r-panel: 8` tokens. Every chip, card, and panel rewired. Countcapsule dropped 10→3.
- [x] **`copy selector` chip is dashed.** 2015 admin-console reflex. Solid ghost treatment OR icon-only (clipboard). **Done** — `.labelCopyAll` border switched from dashed to solid 1px. Hover fills bg slightly for affordance.
- [x] **Cut list**: `.policyCard*` left-bars, `.postureRow*` left-bars, `.peerRow` dashed border, `.engineGridDivider` dashed line, `.postureCoverage` triple-emphasis (uppercase + letter-spacing + weight 700 — pick one), emoji `×` close. **Done** — left-bars stripped (see border creep entry); dashed borders gone; `.postureCoverage` dropped letter-spacing (weight + uppercase kept = two dimensions, not three); emoji `×` replaced with `IconClose` SVG stroke.

### P3 — SRE data guards (variance at edges)

- [x] **Empty vs unavailable distinction.** `Compare.engines[].srcSelecting = []` renders "none" — doesn't say whether it's no-policies or engine-unavailable. Same silent-lie class as `StatusBadges: —`. Model as `{ policies: SelectingPolicy[] } | { unavailable: true, reason: string }`. **Done** — added `srcUnavailable` / `dstUnavailable` optional `{ reason: string }` on Compare engines. Mock istio engine simulates RBAC-denied src side. Grid renders `.engineUnavailable` caution card (yellow-tinted) w/ reason text instead of falling through to "none".
- [x] **Mesh-not-enrolled state.** V2 hides `MeshBlock` when `mesh` absent. Can't tell "not in mesh" from "mesh data not fetched". In mesh-tracked ns, render explicit "provider: istio · not enrolled" empty state. **Done** — added `enrolled: boolean` on `MeshInfo`. When `enrolled: false`, `MeshBlock` renders an explicit "not enrolled" state w/ warn-tinted badge + prose explaining the consequence (no SA identity, no mTLS, no AuthorizationPolicy enforcement) + remediation hint (`istio-injection=enabled` label).
- [x] **`principals` / `notPrincipals` on MeshBlock + peers.** Istio Auth matches on SPIFFE principal. Currently only surfaces as raw string on peer label (`mockData.ts:187`). Structure as `principal` field on `PolicyPeer`, render as mono chip. **Done** — added `principals` + `notPrincipals` `string[]` on `MeshInfo`. Renders as `.principalChip` (info-tinted mono) + `.principalChipDeny` (deny-tinted, `✗` prefix) under new `Ingress principals` section on MeshBlock. Answers the "who is the mesh expecting" half of Istio Auth.
- [x] **Findings culprit refs may not resolve.** `MOCK_NODE.issues` culprits point to policies not in `selectingPolicies`. Real feed will reference deleted / foreign-ns policies. Add resolved/unresolved styling: dim + `?` icon for unresolved so operator doesn't chase a broken YAML link. **Done** — Node panel computes `resolvedPolicyKeys` set from all `perEngine.selectingPolicies`; each `PolicyRef` on findings takes `unresolved={!resolvedPolicyKeys.has(key)}`. `.policyRefUnresolved` renders dim + strike-through-wavy underline + `?` glyph + tooltip explaining why the YAML link may 404. Mock `allow-legacy-cart-egress` culprit exercises the case.
- [x] **"L4 only" indicator on Istio cards missing L7** (`panels.tsx:319`). Absence of L7Row = ambiguous ("policy is L4-only" vs "L7 got dropped in the UI"). Add explicit `no L7 match — L4 only` line on Istio cards without L7. **Done** — `.l4Only` italic dim line renders on Istio `PolicyCard` when `policy.l7` absent: `L4 only · no host / method / path constraint`. k8s / other L3-only sources render nothing (their absence isn't ambiguous).
- [x] **`lastEvaluated` staleness stamp.** Graph built off cached informer state; lagged reflector = panel confidently lies. Not preview-scope to build the fetch, but reserve room in frame header now. **Done** — `StalenessStamp` component in frame header. Colored dot (green <2min, caution 2-15min, deny >15min) + relative time. All 3 panel headers wired w/ mock ages (42s, 310s, 1200s = one per bucket). Tooltip explains reflector-lag failure mode.
- [ ] **`PolicyRefList` unattributed policies missing** (still open from prior round). Live surfaces raw selecting policies that produced zero rules (`rows.tsx:587`). V2 only has `selectingPolicies`. Verify feed shape at merge. **Deferred to wire-up** — feed shape decision, not preview logic.
- [ ] **`NodeIssues` feed** (still open from prior round). Confirm V2 `issues` is wired to same store cluster-findings feed as live `WorkloadView.tsx:79`, else lose "another workload just locked yours out" signal. **Deferred to wire-up** — same feed-verify class.

### Meta

`ui-design-auditor` asked: does V2 ship over Bootstrap-utility layout, or its own layout primitive? Answer determines whether the "3 panels don't read as siblings" fix (P2) is a token change or a rewrite. Deferred — pick before P2 pass starts.

**Overall verdict:** V2 currently *regresses* correctness at the verdict layer (silent-lie bug) while adding surface polish. Ship w/o P0 = pretty tool that lies. P0 + P1 = real upgrade. P2 + P3 = zenith of craft + data honesty.

---

## Parity gaps flagged by backend-sre (ship-blockers)

SRE gap analysis of V2 Node panel vs live `WorkloadView.tsx` + `NeighborList` / `NeighborGroup` / `PostureRow`. V2 currently loses signal live carries; visual upgrade without these = functional regression.

- [x] **PostureRow missing (ship-blocker).** Live `PostureRow` (`rows.tsx:512`) renders blanket-posture rules (`deny all` / `allow all` / `unenforced`) as one-line whole-direction verdicts *above* per-peer cards. V2 buries root-default-deny as an "inert" `PolicyCard` in the list. On locked-down namespace this is *the* answer to "why can nothing talk to this pod" — V2 forces reading 3 cards to arrive at what live shows on one line. Add a `PostureRow` shape at top of each per-engine section for policies whose peer set is empty + coverage is blanket. **Done** — `PostureRow` component + `isPosture` split. Per-engine body now renders posture rules (deny all / allow all / unenforced) as one-line verdicts w/ colored coverage chip + direction arrow + `PolicyRef` + cross-ns badge, ABOVE the per-peer `PolicyCard` list. Uses `.postureDenyAll` / `.postureAllowAll` / `.postureUnenforced` shells (semantic border + tinted bg).
- [x] **`unenforced` coverage state dropped (ship-blocker).** Live tracks three coverage values via `CoverageBadge` (`rows.tsx:415-416`) and `PostureRow`. V2 `PolicyState` (`mockData.ts:25`) = `allows | blocks | inert` only. `unenforced` ≠ `inert`: unenforced means "policy selects this workload but has *no rules* on this direction, leaving it wide open" — the classic footgun where someone writes a NetworkPolicy without an ingress block and now nothing else applies. Backend already emits it. Add `unenforced` to `PolicyState` (or split into a `Coverage` field) + render distinctly on `PolicyCard` + `PostureRow`. **Done** — `PolicyState` extended to `allows | blocks | inert | unenforced` + new `PolicyCoverage` field (`restricted | deny all | allow all | unenforced`) so posture classification travels separately from per-rule state. `PolicyCard` gets `.policyCardUnenforced` warn-tinted shell + state chip color-coded warn w/ tooltip explaining the footgun. Also fixed `inert` tooltip so operator can tell "rule exists but doesn't fire" from "no rule at all".
- [x] **No mesh block (Istio ship-blocker).** Live `WorkloadView.tsx:123` renders `MeshCard` per source: ServiceAccount, mTLS mode, revision. V2 has zero mesh surface. On any Istio-heavy target this is the first thing anyone checks when a pod isn't reaching the mesh — SA identity + mTLS mode drive every AuthorizationPolicy match. Add a mesh section on Node panel below verdict callout, above per-engine. **Done** — `MeshBlock` component renders below verdict callout: provider name eyebrow + `no sidecar` warn badge (when applicable) + SA mono line + mTLS mode chip color-coded (STRICT=allow, PERMISSIVE=caution, DISABLE=deny) + revision. Placed above per-engine section so the mesh answer is visible on scroll open.
- [ ] **NodeIssues feed unclear.** V2 mock hardcodes `MOCK_NODE.issues`. Live pulls cluster-level findings via store (conflicts, lockouts referencing this node — `WorkloadView.tsx:79` renders `NodeIssues`). At merge, verify V2's `issues` array is wired to the same feed. If not, "another workload's policy just locked yours out" signal is silently dropped.
- [ ] **`PolicyRefList` unattributed policies missing.** Live surfaces raw selecting policies that produced *zero* rules on this direction (`rows.tsx:587`). V2 collapses everything under `selectingPolicies` — fine if the feed is the same shape, but if V2 only ingests rules-that-fired, drops policies that select the workload with zero effect. Same silent-lie class as the coverage gap.

---

## Follow-ups from V2 review (open)

Raised by operator after reviewing V2 preview. Not yet implemented.

- [x] **Policy badges = icon + name, not just name.** Every place a policy is named (Edge `PolicyRow` header, Compare blocker culprit line, per-engine `Blocking:` row, findings — see next item), prepend a small action icon (✓ allow / ✗ deny) or a policy-type glyph so scan-value = 1 glance. Right now the name is the only signal; on a busy Edge panel operator has to read the meta strip to tell allow-policies from deny-policies. Icon color = allow/deny token, not engine token, so severity stays legible independent of hue. **Done** — `ActionIcon` component (colored ✓/✗ square), wired on Edge PolicyRow header, Node per-engine Blocking, Node/Edge Findings, Compare blockers, Compare per-engine src/dst-selecting rows. Also introduced `PolicyRef` component = ActionIcon + EngineChip + mono `ns/name` + YAML for one-line policy reference reused across all sites.
- [x] **Findings must name culprit policy + link its YAML.** `.semanticWarn` findings currently render as plain text bullets (`{issue.text}`). Add per-finding: (a) culprit `PolicyRef` (engine + `ns/name` mono), (b) `<YAML>` manifest button. Applies to Node `Findings`, Edge `Findings`, Compare blocker rows. Operator answering "which policy broke this?" should see the answer in the finding, not have to reconstruct it from the policy list below. **Done** — each finding renders severity dot + text on top line, `PolicyRef` (icon + engine + `ns/name` + YAML) indented below. Applied to Node + Edge findings; Compare blockers already reference culprit and now use `PolicyRef` for consistency.
- [x] **Per-engine evidence must render selecting policies (currently blank).** Node panel per-engine `<details>` body says `Peer lists elided in preview — real component renders NeighborList here` — but even in the live app there is no way to see *which policies selected this workload per engine*. Every per-engine section should list the selecting Allow + Deny policies for that engine w/ direction (ingress/egress), state (blocks / allows / inert), and `<YAML>`. Same shape as Compare `srcSelecting` / `dstSelecting` cells so the vocabulary matches across panels. Without this, per-engine `Statuses` badges are a verdict w/o evidence — the "silent lie" pattern applied to per-engine intersections. **Done + enriched** — first pass rendered one row per policy (icon + engine + name + direction + state). Operator flagged that render was thinner than what the backend `NodePolicies` struct already exposes: no ports, no label selectors, no peer list, no cross-ns callout, no L7. Upgraded to `PolicyCard` component that mirrors the live `NeighborGroup` shape in the tier1/tier2 visual system:
    - Header: `ActionIcon` + policy name + `EngineChip` + `ns` mono + `cross-ns` badge (when policy lives outside workload ns) + state chip + `<YAML>` on right. Whole card tinted by state (`policyCardBlocks` / `policyCardAllows` / `policyCardInert`) so verdict reads at a glance mid-scroll.
    - Meta row: direction arrow (colored per role — egress cyan, ingress amber) + `ports:` + `PortList` (blue mono chips per port, or `any port` italic when unrestricted).
    - `SelectorBlock` labelled *"this workload matched by"* — same `LabelChip` shape as the workload's identity strip so operator diffs by eye. Catch-all case (`podSelector: {}`) renders "any workload in `{ns}`" instead of an empty chip row.
    - Peer list — collapsible above 4 peers. Each peer row = direction arrow (colored) + label + `/ ns` mono + `kind` chip (`workload` / `namespace` / `external`) + per-peer `SelectorBlock` labelled *"matched by"* underneath so operator sees which labels the policy matched on the peer end. Empty peer list is silent (the header verdict + node-selector already carry meaning).
    - Istio-only `L7Row` — hosts / methods / paths chips (mono, dim tint) so the L7 gate is visible without opening YAML.

    Empty engines still say "No {name} policies select this workload — default posture applies" to avoid the silent-lie empty case.

---

## Additions beyond original doc (session log)

Session on 2026-07-16 layered these on top of the checklist above. Not in the original agent consensus — added after operator + backend-sre feedback on the V2 preview:

- **Click-to-copy label chips.** Each `LabelChip` copies `key=value` to clipboard on click. Trailing `copy selector` chip on the strip copies the full comma-joined `k1=v1,k2=v2` for pasting into `kubectl -l`. **Why:** operator flagged that debugging "why does the policy match?" ends in a `kubectl -l` command; every second of retyping is friction. Backend-sre backed as highest-leverage add.
- **Colored count capsules for per-engine row.** Node panel per-engine summary shows inbound + outbound as capsules (`IN 12` blue, `OUT 3` amber) instead of text (`in 12 · out 3`). **Why:** operator complaint was "V2 is text-heavy, low info-at-a-glance". Numerals inside colored capsules scan faster than dot-separated text.
- **Per-engine hue tokens.** `--engine-k8s` (k8s blue), `--engine-istio` (purple), `--engine-mesh` (teal). Applied to `EngineChip` as a colored dot + tinted bg. **Why:** REDESIGN.md line 154 originally called for a neutral engine chip to avoid competition with verdict; reversed after SRE consult — operators anchor to color, and the verdict callout still dominates via left-bar + tint + hero size. Same tokens can drive graph edge stroke color so panel and canvas share vocabulary.
- **Dropped `Effective status` eyebrow** on Node verdict callout. **Why:** tint + pill weight already say "this is the effective status"; the eyebrow was prose noise.
- **Reverse-direction blocker disclaimer** on Compare bidirectional label. **Why:** `blocked both ways` silently implied same reason both directions. Backend-sre flagged as silent-lie.
- **Dead-letter wording softened.** `⚠ dead-letter` → `⚠ no declared listener on this port` + tooltip explaining containerPorts are declarative-only. **Why:** backend-sre — treating containerPorts as ground truth pages people about working rules.
- **`Delete to allow` renamed `Matching subrule`** + clarifier "Removing this subrule unblocks the pair. Does not delete the surrounding policy". **Why:** backend-sre — tired operator was one `kubectl edit` away from nuking the whole AuthorizationPolicy.

---

## Compare / Reachability panel gaps (2026-07-16, second sweep)

Operator flagged the Compare panel was regressing on backend-provided signal even after P0–P3. Fixed sequentially:

- [x] **Reverse button was missing.** Reverse verdict was computed and displayed (`bidirectional.reverse`), but no affordance to swap SRC↔DST and inspect reverse blockers. Operator was told "pin DST" — friction. **Fix:** `.reverseButton` in the Reverse row (`⇄ Swap SRC ↔ DST`), plus the endpoint-row arrow becomes clickable (`.endpointArrowButton`) with the same swap action. Copy on the caveat text updated from "Pin DST as source" to "Swap to see reverse blockers".
- [x] **Src-side unavailable buried in engine grid.** RBAC-denied fetch of AuthorizationPolicies on the SRC namespace was only surfaced deep in the `Src selecting` column of the per-engine grid. Verdict banner above still read `✗ cannot reach` with no hint that it was computed off a half-visible policy set. Classic silent lie — allow rules the operator can't see could flip the answer. **Fix:** promoted an aggregated `Verdict computed with partial visibility` caveat above the blockers section, listing engine + side + reason. Message copy retained the specific RBAC verb (`Grant read on authorization.istio.io/authorizationpolicies`) so the fix is one-line kubectl.
- [x] **Selecting policies missing direction / coverage / ports / verdict detail.** Engine grid rendered each `srcSelecting` / `dstSelecting` entry as a bare `PolicyRef` + state word (`blocks` / `allows`). Node panel `PolicyCard` shows direction, ports, coverage (`deny all` / `restricted` / `unenforced`), and a one-line reason — Compare showed none. Operator could not tell ingress from egress selectors, or whether a policy was actually firing. **Fix:** `SelectingPolicyRow` component + enriched mockData shape (direction, coverage, ports, `verdictDetail`). Row header = `PolicyRef`; meta row = colored direction arrow + coverage capsule (when not `restricted`) + ports + state chip; body = one-line verdict reason (e.g. "DENY on notPrincipals — SRC SA … on deny list"). Same visual vocabulary as `PolicyCard` on Node so operator switches panels without re-learning shape.
- [x] **No mesh verdict.** Node `MeshBlock` exposes SA + mTLS + principals per workload, but Compare had zero mesh surface — operator had to open both Node panels and diff by eye to answer "does the mesh allow this pair?". **First pass:** rendered `MeshComparisonBlock` as a top-level tier between endpoint pair and blockers. Operator flagged the placement was wrong — mesh is an Istio-scoped concern, not a pair-scoped one; belongs under `Per-engine detail` for the istio row, next to that engine's own selecting policies + verdict. **Revised:** deleted the top-level block. Split mesh rendering into three pieces:
    - `MeshSideCard` in each engine grid `Src selecting` / `Dst selecting` column when the engine has mesh info (istio only). Compact SA + mTLS mode + sidecar-injected state; DST card also lists `principals` / `notPrincipals` chips.
    - `MeshAxisRow` in the `Verdict detail` column, stacked with the direction rows (see next bullet). Same three-axis derivation as the old block but sized to fit the column.
    - Mesh data moved from top-level `src.mesh` / `dst.mesh` to `engine.mesh: { src, dst, verdict, reason }` on the istio engine so k8s row cleanly omits the section instead of rendering a "no mesh" placeholder.
- [x] **Per-engine verdict detail was a single line.** Verdict column read one row per engine (`{verdict} · {verdictReason}`), even though traffic is evaluated per-direction (ingress = DST-side rules; egress = SRC-side rules) and — for istio — also per mesh axis. Operator could not tell which axis flipped the verdict. **Fix:** engine mockData grew `ingressVerdict` + `egressVerdict` fields; each renders as a `DirectionVerdictRow` (colored direction arrow + `✓ allow` / `✗ deny` chip + one-line reason). Istio row's `egressVerdict` is explicitly marked N/A ("no egress semantics — treated as pass-through") so an absent egress row does not read as "no problem there". Mesh axis (istio only) renders as a `MeshAxisRow` beneath, tinted by its own verdict.
