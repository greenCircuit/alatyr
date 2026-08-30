# Demo data

Curated, realistic cluster embedded into the binary and served when
`DEMO_MODE=true`. Modelled on a mid-size SaaS: a public storefront and a
regulated payments tier on an Istio mesh, plus a plain-Kubernetes analytics
pipeline, a private-network connectivity tier, and a dedicated box collecting
every mesh footgun. Names are generic on purpose — this is what a prospect sees.
Shaped for legibility: each namespace is a self-contained story, and only two
cross-namespace arrows cross the whole default view (`web → payments` allow,
`analytics → payments` deny).

One file per namespace, self-contained (Namespace + workloads + all its
policies), so it reads like `kubectl apply -f` output. `DemoClient` walks the
whole tree, so file/apply order does not matter.

Apply against a live cluster: `kubectl apply -f test-data/`

## Namespaces

| File | Namespace | Role | Engines |
|---|---|---|---|
| `10-analytics.yaml` | `analytics` | Plain-Kubernetes tier — no mesh, clean pipeline DAG | k8s |
| `20-storefront.yaml` | `storefront` | Public app tier — the clean request-flow hero | k8s + istio |
| `30-payments.yaml` | `payments` | Regulated tier, both engines per workload | k8s + istio |
| `40-private-net.yaml` | `private-net` | Private/on-prem (RFC1918) connectivity tier | k8s |
| `50-mesh-conflicts.yaml` | `mesh-conflicts` | Deliberate cross-engine conflicts + mTLS misconfigs | k8s + istio |

`10-analytics.yaml` has no mesh (no ambient label, no Istio) — it proves the tool
reads a plain-NetworkPolicy shop, and its `ingest → enrich → warehouse` pipeline
is a clean self-contained DAG. `storefront/catalog` carries the "your cluster
isn't as locked down as you think" gap: no policy at all, renders `internet-full`.

## Badge coverage

Every status key the UI can render is exercised by at least one workload:

| Badge | Workload(s) |
|---|---|
| `internet-full` | `storefront/catalog` (no policy — the coverage gap) |
| `internet-ingress` | `storefront/web`, `storefront/checkout`, `analytics/ingest` |
| `internet-egress` | `storefront/web`, `payments/fraud-check`, `payments/payout-worker`, `private-net/telemetry-shipper` (with `ipBlock.except` carving out pod+svc CIDR) |
| `lan-ingress` | `private-net/ldap-proxy` |
| `lan-egress` | `private-net/backup-agent`, `private-net/metrics-relay` |
| `lan-full` | `payments/ledger-db`, `private-net/vpn-gw` |
| `air-gapped` | `payments/payments-api`, `payments/webhook-receiver`, `storefront/orders-api`, `storefront/sessions`, `analytics/warehouse` |
| `cross-namespace` | `storefront/web` (→ payments, the one rendered allow arrow); `storefront/orders-api` + `payments/payments-api` badge-only (egress target ns not in demo) |
| `ns-ingress-access` | `storefront/sessions`, `analytics/enrich` |
| `ns-egress-access` | `storefront/checkout`, `payments/webhook-receiver` |
| `ns-full-access` | `storefront/redis-cache` |
| `l7-applied` | `storefront/orders-api`, `payments/payments-api`, `payments/webhook-receiver` |

## Cross-engine issue fixtures

- **`cidr scope mismatch`** — `private-net/metrics-relay`: Calico GNP
  `egress-corp-metrics-lan` allows the whole `172.20.0.0/16`; paired k8s NP
  `metrics-relay-netpol` only names `172.20.5.42/32`. The pair against the
  broader aggregate node surfaces one `cidr scope mismatch` issue naming both
  masks.

Not covered by fixtures alone:

- **`api-server-egress`** — derives only when `apiServerCIDRs` is set in config
  (empty in `defaultConfig.yaml`); an egress rule to that CIDR otherwise
  classifies as `lan-egress`. Set a cluster CIDR in config to light it up.

## Node type coverage

One fixture per workload node type the UI can filter on:

| Node type | Fixture |
|---|---|
| `deployment` | most workloads (`storefront/web`, `analytics/ingest`, …) |
| `statefulset` | `payments/ledger-db` (postgres, stable identity) |
| `daemonset` | `private-net/telemetry-shipper` (one shipper per node) |
| `job` | `analytics/nightly-rollup` — standalone Job + its live pod |
| `pod` | `analytics/adhoc-query` — bare pod, no controller |
| `cidr` | ipBlock peers in `private-net`, `payments`, `storefront` |
| `namespace` | every ns node |
| `cronjob` | `analytics/retention-sweeper` (node from the CronJob object) |

Job and bare-pod nodes come from live Pods, not controller objects
(`AssembleNsIndex` reads the Jobs list for owner lookup only), so both fixtures
spell out a Pod with an explicit `metadata.uid` — DemoClient does not synthesize
pod UIDs, and node IDs key on that UID.

## Istio scenarios

- ALLOW L7 (methods/paths): `payments/payments-api`, `payments/webhook-receiver`; host-based on `storefront/orders-api`. All three use `from.source.namespaces` for the peer selector — the Istio engine only expands `src.Namespaces`, so `principals`-based sources render no edge.
- DENY (separate edge): `analytics` → `payments/payments-api` (cross-ns); `plaintext-client` → `dual-verdict-api` (in-ns, opposite the k8s allow)
- STRICT mTLS: namespace-local on `payments`; `mesh-conflicts/strict-server`
- ambient HBONE 15008: ingress rule on `payments/payments-api`
- mTLS + HBONE footguns (all in `mesh-conflicts`, one box): mismatch (`plaintext-client` DISABLE → `strict-server` STRICT), missing 15008 on that same edge, duplicate PeerAuth at scope (`strict-server`), all-UNSET PeerAuth (`dual-verdict-api`)

Note: keep Istio ALLOW policies off workloads whose k8s NetworkPolicy grants
internet-egress — the cross-engine intersection reads a same-namespace Istio
source as egress-transparent, which can mask/alter the effective egress badges.
Pair Istio ALLOW with a fully-scoped (air-gapped-shaped) k8s policy instead, as
`payments-api` and `orders-api` do.
