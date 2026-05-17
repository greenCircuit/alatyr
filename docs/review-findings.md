# Review Findings — branch `1-add-labels-for-workloads`

## Blockers

### 1. StatusKey strings fractured across 3 files ✅ fixing
Backend (`statusKeys.go`), old `node.go` constants, and frontend (`policies.ts`) all had different string values. UI must follow backend naming.

| Constant | Backend value | Old frontend value |
|---|---|---|
| `StatusIsolated` | `"air-gapped"` | `"isolated"` |
| `StatusNamespaceEgress` | `"ns-egress-access"` | not present |
| `StatusNamespaceIngress` | `"ns-ingress-access"` | not present |
| `StatusNamespaceFull` | `"ns-full-access"` | not present |

Frontend `STATUS_CFG`, `STATUS_LABELS`, `StatusKey` type union, demo data all need updating to match backend values.

---

### 2. `GetStatusKeys()` stale and incomplete
`build-nodes.go:156` returns hardcoded 4 keys — missing `ns-egress-access`, `ns-ingress-access`, `ns-full-access`. `/api/cluster-state` uses this to populate the status filter dropdown — new badges will never appear in the filter UI.

**Fix:** add the 3 new keys to the returned slice, or derive it from `statusKeys.go` constants dynamically.

---

### 3. Dead `buildStatusKeys` copy in `build-nodes.go`
`build-nodes.go` still contains a full `buildStatusKeys` implementation from before the refactor to `build-status-keys.go`. Go builds `build-status-keys.go` version but the dead copy diverges (no `innerNsEgress`/`innerNsIngress` logic). Delete the dead copy.

---

### 4. Double `GetPolicies` per namespace
`workload.go:BuildGraph` fetches policies twice per namespace:
- Line 30: populates `policiesByNS` (for status key computation)
- Line 48: `fetchPolicies()` fetches again for edge building

`policiesByNS` already has everything — flatten it instead of calling `fetchPolicies`.

```go
var policies []networkingv1.NetworkPolicy
for _, nsPolicies := range policiesByNS {
    policies = append(policies, nsPolicies...)
}
```

---

### 5. `coverage.out` committed
Generated file tracked in git. Add to `.gitignore`.

---

### 6. `embed.go` build tag removed
Original `embed.go` was guarded by `//go:build embed` so binary compiled without UI in dev. Guard removed and `embed_stub.go` deleted — `go build` now always requires `ui/dist` to exist. Breaks CI test jobs that don't run `npm run build` first.

---

### 7. `getConfigBadgesIngress()` stub
Empty stub with no return type in `build-status-keys.go`. Implement or delete.

---

## High

### 8. `IsIpBlockInternetAccess` too narrow

Current implementation only matches exact `"0.0.0.0/0"`. The `except` field is ignored entirely. Common real-world pattern `0.0.0.0/0` with `except: [10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16]` is internet access but returns `false`. Specific public CIDRs like `8.8.8.8/32` also missed.

| Approach | How | Pros | Cons |
|---|---|---|---|
| **A — exact `0.0.0.0/0`** (current) | string match on CIDR | trivial | misses except-clauses; misses specific public ranges |
| **B — `0.0.0.0/0` + except-aware** | match `0.0.0.0/0`, then check if excepts cover all RFC1918 space | handles most real clusters | complex except-coverage check; still misses specific public CIDRs |
| **C — config CIDRs** | anything outside `podCIDR` + `svcCIDR` from config = external | cluster-aware; already loaded | can't distinguish internet from other private networks the cluster reaches; flags internal corporate ranges too |
| **D — RFC1918 exclusion** | parse CIDR, flag if it contains any public IP space (not RFC1918, not loopback, not link-local) | comprehensive; catches specific public ranges | complex to implement; RFC1918 used for public services in some setups |
| **E — hybrid** | `0.0.0.0/0` always = internet even with excepts; anything outside config CIDRs + RFC1918 = internet | covers common cases | most complex; depends on config being set correctly |

**Chosen: simplified C** — if CIDR matches `podCIDR` or `svcCIDR` from config → internal, skip. Anything else (including `0.0.0.0/0`, specific public ranges) → internet. No RFC1918 math needed. Requires config to be wired into `IsIpBlockInternetAccess` (see #10).

```go
func IsIpBlockInternetAccess(block networkingv1.IPBlock, podCIDR, svcCIDR string) bool {
    return block.CIDR != podCIDR && block.CIDR != svcCIDR
}
```

---

### 9. `IsLabelMach` typo
Exported function in `internal/utils/labels.go`. Rename to `IsLabelMatch` and update all callers.

---

### 10. Config loaded but never wired into graph logic
`internal/config/` defines `Config` with `PodCIDR`, `SvcCIDR`, `CustomRules`. `main.go` calls `Load()`. None of it reaches `Builder` or `buildStatusKeys`. `CustomRules` evaluation (the `present`/`missing` peer matching design) is not implemented yet.

---

### 11. `workloadLabel` priority swapped
`build-nodes.go` now prefers `app` label over `app.kubernetes.io/name`. Most Helm-deployed workloads use `app.kubernetes.io/name` — swapping this changes displayed labels for the majority of real clusters. Revert to `app.kubernetes.io/name` first.

---

## Medium

### 12. Frontend `STATUS_LABELS` / `STATUS_CFG` had stale keys
Had entries for `dns-missing`, `no-policy`, `orphaned-selector`, `kube-api-access`, `ingress-exposed` — none emitted by backend anymore. Missing entries for new keys. (Being fixed alongside #1.)

### 13. Status filter dimming may lag layout
`applyDimming` reads `selectedStatusesRef` inside the `layoutstop` handler. If layout fires before React commits the ref update, stale dimming persists until next interaction. Low probability but worth noting.

---

## Low / Tests

### 14. `getNodePolicies` untested
Namespace-scoping of policy matching is untested.

### 15. `build_nodes_test.go` tests `build-status-keys.go` implicitly
Tests are `package graph` so they call whichever `buildStatusKeys` survives the build. If `build-status-keys.go` is deleted, tests still pass but against different (older) logic. Remove the dead copy in `build-nodes.go` (see #3) to make this unambiguous.
