# Story: CIDR nodes should carry pod/svc/internet classification, not leave the UI to guess

## Status
Open — backend to implement. Frontend has a stopgap (string-parse guess) that
must be deleted once the backend field lands.

## Summary
Synthetic CIDR peer nodes (`k8spolicy/buildRules.go:297`) ship only
`ID` / `Label` / `Type` over the wire. The node has **no classification** of what
the range represents. The DetailPanel is left inferring it from the raw CIDR
string in TypeScript (`ui/src/components/DetailPanel/views/WorkloadView.tsx`
`classifyCidr`), which can only say "Private / LAN range" vs "Public internet
range" vs "Any address" by RFC1918 octet matching.

That guess is blind to cluster topology. An ipBlock inside the actual pod or
service CIDR (e.g. `10.42.0.0/24`) is the single most operationally interesting
case — it means a NetworkPolicy is reaching **in-cluster** workloads by IP rather
than by selector — and the UI currently mislabels it as a generic private LAN
range. Worse, if the pod/svc CIDR doesn't fall in RFC1918 at all, the string
guess calls it "Public internet range" outright. That is the styleguide's
"quietly lies" failure: a confident, wrong label on a security-relevant node.

## Why the backend already knows this and the UI cannot
Classification is a function of **cluster config**, which lives server-side only:

- `internal/config/config.go:24-25` — `PodCIDR` / `SvcCIDR` loaded at startup.
- `internal/policy/networkPolicyHelpers.go:35` — `IsIpBlockClusterInternal`
  already computes `cidrContains(PodCIDR) || cidrContains(SvcCIDR)`.

The helper folds pod and service into one bool and the result is thrown away —
never stamped on the node. The frontend has no access to `PodCIDR`/`SvcCIDR`
(not exposed via any API), so it *structurally cannot* classify correctly.
Pushing config values to the client to let TS redo the math would scatter CIDR
logic across two languages for no gain. Classification belongs where the config
already is.

## Value add
- **Truthful node identity.** "Pod CIDR" / "Service CIDR" / "Public internet" /
  "Private LAN" instead of an RFC1918 guess that misfires on the exact ranges an
  operator most needs flagged.
- **Surfaces IP-based in-cluster reach.** A policy admitting `10.42.x.x/24`
  ipBlock is bypassing selector-based intent — the highest-signal thing to see on
  a CIDR node. Today it's indistinguishable from "some private LAN box."
- **One source of truth.** Kills the TS `classifyCidr` stopgap; CIDR math stays
  in Go next to the config it depends on.
- **No config = honest unknown.** When `PodCIDR`/`SvcCIDR` are unset, backend
  emits "unclassified" rather than a false "public internet" verdict. Never a
  confident lie.

## Shape (suggestion — backend implements)
1. Split the classifier so it distinguishes pod vs svc, not a merged bool:
   `cidrContains(cfg.PodCIDR, block.CIDR)` vs `cidrContains(cfg.SvcCIDR, block.CIDR)`.
   Return an enum: `pod` / `service` / `api-server` / `internet` (`0.0.0.0/0`) /
   `lan` / `unclassified` (config empty → cannot tell).
2. Carry it on the node. Add a field to `models.WorkloadNode` (e.g.
   `CIDRKind string json:"cidrKind,omitempty"`), populated only for
   `NodeTypeCIDR`. Stamp it at `k8spolicy/buildRules.go:297` where the CIDR node
   is synthesized (config is reachable there via `config.Get()`).
3. Frontend reads `node.cidrKind`, renders the word directly, and **deletes**
   `classifyCidr`. Fall back to "unclassified range" when the field is empty.

## Backend touchpoints (user implements)
- `internal/policy/networkPolicyHelpers.go` — pod-vs-svc-aware classifier
  (extend/replace `IsIpBlockClusterInternal`).
- `internal/models/node.go` — new `CIDRKind` field on `WorkloadNode`.
- `internal/policy/k8spolicy/buildRules.go:297,305` — stamp `CIDRKind` on the
  synthesized CIDR node (and its except carve-out nodes).

## Frontend touchpoints
- `ui/src/data/policies.ts` — add `cidrKind?` to `WorkloadNode`.
- `ui/src/components/DetailPanel/views/WorkloadView.tsx` — render `node.cidrKind`;
  delete the `classifyCidr` string-parse stopgap.

## Files
- `internal/config/config.go:24-25`
- `internal/policy/networkPolicyHelpers.go:33-54`
- `internal/policy/k8spolicy/buildRules.go:293-311`
- `internal/models/node.go:3-11`
- `ui/src/components/DetailPanel/views/WorkloadView.tsx`
