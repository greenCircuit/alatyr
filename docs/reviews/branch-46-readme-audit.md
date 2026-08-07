# branch-46 README audit

Target: `/app/README.md` on branch `46-ui-bugs` (working tree, post Calico / Cluster Status / Issues drawer edits). Audited 2026-08-05 against `main` at commit `678b2bb` (Calico merge). Reader triple: SRE-at-3am, engineering exec, new adopter.

---

## Findings

### `[3am-pager]`

**README.md:207-210: [3am-pager] RBAC omits `watch` — informer clients will fail to start.**
Evidence: `internal/k8s/informerClient.go:74-77` builds `SharedInformerFactory` for pods, cronjobs, networkpolicies, namespaces; `internal/k8s/informerClient.go:93-98,106-109` do the same for AuthorizationPolicies / PeerAuthentications / GlobalNetworkPolicies. Every shared informer issues LIST + WATCH; the header comment at `informerClient.go:30-32` says so explicitly ("One cluster-scoped LIST + WATCH per GVK at startup"). RBAC in the README grants only `["get", "list"]`. Copy-pasting that manifest into a real cluster produces `Failed to watch *v1.Pod: unknown (get pods)` errors at startup and no cache sync — the process exits from `informerClient.go:120-124`.
Rewrite:
```yaml
rules:
  - apiGroups: [""]
    resources: ["pods", "namespaces"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["cronjobs"]
    verbs: ["get", "list", "watch"]
  # Optional — only required if Istio is installed.
  - apiGroups: ["security.istio.io"]
    resources: ["authorizationpolicies", "peerauthentications"]
    verbs: ["get", "list", "watch"]
  # Optional — only required if Calico is installed.
  - apiGroups: ["projectcalico.org"]
    resources: ["globalnetworkpolicies"]
    verbs: ["get", "list", "watch"]
```
The shipped Helm ClusterRole (`chart/templates/clusterrole.yaml`) has the same bug (also missing istio + calico + `watch`); fix both, not just the README.

**README.md:183-187: [3am-pager] Quickstart `go build -o graph .` fails cold — the binary embeds `ui/dist/` and the README never says to build it.**
Evidence: `embed.go:5-6` declares `//go:embed all:ui/dist`. `go build` fails with `pattern all:ui/dist: no matching files found` on a fresh clone where the UI hasn't been built. The reader following the quickstart top-to-bottom hits this within thirty seconds.
Rewrite:
```bash
cd ui && npm install && npm run build && cd ..
go build -o graph .
KUBECONFIG=~/.kube/config ./graph
# UI → http://localhost:8080
```
Or: publish a release binary and point the quickstart at the release tarball instead of `go build`. Either fixes the trap. Ignoring it doesn't.

### `[trust]`

**README.md:120: [trust] Screenshot reference `docs/selectNode.png` points at a file that does not exist.**
Evidence: `ls /app/docs/*.png` returns `fullGraph.png` and one `Screenshot 2026-06-05 at 16-50-47 ui.png`. There is no `selectNode.png`. The image tag renders as broken alt-text in the rendered README — first thing the reader sees below the reachability copy.
Rewrite: Either drop the image tag or generate the screenshot and check it in. If dropping, remove the entire line — do not leave a `<!-- screenshot: ... -->` placeholder there in its place, that pattern is what produced this bug elsewhere in the file.

**README.md:73, 84: [trust] Two live `<!-- screenshot: ... -->` placeholders that never got filled — the same pattern that produced the `selectNode.png` regression.**
Evidence: `README.md:73` (edge click panel) and `README.md:84` (tables view) both carry commented placeholders with no matching image below. On GitHub the comment renders as nothing at all — reader sees a heading, prose, and then no visual, wondering if the screenshot failed to load. Ship them or delete them.
Rewrite: Delete both placeholder comments. If a screenshot is planned, file it as an issue, not as a commented TODO in the README.

