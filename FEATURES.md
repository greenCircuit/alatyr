# FEATURES — what each engine actually reads

Ground truth for "does the graph account for field X". Every claim below is what the
code does today, cited at `file:line`. Fields listed as **not read** are silently
ignored unless the row says the engine fails loud — a rule that depends on an
ignored field is rendered wrong or not at all, and that is the thing to check
first when the graph disagrees with the cluster.

Three policy engines are registered (`internal/store/buildStore.go:46`):
`k8s`, `istio`, `calico`. One mesh source: `istio` (ambient only). Every engine
returns rules + per-workload `PolicyStatus`; the graph layer renders rules as
edges and intersects statuses into node badges.

Jump to: [k8s](#1-k8s--kubernetes-networkpolicy) · [istio](#2-istio--authorizationpolicy-ingress-only) ·
[calico](#3-calico--globalnetworkpolicy) · [status keys](#4-status-keys-node-badges) ·
[mesh](#5-mesh-status-istio-ambient) · [issues](#6-issue-types-and-how-each-is-determined)

---

## 0. Cross-engine capability matrix

| Capability | `k8s` | `istio` | `calico` |
|---|---|---|---|
| Label expression selectors (`matchExpressions` / Calico selector language) | not read — policy silently selects nothing | not read — `WorkloadSelector` is matchLabels only | full expression language (vendored libcalico parser) |
| Port ranges | `endPort` stored, never expanded | n/a — numeric ports only | rejected, engine errors |
| Named ports | stored as a string, never resolved | n/a | rejected, engine errors |
| Identity peers (ServiceAccount / JWT principal) | n/a | not read — no edge produced | rejected, engine errors |
| CIDR peers | `ipBlock` + `except` | status only, no edges | the engine's primary peer dimension |
| Egress rules | yes | no — AuthorizationPolicy is ingress-only | yes |
| Cluster-scoped policy kind | n/a | root-ns policies: status only, no edges | the only kind read (namespaced `NetworkPolicy` ignored) |
| L7 matchers | n/a | hosts / methods / paths captured | n/a |

### Out of scope — not partial support, not modelled at all

- Istio sidecar mode; ambient only.
- Istio egress control (`Sidecar`, `ServiceEntry`).
- Namespaced Calico `NetworkPolicy` (only `GlobalNetworkPolicy` is fetched).
- Istio `targetRef` / `targetRefs` (Gateway-API-attached AuthorizationPolicy).
- Waypoint binding and anything L7 that happens at the waypoint.
- Admission-time policy (Kyverno, OPA) — does not shape runtime reachability.

---

## 1. `k8s` — Kubernetes NetworkPolicy

Fetch: `GetPolicies(ns)` per requested namespace, in parallel; one ns failure
fails the engine (`internal/policy/k8spolicy/evaluate.go:33`).

### Read

| Field | Use |
|---|---|
| `spec.podSelector.matchLabels` | Which workloads the policy protects. Empty → the namespace node stands in for "every pod" (`buildRules.go:99`) |
| `spec.policyTypes` | Direction locks. Empty list → ingress locked; egress locked only when `spec.egress` is non-empty (`buildRules.go:110-131`, `buildStatusKeys.go:115`) |
| `spec.ingress[].from[]`, `spec.egress[].to[]` | Peer expansion → one rule per matched peer (`buildRules.go:251`) |
| `peer.podSelector.matchLabels` | Same-ns pod peers; empty selector collapses to the ns node with `Coverage=allow all ns` |
| `peer.namespaceSelector.matchLabels` | Matched against **namespace labels**; peer becomes the ns node |
| `peer.ipBlock.cidr` | Synthesizes a `cidr:<block>` node, classified WAN / pod / svc / LAN (`policy/networkPolicyHelpers.go:83`) |
| `peer.ipBlock.except[]` | Emitted as separate `ActionDeny` rules with `Coverage=except` — carve-outs, ranked above the covering allow in reachability (`store/reachability.go:128`) |
| `ports[].protocol` / `.port` / `.endPort` | Protocol defaults to TCP; numeric or named port; `endPort` kept as a range (`buildsPorts.go:12`) |
| `metadata.creationTimestamp` | Stamped on every `PolicyRef` |

### Not read

- **`matchExpressions` — anywhere.** Label matching is `matchLabels`-only
  (`utils.LabelsMatch`). A `podSelector` with only `matchExpressions` is not
  catch-all (`isCatchAll` requires zero of both, `evaluate.go:100`) and matches
  no labels either, so **the policy selects nothing and vanishes from the graph**.
  Same for peer `podSelector` / `namespaceSelector`. This is the single biggest
  fidelity gap in this engine.
- **`namespaceSelector: {}` peers** (catch-all ns, no podSelector) — dropped
  outright, no rule emitted (`buildRules.go:258`).
- **Named ports** are carried as a string, never resolved against container
  specs. Anything that reasons about port numbers (DNS check, HBONE check) will
  not see them.
- `endPort` is stored but not expanded — reachability compares `Port`/`EndPort`
  literally, except the DNS detector which does a range check for 53.

### PolicyStatus signals (`buildStatusKeys.go:103`)

Per workload, accumulated over the policies whose `podSelector` matches it:

- `IngressLocked` / `EgressLocked` — from `policyTypes` as above.
- `CrossNS` — an ingress/egress peer has a `namespaceSelector` whose
  `kubernetes.io/metadata.name` label is not the policy's own namespace. A
  label-based ns selector (no metadata.name key) therefore also counts as cross-ns.
- `InternetEgress` / `InternetIngress` — an `ipBlock` of exactly `0.0.0.0/0`.
- `LanEgress` / `LanIngress` — any `ipBlock` that is not `0.0.0.0/0`, not inside
  `podCIDR`/`svcCIDR`, and not inside `apiServerCIDRs` (config, `internal/config/defaultConfig.yaml`).
- `ApiServerEgress` — egress `ipBlock` inside a configured `apiServerCIDRs` entry.
- `InnerNsEgress` / `InnerNsIngress` — catch-all pod peer scoped to the policy's own ns.
- `HasL7` — never set. k8s NetworkPolicy has no L7.

---

## 2. `istio` — AuthorizationPolicy (ingress only)

Fetch: `GetAuthorizationPolicies(ns)` per namespace, plus the root namespace
`istio-system` (`internal/policy/istio/evaluate.go:42`). AuthorizationPolicy
gates traffic **into** the selected workload; Istio egress (Sidecar,
ServiceEntry) is out of scope, so every rule this engine emits is
`DirectionIngress`.

### Read

| Field | Use |
|---|---|
| `spec.action` | `ALLOW` (0) and `DENY` (1) only. **`AUDIT` and `CUSTOM` are dropped** — no rules, no status contribution (`evaluate.go:113`, `buildPolicyStatus.go:184`) |
| `spec.selector.matchLabels` | Protected workloads; nil or empty = every workload in the policy's ns (`utils.go:22`) |
| `spec.rules[].from[].source.namespaces` | The **only** source dimension that produces edges — each named ns resolves to that ns node (`buildRules.go:322`) |
| `spec.rules[].to[].operation.ports` | Numeric strings → `models.Port` with protocol TCP implied; unparseable entries are logged and skipped (`buildPorts.go:19`) |
| `operation.hosts` / `.methods` / `.paths` | Captured into `L7Match` on the rule; drives the `l7-applied` badge |
| `metadata.creationTimestamp` | Stamped on every `PolicyRef` |

Shape semantics (`buildRules.go:84-243`) — these decide the `Coverage` an edge carries:

| Policy shape | ALLOW | DENY |
|---|---|---|
| no `spec.rules` key | `deny all` | `unenforced` |
| `rules: [{}]` | `unenforced` | `deny all` |
| rule with `from` but no `to` | `allow all` | `unenforced` |
| no `from`, `to` with no narrowing | `allow all` | `deny all` |
| no `from`, `to` with ports only | `allow all` | — |
| no `from`, `to` with L7 | `restricted` | — |
| `from` + `to` | `restricted` (ns peer → `allow all ns`) | same |

### Not read (rules)

- **`source.principals`, `notPrincipals`, `requestPrincipals`, `notRequestPrincipals`** —
  service-account and JWT identity is invisible. A policy that allows only
  `cluster.local/ns/foo/sa/bar` produces **no source edge at all**;
  `expandFromSource` returns nothing when `namespaces` is unset.
- **`source.ipBlocks`, `remoteIpBlocks`, `notIpBlocks`, `notNamespaces`** — no
  edges. They *do* feed status accumulation (below).
- **`operation.notPorts`, `notHosts`, `notMethods`, `notPaths`** — not subtracted
  anywhere; `notHosts`/`notMethods`/`notPaths` only flip `HasL7`.
- **`spec.rules[].when[]`** conditions — entirely ignored.
- **`targetRef` / `targetRefs`** (Gateway-API-attached policies) — ignored; only
  `selector` is honoured.
- **Root-namespace policies do not produce edges.** `istio-system` policies are
  fetched and included in per-workload status + the detail panel, but rule
  expansion selects workloads by the policy's own namespace, so a mesh-wide
  policy is missing from the graph (`evaluate.go:34` TODO).

### PolicyStatus signals (`buildPolicyStatus.go:130`)

Candidate set = ns-local policies + root-ns policies that select the workload.
Two short-circuits, then an allow-minus-deny walk:

1. Any **DENY** policy with a wildcard-source rule (`from` empty or a nil
   `source`) → `{IngressLocked: true}` and nothing else. Deny wins.
2. Any **ALLOW** policy with a wildcard-source rule → `InnerNsIngress` +
   `CrossNS`.
3. Otherwise: accumulate `namespaces` / `notNamespaces` / `ipBlocks` /
   `notIpBlocks` per action, subtract deny from allow (`combinePolicySignals`),
   then flip: own ns present → `InnerNsIngress`; other ns present → `CrossNS`;
   surviving CIDRs classified into `InternetIngress` / `LanIngress`;
   any surviving `notNamespaces` over-claims both ns dimensions, any surviving
   `notIpBlocks` over-claims both CIDR dimensions.

`IngressLocked` is set whenever any policy selects the workload. Egress fields
and `ApiServerEgress` are never set — the badge deliberately shows ingress-side
intent only. `HasL7` fires when any selecting rule carries a host/method/path
matcher or its `notX` form.

---

## 3. `calico` — GlobalNetworkPolicy

Fetch: `GetGlobalNetworkPolicies()` once, cluster-scoped
(`internal/policy/calico/evaluate.go:189`). Selector expressions are parsed with
the **vendored libcalico parser** (`internal/thirdparty/calicoselector`), so the
full expression language is supported — not just `matchLabels`.

**Namespaced Calico `NetworkPolicy` is not read.** Only the cluster-scoped
`GlobalNetworkPolicy` kind is fetched.

### Read

| Field | Use |
|---|---|
| `spec.tier`, `spec.order` | Precedence sort: tier, then order, then name. Unset order sorts **last** (`utils.go:16`) |
| `spec.selector` | Per-workload match, full Calico expression; `""` / `all()` = namespace-wide by construction (`NamespaceWide` on the rule) |
| `spec.namespaceSelector` | First-pass ns filter, evaluated once per namespace; empty = all namespaces |
| `spec.types` | Direction governance; when empty, Calico's default is derived (`decode.go:172`) |
| `spec.ingress[]` / `spec.egress[]`, in array order | First-match resolution — order is load-bearing, not a union |
| rule `action` | `Allow`, `Deny`, `Pass` (ends this policy's contribution to the bucket), `Log` skipped as verdict-neutral |
| rule `protocol` | Stamped onto the port |
| peer `nets`, `notNets` | The peer dimension. `notNets` carve out first; empty `nets` = every bucket |
| peer `ports` | Single numeric ports only |
| `metadata.creationTimestamp` | On the `PolicyRef`, alongside `Order` and `Tier` |

Peer side is `destination` for egress rules, `source` for ingress.

### Fails loud (engine returns an error, no partial graph)

Rather than silently broadening a rule, `checkPeerSupported` /`convertPorts`
reject (`decode.go:112`, `decode.go:147`):

- peer `selector`, `notSelector`, `namespaceSelector`
- peer `serviceAccounts`, `services`
- peer `notPorts`
- named ports
- port **ranges** (`minPort != maxPort`)

A cluster using any of these gets an explicit `/api/graph` error naming the
policy and rule index. That is deliberate: the bucket model keys on
`(cidr, port)`, and a label-scoped peer with no CIDR would render as
match-everything and invert the verdict.

### Resolution model (`buildIndex.go`, `buildRules.go`)

Buckets = every `(cidr, port)` tuple any selecting policy names, plus a seeded
`(0.0.0.0/0, all-ports)` bucket. Per bucket, policies are walked in precedence
order and the first whose selector matches the workload wins. Consequences worth
knowing:

- A bucket whose winner is the same rule that already won the direction-wide
  bucket is dropped (`dropRestatedEdges`) — otherwise a catch-all winner would
  draw arrows to CIDRs its policy never named.
- Direction governed but no direction-wide verdict resolved → a synthetic
  `deny all` baseline rule is added, because Calico default-denies the rest.
- Direction not governed by any selecting policy → an `unenforced` marker;
  reachability reads it as "no opinion", not as an allow.
- `0.0.0.0/0` buckets do not synthesize a CIDR node; the coverage marker carries
  the direction-wide semantic (same convention as k8s blanket rules).

### PolicyStatus signals (`buildPolicyStatus.go:21`)

- `IngressLocked` / `EgressLocked` — any selecting policy governs the direction.
- `InternetEgress` / `InternetIngress` / `LanEgress` / `LanIngress` — from the
  CIDR class of **Allow** winners only.
- `CrossNS`, `InnerNs*`, `ApiServerEgress`, `HasL7` — never set by this engine.

---

## 4. Status keys (node badges)

One shared catalog for every engine (`internal/policy/catalog.go`):
`internet-ingress`, `internet-egress`, `internet-full`, `lan-ingress`,
`lan-egress`, `lan-full`, `api-server-egress`, `air-gapped`, `cross-namespace`,
`ns-egress-access`, `ns-ingress-access`, `ns-full-access`, `l7-applied`.

Engines never emit keys directly. They emit `PolicyStatus` (intent only — flip a
dimension **only** when a policy explicitly references it), and
`policy.DeriveStatusKeys` maps it to keys (`internal/policy/statusKeys.go:14`):

- **Internet is derived, not just declared:** an *unlocked* direction counts as
  internet access on that side (`!EgressLocked || InternetEgress`). Both sides →
  `internet-full`.
- LAN, api-server: explicit CIDR peers only.
- `air-gapped` — both directions locked and no internet/LAN/api-server escape hatch.
- `ns-*-access` — catch-all peer scoped to the workload's own namespace; both → `ns-full-access`.
- `l7-applied` — `HasL7`.

Each node carries **per-engine** keys (`StatusesBySource`, shown in the detail
panel) and one **effective** set. Effective = `IntersectPolicyStatus` then derive
(`statusKeys.go:90`): engines with an all-zero status are dropped as no-opinion;
access dimensions AND (every engine must permit), locks OR, `HasL7` OR, and an
unlocked direction is treated as transparent for the dimensions it gates.

---

## 5. Mesh status (Istio ambient)

Single mesh source, `internal/mesh/istio/`. Sidecar mode is not modelled.

### Membership

Resolved for every workload during cache population
(`buildMeshMembership.go:16`):

- Namespace enrolled ⇔ ns label `istio.io/dataplane-mode: ambient`, or the ns is
  `istio-system` / `istio-ingress`.
- Workload label **wins over** the ns label: `dataplane-mode: ambient` opts a
  workload into a non-enrolled ns; `dataplane-mode: none` opts it out of an
  enrolled one.
- Exception: Istio's own components (`app=istiod`, `app=ztunnel`,
  `k8s-app=istio-cni` in `istio-system`; `app=istio-ingress` in `istio-ingress`)
  carry `dataplane-mode: none` because they *are* the proxies — the opt-out is
  ignored for them (`utils.go:18`).
- Waypoint binding (`istio.io/use-waypoint`) is **not** resolved yet; the label
  constants exist, nothing consumes them.
- Namespace nodes get the ns-level verdict so the mesh filter can select whole
  namespaces; they never count toward workload metrics.

### mTLS verdict

PeerAuthentications for the workload's ns + the root ns, walked in Istio's
precedence order (`peerauth.go:27`):

1. **workload-scope** — ns-local PA with a `selector` whose `matchLabels` match
   the workload (matchLabels only). A root-ns PA with a selector counts only for
   workloads inside `istio-system`.
2. **namespace-scope** — ns-local PA with no selector.
3. **mesh-scope** — root-ns PA with no selector.

Within each bucket the **oldest** PA wins (creationTimestamp, name as tiebreak).
`UNSET` falls through to the next scope; if every scope is UNSET or there is no
PA at all, the verdict is `permissive` (the install default) and the "all unset"
hygiene issue fires. `portLevelMtls` runs the same walk per referenced port and
lands in `PortOverrides`; ports nobody references are absent and inherit the
workload verdict.

A PA fetch failure never invents a verdict — membership still resolves from
labels and the verdict becomes `unknown`, plus a `failed to fetch` issue.

### Mesh reachability (`detect.go:49`)

`CanReach(src, dst, port)` denies only when the destination requires STRICT —
port override first, else the workload verdict — **and** the source either is not
in the mesh or has mTLS `disable`. Everything else allows. This is a transport
verdict, fused with the policy verdict by
`store.IsEndpointsReachable`; a mesh deny overrides an all-engines-allow.

### Mesh metrics

Counted in the same pass: namespaces enrolled, namespaces partially enrolled (an
enrolled ns with at least one opted-out workload), workloads enrolled, and mTLS
verdict tallies (strict / permissive / disabled / unset / unknown).

---

## 6. Issue types and how each is determined

Aggregated by `store.GetIssues` (`internal/store/buildIssues.go:18`): policy
conflicts + DNS are computed per request; mesh issues are computed once during
cache population and parked on the cache.

| Type (wire value) | Scope | How it fires |
|---|---|---|
| `no dns` | src → DNS pod, per engine | DNS target from config (`dnsNs`, `dnsLabels`; default `kube-system` / `k8s-app=kube-dns`). For every workload and engine, the egress verdict toward DNS is computed. `no-opinion` skips. `permitted` with all-ports skips; `permitted` on a port set containing neither 53 nor a range covering it → flagged with the allowing policies as culprits. Any blocking reason → flagged with that reason (`buildIssues.go:31`) |
| `policy conflict` | src → dst pair | Some engine emits an ALLOW rule for the pair, but the fused verdict across all engines is `deny`. One issue per **blocking** engine (attribution follows the blocker, not the permitter), carrying that engine's ingress/egress culprits, reasons, and the allows that did match (`buildIssues.go:159`) |
| `partial access` | src → dst pair | Same trigger as above, reclassified. Either (a) an endpoint is a synthetic pod/svc-range CIDR node and the blocker simply has no rule naming CIDRs — overlapping scope, not disagreement; or (b) an endpoint is a namespace node and `classifyLayering` finds at least one pod-granular path under it that *is* reachable — a coarse ns allow layered over a pod-granular baseline (`layering.go:35`) |
| `cidr scope mismatch` | src → dst pair | The blocking engine allows a CIDR strictly **inside** the aggregate CIDR node the edge points at (a /32 under a /24). `0.0.0.0/0` is excluded, since it contains everything. Not demoted to info: the API objects can't say whether the narrowing was least-privilege or a fat-fingered mask (`buildIssues.go:280`) |
| `mesh conflict` | src → dst pair | The pair has a policy ALLOW rule, but the mesh transport verdict is `deny` (dst STRICT, src plaintext or disabled). Culprit is the effective PeerAuthentication, referenced with source `pa` so the YAML button hits the PA fetcher (`buildIssues.go:191`) |
| `mesh transport blocked` | ambient workload, per policy+direction | For each enrolled ambient workload, every other engine's ALLOW rule that restricts ports without including ztunnel's HBONE port **15008** is flagged — but only when the peer on the other end also participates in the mesh (external CIDRs and non-ambient peers never tunnel), and only when that engine has no allow-any-peer stanza already opening HBONE in that direction (`mesh/istio/detect.go:155`) |
| `mesh policy` | workload | PeerAuthentication hygiene promoted from the mTLS walk (`peerauth.go`): **root selector ignored** (selectorful PA in `istio-system` only matches root-ns workloads), **duplicate at scope** (two PAs at the same scope — oldest wins, the rest are silently dead), **all unset** (every matching PA is UNSET, so the install default applies), **port level without selector** (ns-wide PA carrying `portLevelMtls`, hitting every workload in the ns) |
| `failed to fetch` | namespace or cluster | PeerAuthentication fetch failed for a namespace or for the root namespace. Mesh build degrades instead of failing: membership still resolves from labels, mTLS verdicts become `unknown`, and this issue names the namespace and error (`buildMeshMembership.go:25`) |
| `node lockout` | — | **Declared but not produced.** No detector is wired for it today (`models/issues.go:11`) |

### Direction reasons carried on issues

`models/reachability.go` — every issue that names a blocked direction carries one
of: `permitted`, `explicit-deny` (a deny rule names the peer), `carved-out`
(covered by an allow but excluded by an `ipBlock.except`, so there is no deny
object to delete), `default-deny` (locked, zero allows), `locked-no-match`
(locked, allows exist but none for this peer), `no-opinion` (nothing governs).

Precedence when a direction is decided (`reachability.go:217`): explicit deny →
carve-out → allow → allows-elsewhere → deny-all marker → no opinion. Carve-outs
deliberately outrank allows, so a `0.0.0.0/0` allow can't win back a range its
own `except` list removed.

### Per-engine path verdict

`PoliciesCanReach` (`reachability.go:286`) evaluates both directions per engine —
egress rules of the source (its own bucket + its namespace node's bucket) and
ingress rules of the destination. A direction permits when it is `permitted` or
`no-opinion`. Both directions no-opinion → `not enforced`; both permitting →
`allow`; anything else → `deny`, and the fused verdict is a deny if any engine
denies. One subtlety that makes ordered first-match engines resolve correctly: a
policy's catch-all deny is skipped for a peer that the *same* policy allowed
specifically (`policiesAllowingPeerSpecifically`, `reachability.go:52`).
