# Branch 44 — Maintainability Audit (independent pass)

Scope: `git diff f8a69fa..HEAD -- internal/ ui/src/` plus uncommitted working-tree changes (`internal/models/cache.go`, `internal/store/buildStore.go`, `ui/src/components/DetailPanel/views/WorkloadView.tsx`). This is a second, independent pass focused on maintainability/coupling, not a restatement of `docs/reviews/branch-44-cidr-reachability-review.md` — that review's two backend blockers (string-equality CIDR containment, CIDR nodes missing from `WorkloadByID`) and its UI blocker (`WorkloadView` faking a health verdict for CIDR nodes) are already marked `[FIXED]` there and I confirmed the fixes land correctly (`internal/policy/networkPolicyHelpers.go:75-96` now does real subnet containment; `internal/models/cache.go:31-34` + `internal/store/buildStore.go:167-169` fold `EvaluationResult.Nodes` into `WorkloadByID`; `ui/src/components/DetailPanel/views/WorkloadView.tsx:99-132` short-circuits the CIDR identity block and drops the hero/pin button). Test coverage for the CIDR/except/reachability logic (`networkPolicyHelpers_test.go`, `edge_test.go`, `reachability_test.go`) is genuine — each case asserts a distinct containment/coverage/precedence scenario, not padding.

**Verdict: the two previously-flagged blockers are correctly closed.** Everything below is medium/low.

*Correction after discussion:* an earlier draft of this review flagged `WorkloadByID` dropping CIDR nodes on namespace-filter narrowing (`a,b` → `a`) as a high-severity staleness bug. On review with the branch owner, that's request-scoped behavior working as intended — `WorkloadByID` is meant to reflect exactly the current request's scope, same as `EvaluationResults` narrowing correctly today, not to accumulate identity resolution across the cache's lifetime. No downstream caller resolves a node id outside what the current response just rendered, so there's no accumulation/eviction problem to solve here. Retracted; see "Not re-flagged" below.

---

## 1. `graphStore.ts` (state layer) imports from `FilterPanel/parts/constants.ts` (component layer)

- **Severity:** medium
- **File:** `ui/src/store/graphStore.ts:5`, `ui/src/components/FilterPanel/parts/constants.ts:81`
- **What changed:** `graphStore.ts` used to own its own type list (`const ALL_TYPES = [...]`, deleted). It now does `import { ALL_NODE_TYPES } from '../components/FilterPanel/parts/constants'` to seed `selectedNodeTypes`'s initial value (`graphStore.ts:122`).
- **Why it's a problem:** CLAUDE.md's own architecture note calls out `store/filters.ts` as the pure-derive layer that keeps store logic testable and decoupled from components. This inverts that direction — the store (state, supposedly the thing components depend on) now depends on a leaf component's constants file for a piece of domain data (which node types the backend can emit). Nothing stops a future FilterPanel refactor that deletes or moves `parts/constants.ts` from silently breaking `graphStore.ts`'s default filter state, and `graphStore.test`/`filters.test.ts` wouldn't catch it structurally (a rename compiles fine as long as the string array is still exported from *somewhere*, but the coupling itself is what a reader has to untangle six months from now — "why does my state file import from a components folder?").
- **Where it surfaces:** not a runtime bug today — TypeScript will catch a broken import at build time. It's a boundary violation that will bite whoever next reorganizes `FilterPanel/` and doesn't think to grep `store/` for cross-imports.
- **Fix:** move `ALL_NODE_TYPES` / `NODE_TYPE_LABEL` to `graphStore.ts` (or a shared `data/` module next to `data/policies.ts`) and have `FilterPanel/parts/constants.ts` import from there instead — same direction every other filter constant already flows (`ALL_ISSUE_TYPES`, `MESH_FILTER_LABEL` etc. all live in `constants.ts` and are consumed by the store's siblings, not the reverse). This is a one-file, mechanical move — not a redesign.

---

## 2. `classifyCidr` in `WorkloadView.tsx` duplicates backend CIDR classification with different semantics

- **Severity:** low
- **Files:** `ui/src/components/DetailPanel/views/WorkloadView.tsx:33-47` vs. `internal/policy/networkPolicyHelpers.go` (`IsIpBlockLanAccess`, `IsIpBlockInternetAccess`, `IsCIDRClusterAccessRule`)
- **Why it's a problem:** the backend already has a private/public/cluster-CIDR classification vocabulary that is pod/svc-CIDR-aware (config-driven) and unit-tested. The frontend now has a second, independent classifier that only knows RFc1918 + loopback + link-local, has no concept of the cluster's actual pod/svc CIDR, and is untested. The two will drift: a CIDR node showing `Private / LAN range` in the panel with no signal for "this is literally your pod CIDR" (which is a much more useful fact and is exactly what `IsCIDRClusterAccessRule` already computes) is a missed opportunity, and any future refinement to the backend's classification (the API-server-access carve-out, for instance) won't be reflected here unless someone remembers to hand-port it.
- **This is not a blocker** — the frontend can't call Go code, and a client-side heuristic for "private vs public" is a reasonable stopgap. Flagging so it's a conscious tradeoff, not an oversight: if `cluster-state` or `node-info` ever start returning a `PodCIDR`/`SvcCIDR`-aware classification for CIDR nodes, `classifyCidr` should be deleted in favor of it rather than kept as a permanent parallel implementation.

---

## Not re-flagged (checked, found sound)

- `WorkloadByID`/`EvaluationResults` narrowing on namespace-filter changes (`internal/models/cache.go:23-34`, `internal/store/buildStore.go:142-169`): full overwrite per request, not cumulative. Confirmed intentional — request-scoped resolution, not a staleness bug. See correction above.
- `IsCIDRClusterAccessRule` living in `internal/policy/networkPolicyHelpers.go` (shared package) rather than `k8spolicy`: this matches the existing pattern in that file (`IsIpBlockLanAccess` etc. are already shared CIDR helpers per CLAUDE.md's own file inventory), and it's keyed off the `models.CIDRIDPrefix` convention rather than any k8s-specific type, so it's engine-agnostic in practice even though only `k8spolicy` emits `cidr:`-prefixed rules today. When Istio's `IpBlocks[]` support lands (flagged as out-of-scope in `docs/backlog/cidr-peer-modeling.md`), this function is reusable for free. Correctly placed.
- `AllowByNs` still carrying `ActionDeny` except-rules (`buildRules.go`): already flagged in the existing review as medium/deferred; confirmed still present, not re-litigating.
- `WorkloadView.tsx` CIDR branch: mesh block and per-engine evidence section are correctly gated by data presence (`nodeInfo?.mesh`, `engines.size === 0`) rather than by `isCidr`, so they degrade gracefully to "nothing rendered" for CIDR nodes instead of rendering fabricated data. Pin-as-source button is correctly omitted in the CIDR branch (`WorkloadView.tsx:99-111` has no button; only the non-CIDR branch at 121-128 does).
- Test coverage on `edge_test.go` / `networkPolicyHelpers_test.go` / `reachability_test.go`: each new test asserts a distinct scenario (self-nullifying except, broader-than-pod-CIDR containment, two-CIDR except carve-out, ingress vs. egress mirroring) — no single-assertion padding.