**README.md:169: [trust] "Informer-backed cache is wired... but every `/api/graph` still re-evaluates the full ns set" — the informer part is true, the re-evaluation caveat is stale for the request path being described.**
Evidence: `internal/k8s/informerClient.go:171-201` — every `Get*` call is an in-memory lister hit, no API roundtrip on warm requests. The re-evaluation claim is correct in the sense that `store.PopulateCache` (`internal/store/buildStore.go:89`) recomputes engine results per request, but the immediately-preceding phrase "informer-backed cache is wired" invites the reader to assume the graph itself is cached. It isn't. Two different caches (informer LIST cache vs. graph evaluation result) collapsed into one sentence.
Rewrite: "Informers back the k8s / Istio / Calico reads (`internal/k8s/informerClient.go`) — warm requests skip the API-server roundtrip. Per-request graph evaluation still runs every engine over the full ns set. Fine at a few hundred pods; will need debounced snapshotting at thousands."

**README.md:233: [trust] Dev section names a symbol that no longer exists — `graph.PolicySources`.**
Evidence: `grep -rn "PolicySources" /app/internal/` returns no function definition in `internal/graph/`. The engine registry moved to `store.Builder.defaultSources` (`internal/store/buildStore.go:46-52`) and the public accessor is `Builder.EngineNames()` (`internal/store/buildStore.go:76`). `CLAUDE.md` also names the stale symbol in multiple places — same drift, different file. A contributor reading the README to find the engine registry will `grep` and find nothing.
Rewrite: "Per-namespace fetches run concurrently across every registered engine (`store.Builder.defaultSources` in `internal/store/buildStore.go`)."

**README.md:165: [trust] "Stable enough to use against a real cluster" — undermined by the RBAC and quickstart bugs above.**
Evidence: If neither the RBAC nor the build command works out of the box (see 3am-pager findings), the "stable enough" line is aspirational, not observed. This is the exec's load-bearing sentence for the "should we adopt?" decision.
Rewrite: Do not soften; fix the two upstream bugs and the sentence becomes true. If either can't be fixed this pass, downgrade to "Runs against a real cluster with the RBAC bundle below and a UI build step — see Quick start."

**README.md:44: [trust] "Every workload that can reach `0.0.0.0/0` lights up" is precise for the WAN badge; the neighbouring `LAN⇆` claim in the table (line 140) is not equivalent evidence.**
Evidence: `internal/policy/networkPolicyHelpers.go:12-16` — `IsIpBlockInternetAccess` only fires on the literal `0.0.0.0/0`; other public CIDRs won't earn `WAN⇆`. That's documented in the code comment, not in the README. A reader running a security audit and expecting `WAN⇆` on a rule allowing `1.2.3.0/24` (a public range) gets a false negative and doesn't know it.
Rewrite: Add one line below the `WAN⇆` row: "Only the literal `0.0.0.0/0` counts as internet today. A rule targeting a specific public CIDR (e.g. `1.2.3.0/24`) does not flip this badge — see `internal/policy/networkPolicyHelpers.go`."

### `[ergonomics]`

**README.md:1-12: [ergonomics] Load-bearing first screen buries the reachability answer under three restatements of the same claim.**
Evidence: Line 3-4 (blockquote), line 6 (`**Stop guessing...**`), line 8 (`Policy is written per-engine and per-namespace...`) — three sentences saying "we intersect engines and show you the answer." The SRE at 3am gets one screen and expects the load-bearing sentence alone, not a three-verse chorus.
Rewrite (keep one, cut two):
> `> Read-only Kubernetes policy reachability + gap analyzer. Reads NetworkPolicy, Istio AuthorizationPolicy, Calico GlobalNetworkPolicy, and ambient-mesh mTLS from a live cluster; intersects them; shows what your pods can actually reach and where the gaps are. Single static binary.`
Then straight into the graph screenshot. Cut lines 6 and 8.

**README.md:36-45: [ergonomics] "Who it's for" is four paragraphs where two would land harder.**
Evidence: SRE, platform, DevOps, and security-audit paragraphs each say the same underlying thing ("look at what's actually reachable, faster than reading three YAMLs"). Reader-jobs collapse.
Rewrite: Cut to two — "SRE on call" and "Cluster inheritor / auditor" — and merge the mesh-rollout paragraph into the mesh section further down where it belongs.

