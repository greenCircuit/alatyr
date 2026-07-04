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
| `internet-egress` | `storefront/web`, `payments/fraud-check`, `payments/payout-worker` |
| `lan-ingress` | `private-net/ldap-proxy` |
| `lan-egress` | `private-net/backup-agent` |
| `lan-full` | `payments/ledger-db`, `private-net/vpn-gw` |
| `air-gapped` | `payments/payments-api`, `payments/webhook-receiver`, `storefront/orders-api`, `storefront/sessions`, `analytics/warehouse` |
| `cross-namespace` | `storefront/web` (→ payments, the one rendered allow arrow); `storefront/orders-api` + `payments/payments-api` badge-only (egress target ns not in demo) |
| `ns-ingress-access` | `storefront/sessions`, `analytics/enrich` |
| `ns-egress-access` | `storefront/checkout`, `payments/webhook-receiver` |
| `ns-full-access` | `storefront/redis-cache` |
| `l7-applied` | `storefront/orders-api`, `payments/payments-api` |

Not covered by fixtures alone:

- **`api-server-egress`** — derives only when `apiServerCIDRs` is set in config
  (empty in `defaultConfig.yaml`); an egress rule to that CIDR otherwise
  classifies as `lan-egress`. Set a cluster CIDR in config to light it up.
- **CronJob node type** — `DemoClient.parseFile` does not parse `CronJob`, so no
  fixture exercises the cronjob node type. Add a `CronJob` case to
  `internal/k8s/demo.go` to light it up.

## Istio scenarios

- ALLOW L7 (methods/paths): `payments/payments-api`; host-based on `storefront/orders-api`
- DENY (separate edge): `analytics` → `payments/payments-api` (cross-ns); `plaintext-client` → `dual-verdict-api` (in-ns, opposite the k8s allow)
- STRICT mTLS: namespace-local on `payments`; `mesh-conflicts/strict-server`
- ambient HBONE 15008: ingress rule on `payments/payments-api`
- mTLS + HBONE footguns (all in `mesh-conflicts`, one box): mismatch (`plaintext-client` DISABLE → `strict-server` STRICT), missing 15008 on that same edge, duplicate PeerAuth at scope (`strict-server`), all-UNSET PeerAuth (`dual-verdict-api`)

Note: keep Istio ALLOW policies off workloads whose k8s NetworkPolicy grants
internet-egress — the cross-engine intersection reads a same-namespace Istio
source as egress-transparent, which can mask/alter the effective egress badges.
Pair Istio ALLOW with a fully-scoped (air-gapped-shaped) k8s policy instead, as
`payments-api` and `orders-api` do.
