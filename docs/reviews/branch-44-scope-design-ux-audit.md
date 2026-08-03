# Branch 44 Audit — Scope, Design, UX (backend-sre / ui-design-auditor / frontend-ux)

Scope: commits on `44-better-cidr-managment-and-can-reach` since it diverged from `main` (`f8a69fa...HEAD`) plus current uncommitted working-tree changes. Each agent reviewed independently. Criteria: scope creep, plain-english justification for breakage, user impact, better way to catch it.

---

## Backend (Go)

[FIXED] **Verified fixes from prior review, both confirmed correct in current diff:**
- `IsCIDRClusterAccessRule` (`internal/policy/networkPolicyHelpers.go:83-104`) now uses `cidrContains` with a mask-size guard (`outerBits > innerBits → false`), not string equality. Correctly handles `10.0.0.0/8` ⊃ `10.244.0.0/16` and correctly rejects the reverse (narrower peer inside pod CIDR). Test coverage (`networkPolicyHelpers_test.go:801-834`) explicitly asserts the narrower-doesn't-count case.
- CIDR nodes now enter `WorkloadByID`: `buildStore.go` calls `cache.RebuildWorkloadIndex()` after all engines run (uncommitted, `internal/store/buildStore.go:167-169`), and `RebuildWorkloadIndex` folds `EvaluationResults[*].Nodes` via `maps.Copy` (`internal/models/cache.go:31-34`). Belt-and-suspenders — either call site alone would fix it; redundant but harmless.

[IGNORE] **New finding — `internal/api/node-data.go:22-26`, scope creep, medium severity.** Dropping the `namespace` query param requirement from `getNodeInfo` is unrelated to CIDR/reachability on its face. If CIDR nodes have no namespace (so a namespace-required lookup would 400 on click), that's a real dependency and should be stated explicitly in the PR description — otherwise it reads as an unrelated contract change slipped into a feature branch, silently widening what `nodeId`-only callers can retrieve.

**Standing, unfixed (carried from prior review):**
[FIXED] - `AllowByNs` still carries `ActionDeny` except-rules alongside real allows (`buildRules.go` except-rule block, `evaluate.go:708`) — the "everything in AllowByNs is Action=ActionAllow" contract is still broken. Low blast radius today (only `deny_ns_count` logging trusts it) but a landmine for the next "allow-only" consumer. Cheapest guard: comment on the `AllowByNs` field in `models/evaluation.go` warning it's mixed, until split into `DenyByNs`.
[IGNORE] - Commit hygiene (`33f6ce7`, `88fb06b`) still has no narrative for why `CoverageExcept` or `IsCIDRClusterAccessRule` exist — only findable by reading the diff.

[N/A] **Test quality note (good, not a finding):** `TestGenerateTuples_IPBlock_Except_SelfNullifying` exercises the case where `except` fully covers the allow CIDR — exactly the adversarial input an overly-broad real-world NetworkPolicy will produce.

**Recommendation:** the two hard blockers from the prior review are resolved. Remaining items (`AllowByNs` contract, `node-data.go` param drop, commit messages) are non-blocking for correctness but should be justified or fixed before merge.

---

## UI Design

[FIXED] 1. **CIDR tier-1 block loses the `canPin` / "Pin as source" button and duplicates the whole tier1 shell instead of branching just the subtitle** — `ui/src/components/DetailPanel/views/WorkloadView.tsx:106-113`. In scope (CIDR node is a new concept, needs its own identity block) but implemented as a full duplicate rather than a shared shell with a conditional. Risk: the CIDR label uses `text-break` with no verified `min-width: 0` flex guard on its parent — a long IPv6 CIDR (`2001:0db8:...:7334/128`) can overflow the one panel where a misread digit means a wrong reachability conclusion. Catch: add a fixture with an IPv6 CIDR and a `/0` route, check panel width at 320px.

2. **Inline `style={{ marginTop: 6 }}` reintroduced** — `WorkloadView.tsx:99`. Not required by the CIDR feature; the file already uses `.module.css` classes elsewhere in this same branch (`ReachabilityView.tsx` / `DetailPanel.module.css:147-150`) but this spot didn't get the same treatment. Fix: promote to a `.cidrRefSpacing` class.

3. [FIXED]**`classifyCidr` reimplements private/public classification client-side**, competing with `IsIpBlockLanAccess`/`IsIpBlockInternetAccess` in `internal/policy/networkPolicyHelpers.go`, with a narrower private-range check (no CGNAT `100.64.0.0/10`, limited IPv6 link-local coverage) — `WorkloadView.tsx:31-44`. Two classifiers for one fact will drift; an operator could see "Public internet range" in the panel while the node badge (backend-derived) disagrees. Fix: pass the backend's classification down instead of reclassifying in the view.