**README.md:167-175: [ergonomics] Roadmap section reads as a defect list rather than a scope statement.**
Evidence: Five bullets, most of them phrased as absence ("not built yet", "product is silent today", "will need debounced snapshotting"). The exec skimming this concludes "not ready." The engineering exec is the reader here; the honest scope is a feature, not a liability, but the phrasing telegraphs weakness.
Rewrite: Reframe as "Deferred by design" with two bullets — cluster-scoped RBAC bundle + failure isolation, and Cilium / AdminNetworkPolicy engines — and drop the operational-survival bullet's re-litigation of the informer caveat already covered above.

### `[hygiene]`

**README.md batch — one pass:**
- Line 175: `docs/FEATURES.md` link is live (verified `ls /app/docs/FEATURES.md`). Keep.
- Line 235: `docs/arch/0003-istio-mesh-membership-and-mtls.md` — verified exists. Keep.
- Line 235: `docs/status-key-computation.md` — verified exists. Keep.
- Line 175: no trailing newline after the `[docs/FEATURES.md]` link block — cosmetic, defer.
- Line 27: "The graph everyone draws by hand is a lie" is good SRE voice. Leave.
- Line 44: `WAN⇆` uses U+21C6 in body copy but the badge table (line 138) uses the same glyph — consistent, verified.
- Line 214-217: "Optional — only required if Istio is installed" comment is correct in intent but the resource plural `peerauthentications` is correct (verified `internal/k8s/informerClient.go:95`).

No dead links, no stale ADR references, no typos worth filing individually. The `[3am-pager]` and `[trust]` findings above are where the leverage is.

---

## Cut list

- `README.md:6` — "Stop guessing what your pods can actually reach." Duplicate of the blockquote above it.
- `README.md:8` — "Policy is written per-engine and per-namespace, but reachability is emergent." Third restatement of the same claim.
- `README.md:73` — `<!-- screenshot: edge click panel showing cross-engine verdict... -->` placeholder that never got filled.
- `README.md:84` — `<!-- screenshot: tables view showing both Workloads and Policies tabs with rollup chips -->` placeholder that never got filled.
- `README.md:120` — `![Selected node](docs/selectNode.png)` — file does not exist; delete until the screenshot lands.
- `README.md:41-42, 43-44` — one of the four "Who it's for" paragraphs (merge DevOps + security-audit or drop the weakest).
- `README.md:169` — the "informer-backed cache is wired..." parenthetical inside the operational-survival bullet. It contradicts its own paragraph; cut and rewrite as noted above.

---

## Reader-fit closer

- **SRE on call at 3am.** Currently served on content, blocked on trust. The RBAC block is the first thing they copy; today it fails at startup. Fix the `watch` verb and this reader is unblocked.
- **Engineering exec.** Currently served on the "what does it do" scan (opening paragraph is strong) but underserved on "should we adopt?" — the roadmap section reads as defect list and the quickstart failure means anyone they hand it to comes back with "it doesn't build." Fix the quickstart and reframe the roadmap; the rest of the doc lands.
- **New adopter / evaluator.** Currently underserved. Load-bearing first screen is triple-stated (SRE-tolerable, evaluator-annoying), then the first concrete action they take (`go build`) fails, then the first click-through screenshot (`selectNode.png`) 404s. Three trust leaks before they reach the RBAC block. Fix the quickstart + the screenshot; the doc converts.

---

no questions, ready to draft

---

## Deep correctness pass

Second pass, top-to-bottom, verifying every factual claim against code (not prose polish). **32 claims checked. 9 wrong or misleading, 23 hold up.** Single most damaging bug: the "Per-workload issue surfacing" section (README:126) quotes an ambient-HBONE message that does not exist anywhere in the codebase — the actual detector emits a different sentence, so any reader who greps for the quoted string to find the code trips a trust wire immediately.

### New findings

**README.md:126: [3am-pager] Fabricated quote — the exact ambient-HBONE message shown in blockquotes does not exist in the code.**
The README quotes: `"This workload is in the Istio ambient mesh, but its NetworkPolicy only allows ingress on port(s) [8080]. Ambient delivers traffic through ztunnel on port 15008, which is blocked. Add an ingress rule allowing TCP port 15008 so mesh traffic can reach the workload."` — this string is nowhere in `/app` outside the README itself. The real detector emits `"Policy does not allow ztunnel HBONE port 15008; ambient ingress traffic is blocked"`.
Evidence: `internal/mesh/istio/detect.go:244` `fmt.Sprintf("Policy does not allow ztunnel HBONE port %d; ambient %s traffic is blocked", ZtunnelHBONEPort, sideLabel)`. Grep for any substring of the README quote returns only the README.
Rewrite:
> **"Policy does not allow ztunnel HBONE port 15008; ambient ingress traffic is blocked."** Same detector fires in the Issues drawer, scoped to what you're looking at. Culprit chip names the offending NetworkPolicy so the fix is `kubectl edit`.

