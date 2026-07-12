# Reachability: egress CIDR rules misclassified as "allowed elsewhere"

Status: **open** — to address later.

## The story

An operator pins alert-digest and clicks prometheus to check reachability. The
verdict is `deny`, and under **Egress (src side)** the panel lists, in the
"Allowed elsewhere (not this peer)" section:

```
To:        0.0.0.0/0
Direction: egress
Ports:     443/TCP
```

alert-digest has an egress rule allowing `0.0.0.0/0:443`. The panel frames that
as a near-miss — "you allow egress elsewhere, just not to prometheus." But
`0.0.0.0/0` *includes* prometheus's IP. So the rule may actually **permit** the
pair, or (if it carries `except: <clusterCIDR>`) genuinely not — and the panel
can't tell which. It presents an all-destinations egress rule as if it were an
unrelated near-miss, which misleads the operator about what the workload can
reach.

## Why it happens

Reachability matches egress rules to the dst by **ID string equality**
(`rule.DstID == dstNodeId || rule.DstID == dstNsNodeId`, `collectToSide` in
`internal/store/reachability.go`). A CIDR `DstID` (`0.0.0.0/0`) never equals a
pod's node ID, so it always falls through to `OtherAllowMatches` → rendered as
"allowed elsewhere."

To classify it correctly we'd need to ask "does this CIDR cover the dst pod's
IP" — and **two inputs for that are missing from the model**:

1. **`except` is dropped.** `internal/policy/k8spolicy/buildRules.go:271-273`
   keeps only `peer.IPBlock.CIDR` and discards `peer.IPBlock.Except`. So
   `0.0.0.0/0` and `0.0.0.0/0 except <clusterCIDR>` collapse to the same
   `DstID` string — one reaches the pod, the other carves the cluster out, and
   reachability can't distinguish them.
2. **No pod IP.** `models.WorkloadNode` (`internal/models/node.go:5-21`) carries
   ID/Label/Namespace/Type/Labels — no IP. There's nothing to containment-check
   a CIDR against.

Existing pieces that would help: `internal/policy/networkPolicyHelpers.go` has
`IsIpBlockClusterInternal` / `IsIpBlockInternetAccess` / `IsIpBlockLanAccess`
(they take a full `networkingv1.IPBlock` = CIDR+Except, not the bare string the
Rule now holds), and config carries pod/svc CIDR ranges.

## Options (pick when addressing)

**A. Reclassify only — no new data (small, UI/classification).**
A CIDR `DstID` on a pod→pod query isn't a near-miss *to this pod*; it's an
external-egress rule. Route CIDR-typed rules out of `OtherAllowMatches` into
their own "external egress" bucket (or drop them from the pod near-miss). Uses
the existing CIDR helpers on the string. Does **not** answer "does `0.0.0.0/0`
actually reach the dst" — it just stops the misleading pod near-miss.

**B. Correct containment — backend model change (correct, larger).**
- Preserve `IPBlock.Except` on `models.Rule` (`buildRules.go:271`).
- Add pod IP to `models.WorkloadNode`.
- In `collectToSide`, when `DstID` is a CIDR, containment-check
  `podIP ∈ (CIDR − Except)`. Then `0.0.0.0/0` (no except) → `AllowMatch` for the
  pod; `0.0.0.0/0 except cluster` → genuinely external, not a match.

This adds backend fields — crosses the pair-programming boundary; confirm before
implementing.

## Acceptance criteria

- An egress rule to `0.0.0.0/0` (no except) against an in-cluster pod dst is
  **not** shown as "allowed elsewhere." Under option B it reads as a permit;
  under option A it moves to an "external egress" section.
- `0.0.0.0/0 except <clusterCIDR>` is treated as external and does **not** count
  as reaching an in-cluster pod (requires option B — `except` plumbed).
- CIDR peers that are the label itself (external ranges) still render their CIDR,
  not a raw-id fallback.

## Related

- `internal/store/reachability.go` — `collectToSide` / `collectFromSide`,
  peer-match by ID equality.
- `internal/policy/k8spolicy/buildRules.go:271-273` — ipBlock expansion (drops
  Except).
- `internal/models/node.go` — `WorkloadNode` (no IP).
- `internal/policy/networkPolicyHelpers.go` — CIDR classification helpers.