4. **`--color-except` (`#fab005`) vs the DST-role amber (`#ffb54e`) are too close** — `ui/src/style/colors.css:37`, `graphTokens.ts:18`. `edgeStyles.ts:100-102` already flags this as a known near-miss in a comment, but the hex delta is small enough that a dim monitor makes the "except carve-out" edge hard to distinguish from a role-tinted edge — the exact confusion the code comment says it's avoiding. Fix: separate the hues by 15-20°+, or drop color as primary cue since shape (dashed + triangle-cross) already differentiates.

5. **`reachColHeader` restyle drops the `border-bottom` divider** — `DetailPanel.module.css:59-65`. Reasonable legibility call, low risk; just confirm the remaining `margin-bottom: 8px` is enough separation without it.

Cut list: fold the CIDR classification into the backend-supplied value (item 3); collapse the duplicated tier1 JSX branches into one shell with conditional subtitle/button (item 1).

---

## Frontend UX

- **Node-type filter silently drops `external` (and narrows `service`/`headless`) with no way back — highest severity.** `ui/src/components/FilterPanel/parts/constants.ts:83-84` replaces `ALL_TYPES = ['service','deployment','headless','external','cronjob']` (deleted from `graphStore.ts:108`) with `ALL_NODE_TYPES = ['deployment','cronjob','cidr']`. `filters.ts:53` still filters by exact type membership, so any node with `type === 'external'` — a real `NodeType` still declared in `internal/models/node.go:20` and referenced defensively in `MeshRollup.tsx:47` / `clusterStats.ts` — is now excluded from every view with zero checkbox to bring it back. This wasn't required to add `cidr` support; the fix was `[...OLD_TYPES, 'cidr']`, not a rewrite of the type vocabulary. If/when the backend emits `external` nodes, operators lose them silently. Catch: a test asserting `ALL_NODE_TYPES` is a superset of every `NodeType` the backend enum declares, not a hand-picked list.

[FIXED] - **CIDR node detail panel has no "Pin as source" affordance.** Same root cause as UI Design finding 1 — `canPin` isn't gated by node type upstream (`DetailPanel/index.tsx:74`), so the omission on the CIDR branch is deliberate-looking but unexplained. An operator opening a CIDR node — the whole point of this branch — expects the same pin workflow every other node gets, finds nothing, no disabled state or tooltip. If unsupported by design, the panel should say so. Catch: manual click-through — select a CIDR node, try "check reachability from this range" — before merge.

- **Client-side CIDR classification duplication is an integrity risk, not just a DRY issue.** Same `classifyCidr()` as UI Design finding 3 — flagged independently by both reviews, which raises its confidence. A `/31` or CGNAT range could show one classification in the panel and a different one on the node's own badge, with no way for the operator to know which is authoritative. Catch: consume the backend field, or add a fixture test asserting parity with `IsIpBlockLanAccess`/`IsIpBlockInternetAccess`.

- **`LabelStrip`/`StatusBadges` now return `null` instead of an explicit "no labels"/"—" empty state — scope creep, not CIDR-scoped.** `ui/src/components/DetailPanel/shared/badges.tsx:94`, `shared/status.tsx:10`. Neither is reached by the new CIDR branch, so this only changes behavior for existing node types with genuinely empty labels/statuses — an unrelated change bundled into a CIDR-feature branch. Operator debugging a zero-label pod now sees blank space instead of a confirming empty state, can't tell "loaded, nothing here" from "still loading." Catch: a snapshot test for the zero-labels/zero-statuses case on a plain `deployment` node.

---

## Conclusion

Two findings were raised independently by both UI Design and Frontend UX — that's the strongest signal in this audit: **client-side CIDR classification duplicating backend logic** (`classifyCidr` in `WorkloadView.tsx` vs `IsIpBlockLanAccess`/`IsIpBlockInternetAccess` in Go) and **the missing "Pin as source" button on CIDR nodes**. Fix the first before merge — it's the "quietly lies" failure mode this project treats as a correctness bug, not styling, and a single fixture test would close it. Resolve the second as an explicit product decision (support it or state it's unsupported), not a silent gap.

The single item worth blocking merge on outright is backend-independent: **`ALL_NODE_TYPES` dropping `external`** in `FilterPanel/parts/constants.ts`. This wasn't touched to support CIDR filtering — it's an accidental narrowing of the existing type vocabulary with no checkbox to recover the lost nodes. One superset assertion test prevents recurrence.

Everything else — the `node-data.go` namespace-param relaxation, the reintroduced inline style, the `--color-except` contrast gap, the `LabelStrip`/`StatusBadges` null-return change, the carried-over `AllowByNs` mixed-action contract — is real but non-blocking scope creep: each is a small, unrelated change riding along in a feature branch. None require architecture rework; all are one-line fixes or one added test. The backend's actual CIDR-containment logic (the hard part of this branch) is verified correct with adversarial test coverage already in place.

**Before merge:** fix the classification duplication and the `ALL_NODE_TYPES` regression. **Before requesting review next time:** split unrelated changes (node-data.go param, badges/status null-return) into their own commits or call them out explicitly in the PR description so reviewers aren't left guessing whether they're intentional.