**README.md:103-106: [trust] Issues-drawer detectors misfiled — three of four don't live where the README says they do.**
- Line 103: `layering.go` is named as home of `PolicyIssues` — it isn't. `PolicyIssues` lives in `internal/store/buildIssues.go:159`. `layering.go` only holds the ns-node scope-classifier helper.
- Line 106: `PeerAuthentication hygiene — STRICT destinations with non-mesh sources` is described as a standalone detector. There is no dedicated STRICT-hygiene scanner. That check runs inside `PolicyIssues` at `buildIssues.go:191-219` — when the reachability probe finds an allow-pair whose mesh verdict is `deny`, it emits a `MeshConflicts` issue. It is a per-pair reachability side-effect, not a hygiene sweep. A workload with a STRICT PA and no non-mesh caller anywhere will not surface.
- Line 105: Ambient HBONE detector is fine — lives at `internal/mesh/istio/detect.go:155` (`ValidateExternalRules`), just not in `store/`.
Evidence: `grep -rn "PolicyIssues" internal/store/` → `buildIssues.go:159`; `grep -rn "STRICT\|hygiene" internal/store/` → only test files.
Rewrite:
> - **Policy conflicts** — one engine allows a path, another (engine or mesh) denies it. `PolicyIssues` walks every allow pair with the same probe used by `/api/reachable` (`internal/store/buildIssues.go`).
> - **PeerAuthentication conflicts** — surfaced as `MeshConflicts` inside the same walk when a STRICT dst blocks a permitted pair. There is no standalone STRICT-hygiene sweep — a STRICT PA with no allowed caller today will not appear.
> - **Ambient HBONE trap** — `ValidateExternalRules` in `internal/mesh/istio/detect.go`, flagged per-workload at graph build.
> - **Missing DNS egress** — `internal/store/buildIssues.go`.

**README.md:104: [trust] "false-positives on global `to: {}` are filtered out" — the filter is real but keys on a different signal.**
Evidence: `internal/store/buildIssues.go:72-74` — the filter is `verdict.Reason == ReasonNoOpinion`, which fires when no policy governs egress at all (the workload has no egress NetPol). `to: {}` (allow-all-egress with no port list) also triggers this by producing an all-ports permitted verdict at `buildIssues.go:76-78`. Two filters, one described. Reader looking for the `to: {}` handling by grepping `to: {}` finds nothing.
Rewrite: "Two false-positive filters: workloads with no egress policy at all (`ReasonNoOpinion`) and workloads whose egress rule is unrestricted-ports (`verdict.AllPorts`). Both skip the check because they trivially permit port 53."

**README.md:65-69: [trust] Edge-panel banner example is invented — the real banner uses different wording and layout.**
README shows: `[DENY] end-to-end / reason: blocked by k8s / Engines: istio=allow k8s=deny / Mesh: istio=allow`. Real render (`ui/src/components/DetailPanel/shared/edge-composites.tsx:112-149`): headline is `→ Reachability: can reach` or `✗ Reachability: cannot reach`, followed by a free-text `result.reason` from the backend, then chip rows labeled `Engines` / `Mesh` with `engine: status` pill text. The `[DENY] end-to-end` prefix and `reason: blocked by k8s` phrasing do not appear in the source.
Rewrite (match reality):
```
✗ Reachability: cannot reach
<reason from /api/reachable>
Engines  [istio: allow]  [k8s: deny]
Mesh     [istio: allow]
```

**README.md:79: [trust] `→ Graph` button glyph is wrong — the actual button reads `◉ Graph`.**
Evidence: `ui/src/components/TablesView/WorkloadsTable.tsx:164` and `ui/src/components/TablesView/PoliciesTable.tsx:194` both render literal `◉ Graph`. `→ Graph` appears nowhere in the frontend.
Rewrite: replace both README occurrences of `→ Graph` with `◉ Graph`, or drop the glyph.

