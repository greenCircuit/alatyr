# DesignPreview vs Live UI — Gap Punch List

Source: backend-sre audit (2026-07-16). LIVE = `views/*.tsx` + `shared/*.tsx`. REDESIGN = `DesignPreview/*.tsx`. Only real drops/demotions listed. "Equivalent" rows skipped.

Legend: `[ ]` open · `[~]` in progress · `[x]` fixed · `[skip]` intentionally not porting

---

## Node panel

- [ ] **N1 — Merged issue channels.** LIVE splits workload-scoped warn callout (`WorkloadView.tsx:62-74`) from cluster `NodeIssues` (`WorkloadView.tsx:79`, `issues.tsx:26,31-36`). REDESIGN one bucket (`panels.tsx:672-700`). Loses per-issue engine badge + Open-reachability deep-link on cluster issues.
- [x] **N2 — Deny/Allow neighbor split gone.** LIVE `rows.tsx:498-565` groups peers into "⛔ Denied by policy" / "✓ Allowed by policy" w/ distinct-peer counts + peer chips. REDESIGN flat `selectingPolicies` list (`panels.tsx:749-792`). Eval-order teaching moment lost.
- [ ] **N3 — Per-peer ports + L7 lost on neighbor rows.** LIVE `NeighborGroup` sub-rows carry own ports + L7 per peer (`rows.tsx:366-470`). REDESIGN `PolicyCard` peers show only label/kind/selector (`panels.tsx:275-289`).
- [x] **N4 — PostureRow deny-all danger border dropped.** LIVE red border on deny-all (`rows.tsx:515-517`). REDESIGN stripe/tint only (`panels.tsx:405-407`). Hard blocks should read at scroll speed.
- [ ] **N5 — ManifestButton highlight broken.** LIVE opens YAML w/ selector + peer stanzas highlighted (`rows.tsx:216-268,336-339,417-424`). REDESIGN `ManifestLink` mock (`panels.tsx:112-114`). Locate-in-200-line-policy feature gone.

## Edge panel

- [ ] **E1 — Mesh disagreement row missing.** LIVE `EdgeReachabilityBanner` shows per-mesh verdict alongside engines (`edge-composites.tsx:98-163`). REDESIGN engine strip only (`panels.tsx:844-851`).
- [x] **E2 — L7 negations dropped.** LIVE renders `notHosts/notMethods/notPaths` in red (`badges.tsx:53-58,92-94`). REDESIGN `L7Row` positive fields only (`panels.tsx:232-249`). Istio `notHosts` = real deny surface; silent drop = silent lie.
- [ ] **E3 — PolicyBundleView entirely missing.** LIVE `AffectedPairsList` per-pair port/L7 chips + click-narrow (`edge-composites.tsx:241-295`). No equivalent in REDESIGN.
- [ ] **E4 — `⊘ No effect` coverage chip missing at edge-level PolicyRow.** LIVE distinct hollow chip (`badges.tsx:23-27`). REDESIGN edge-level PolicyRow no coverage rendering (`panels.tsx:912-940`).

## Compare / Reachability

- [x] **C1 — PortsSummary intersection missing.** LIVE side-by-side SRC-egress vs DST-ingress per engine (`ReachabilityView.tsx:165-187`). REDESIGN headline `reachablePorts` only (`panels.tsx:1270-1275`). "SRC opens 8080, DST accepts 9090" mismatch invisible.
- [ ] **C2 — `allowOtherMatches` near-miss dropped.** LIVE "allowed elsewhere, widen selector" (`ReachabilityView.tsx:266-273,309-316`). Missing.
- [x] **C3 — PeerAuthentication culprit missing.** LIVE `BlockerRow.paSource` names PA behind mTLS block w/ YAML link (`ReachabilityView.tsx:215-220`). REDESIGN blockers = prose + `deleteToAllow` only (`panels.tsx:1346-1360`).
- [x] **C4 — PA chain + port-mode overrides gone.** LIVE `MeshCard.MtlsBlock` renders every PA, marks effective one w/ green border, per-port mTLS overrides, `mtls.issues` (`mesh.tsx:39-94`). REDESIGN `MeshBlock` flat mode + principals (`panels.tsx:463-522`). Strict-ns w/ permissive-port override = invisible.
- [x] **C5 — Waypoint (ambient mode) missing.** (Closed alongside C4 — waypoint field renders in MeshBlock.) LIVE ambient waypoint ns/name (`mesh.tsx:111-115`).
- [x] **C6 — Reverse-unavailable state missing.** LIVE `BidirectionalChip.error` renders `⚠ reverse unavailable` (`ReachabilityView.tsx:62-71`). REDESIGN assumes reverse always resolves (`panels.tsx:1244-1268`). Silent lie surface.

## Cross-cutting

- [ ] **X1 — Reverse verdict = single word in REDESIGN.** LIVE fetches full `reverse` `ReachabilityResult`; REDESIGN `bidirectional.reverse` (`mockData.ts:341`). No path to reverse blockers without swap+refetch.
- [ ] **X2 — CIDR / unresolved endpoint fallback missing in PeerRow.** LIVE `rulePeer` falls back to raw id / `any workload` (`rows.tsx:130-143`). REDESIGN assumes resolved `PolicyPeer` (`panels.tsx:255-290`); external CIDR peers have no rendering path.

---

## Additions REDESIGN brings (worth keeping)

- Staleness stamp on frame header (`panels.tsx:28-42`).
- Wildcard peer explicit render — closes `from: []` silent-empty gap (`panels.tsx:261-274`).
- Unresolved-culprit `?` badge on `PolicyRef` (`panels.tsx:78-93`).
- Compare `srcUnavailable/dstUnavailable` visibility caveat (`panels.tsx:1297-1326`).
- Dead-letter port warning on edge rows (`panels.tsx:908-910,932-939`).
- perEngine deny-driven hero verdict — closes "warn chip while engine denies" lie in LIVE `WorkloadView` (`panels.tsx:568-606`).
- Not-enrolled mesh explicit empty state (`panels.tsx:447-461`).

---

## Fix order (roughly by 3am-pager impact)

1. **E2** — L7 negations (silent-lie: deny surface hidden).
2. **C6** — Reverse-unavailable state (silent-lie: false reachable claim).
3. **C3** — PeerAuthentication culprit (mesh-mTLS blockers unfixable without it).
4. **C4** — PA chain + port-mode overrides (strict-ns exception invisible).
5. **C1** — PortsSummary intersection (mismatched port cases invisible).
6. **N2** — Deny/Allow neighbor split (eval order clarity).
7. **N4** — PostureRow deny-all border (scroll-speed read).
8. **N3** — Per-peer ports/L7 on neighbor rows.
9. **N5** — ManifestButton highlight wiring.
10. **C2** — `allowOtherMatches` near-miss.
11. **N1** — Split issue channels.
12. **E1** — Mesh disagreement row on Edge banner.
13. **E4** — `⊘ No effect` chip on edge PolicyRow.
14. **C5** — Waypoint field.
15. **X1** — Full reverse ReachabilityResult in mock.
16. **X2** — CIDR / unresolved peer fallback.
17. **E3** — PolicyBundleView port (biggest scope; last).