**README.md:104: [trust] "the single most common 'we shipped a NetPol and now nothing resolves' outage" — this is a claim about frequency with no data behind it.**
Evidence: superlative flagged by the writer's own rules (`Claims and evidence`, technical-writer.md:89-91). No metric, no telemetry, no bug-list link. The reader who acts on this expecting the detector to catch their DNS outage learns after the fact that it only fires on locked-egress workloads with no port-53 allow.
Rewrite: drop the superlative — "Workloads whose egress is locked but no rule allows port 53" is enough; the reader knows why it matters.

**README.md:22: [trust] "Nothing in `kubectl get networkpolicies` even hints these exist" — true but under-scoped.**
Evidence: `kubectl get networkpolicies` returns only `networking.k8s.io/v1 NetworkPolicy`; `GlobalNetworkPolicy` is `projectcalico.org/v3` and hidden from that query. Correct. But the same paragraph would apply to Cilium `CiliumClusterwideNetworkPolicy` and any AdminNetworkPolicy the cluster runs — no change needed here, just noting the claim narrowly holds for the engines shipped today.
No rewrite — verified correct, filed under `[trust]` only to note the narrow scope.

**README.md:169: [trust] `internal/k8s/informerClient.go` is name-checked in the "informer-backed cache" line, but the ADR at `docs/informers.md` is the operator-facing doc — README should point there instead.**
Evidence: `/app/docs/informers.md` exists. Line 169 sends a reader who wants operational detail into implementation source instead of the doc that explains sync + startup blocking.
Rewrite: swap the file reference — "Informers back the k8s / Istio / Calico reads (see [`docs/informers.md`](docs/informers.md))."

**README.md:170: [trust] AdminNetworkPolicy is still deferred — verified — but the roadmap phrasing "KEP-2091" is stale KEP number.**
Evidence: KEP-2091 is the correct KEP for AdminNetworkPolicy (`sig-network/2091-admin-network-policy`). No code path implements it (`grep -rn "AdminNetworkPolicy" /app/internal/ /app/*.go` returns nothing). Roadmap claim correct. Filed only as an "if this KEP is renumbered / graduated, this line will rot" note — no rewrite needed today.

### Verified-correct

- README.md:30 — ambient HBONE port 15008 → confirmed at `internal/mesh/istio/istio.go:44` (`ZtunnelHBONEPort = 15008`).
- README.md:22 — Calico `cluster-scoped, tiered, ordered first-match` with `Pass`/`Allow`/`Deny` → confirmed at `internal/policy/calico/calico.go:11-13,33` (tier + order fields), `utils.go:23-28` (tier-then-order sort), `buildIndex.go:110` (Pass short-circuits), `decode.go:98` (Pass mapped).
- README.md:21 — PA precedence `global → namespace → workload` → confirmed at `internal/mesh/istio/peerauth.go:23-27,138-140` (`workload → ns → mesh` walk, same chain traversed in the opposite listing order).
- README.md:52 — pods, cronjobs, namespaces as nodes → confirmed at `internal/models/node.go:18-24` (NodeType* enum) and `internal/graph/buildNodes.go:19-54` (cronjob and workload emission).
- README.md:54 — Calico DENY styling in UI → confirmed via `edge-composites.tsx:160-176` (`isDeny` styling) + Calico rules stamp `ActionDeny` at `internal/policy/calico/buildRules.go:131-135`.
- README.md:56 — `ipBlock.except` rendered as CIDR carve-outs (Calico) → confirmed at `internal/policy/calico/buildIndex.go:146-151` and `decode.go:100` (`notNets` passed through).
- README.md:57 — namespace-rollup view collapses each ns → confirmed at `ui/src/store/graphStore.ts:30,132,303` (`aggregateByNamespace` toggle) and `ui/src/components/PolicyGraph/index.tsx:411` (element rebuild on flag).
- README.md:58 — intersected "open only if every engine permits it" → confirmed at `internal/policy/statusKeys.go:90-129` (`IntersectPolicyStatus`: access = AND, locks = OR, HasL7 = OR).
- README.md:80 — Policies table exists per `(engine, ns, policy, action)` → confirmed at `ui/src/components/TablesView/PoliciesTable.tsx` + `index.tsx:12,155,170`.
- README.md:79 — status-rollup strip clickable to pivot → confirmed at `ui/src/components/TablesView/StatusRollup.tsx:16-17,44,72,108` (chip click toggles the status filter).
- README.md:80 — engine-rollup strip clickable → confirmed at `ui/src/components/TablesView/EngineRollup.tsx:2`.
- README.md:90 — Exposed callout (red, no-policy public workloads) → confirmed at `ui/src/components/ClusterStatusView/ExposedCallout.tsx` (referenced in index.tsx:26,185).
- README.md:91-92 — rule coverage + protection stats + single-engine coverage count → confirmed at `ui/src/store/clusterStats.ts:56-76` (coverageStats), `:84-89` (protectionStats), plus `singleEngineCoveredCount` referenced at ClusterStatusView index.tsx:146.
- README.md:92 — node status severity partition bar (worst per workload) → confirmed at `ui/src/store/clusterStats.ts:104-125` (`nodeSeverityStats`) plus `ProportionBar` at index.tsx:216-224.
- README.md:93 — cluster + per-namespace mesh rollup → confirmed at `ui/src/components/ClusterStatusView/ClusterMeshSummary.tsx` and `MeshRollup.tsx`.
- README.md:94 — top-risky table → confirmed at `ui/src/components/ClusterStatusView/RiskyWorkloadsTable.tsx`.
- README.md:95 — namespace drill-down → confirmed at `ui/src/components/ClusterStatusView/NamespaceTable.tsx`.
- README.md:101 — `/api/issues` endpoint → confirmed at `internal/api/api.go:43`.
- README.md:118 — reachability panel names the PA that forced the mesh verdict → confirmed at `internal/mesh/istio/detect.go:70-77` (`EffectiveSource` populated) + `ui/src/components/DetailPanel/views/ReachabilityView.tsx:383-385` (renders `From PA: <ns>/<name>`).
- README.md:141 — `API↑` fires on configured API-server CIDR → confirmed at `internal/policy/networkPolicyHelpers.go:22-24` (`IsIpBlockApiServerAccess` reads `cfg.ApiServerCIDRs` from `internal/config/config.go:26`).
- README.md:142 — `⊘` on both-locked-no-escape → confirmed at `internal/policy/statusKeys.go:41-47` (`StatusIsolated`).
- README.md:145 — `L7` badge on Istio L7 matchers → confirmed at `internal/policy/istio/buildPolicyStatus.go:144-145,162-163,194-195` (`HasL7 = true` on `ruleHasL7Match`) + `internal/policy/statusKeys.go:62-64` (emits `StatusL7Applied`).
- README.md:191-195 — `DEMO_MODE=true` env var + embedded `test-data/` → confirmed at `main.go:32-33` and `embed.go:8` (`//go:embed test-data`).
- README.md:224 — CRDs probed at startup, engine skipped cleanly if absent → confirmed at `internal/k8s/informerClient.go:88-110,143-169` (discovery probe, no fatal on `IsNotFound`).
- README.md:229 — `go test ./...` covers graph/policy/mesh/reachability → confirmed via test files present in all four package trees (`internal/graph/*_test.go`, `internal/policy/**/*_test.go`, `internal/mesh/istio/*_test.go`, `internal/store/reachability*_test.go`, `internal/mesh/istio/canReach_test.go`).
- README.md:230 — Vite proxy `/api → :8080` → confirmed at `ui/vite.config.ts:9-11`.

### Not verified

- README.md:197 — "real `observability` namespace dump" fixture. `grep 'observability' /app/test-data/` finds only two comment references in `30-payments.yaml` and `60-calico-globals.yaml`; there is no `namespace: observability` object in the fixture bundle. Either the observability dump was renamed/removed and the README lags, or the "real dump" claim was always aspirational. `[unverified]` — resolves by pointing to the file that carries the dump, or dropping the claim.
- README.md:104 — "false-positives on global `to: {}` are filtered out" — filter exists but keys on a different code signal (see finding above). Marked `[trust]`, not `[unverified]`.
