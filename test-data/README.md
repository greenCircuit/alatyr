# Demo data

Curated scenarios embedded into the binary and served when `DEMO_MODE=true`.
Each file maps to a feature the visualizer is designed to showcase. Unit tests
in `internal/policy/...` cover the lower-level rule-construction cases — this
tree is for screenshots and live demo, not regression coverage.

Apply against a live cluster: `kubectl apply -f test-data/`

## k8sEngine

| File | Workload | Showcases |
|---|---|---|
| `00-namespaces.yaml` | — | `test-netpol-a`, `test-netpol-b` ns scaffolding |
| `08-isolated.yaml` | `isolated-svc` | `⊘` air-gapped — both directions locked, no escape hatch |
| `09-internet-full.yaml` | `public-gateway` | `WAN⇆` — explicit `0.0.0.0/0` in + out |
| `10-internet-egress-only.yaml` | `outbound-worker` | `WAN↑` — egress to internet, ingress denied |
| `11-ns-full-access.yaml` | `inner-broker` | `NS⇆` — catch-all `podSelector: {}` opens whole ns |
| `13-empty-podselector.yaml` | `flux-receiver` | `podSelector: {}` ingress footgun (flux pattern) |
| `90-observability-namespace.yaml` + `91-observability-pods.yaml` + `92-observability-netpols.yaml` | real `observability` dump | Headline: partial NetworkPolicy coverage in production — unprotected pods render as `WAN⇆` |

## istioEngine

| File | Workload | Showcases |
|---|---|---|
| `00-namespaces.yaml` | — | `istio-mesh-a`, `istio-mesh-b` ns scaffolding (`istio-injection: enabled`) |
| `02-deny-from-ns.yaml` | `authz-deny-target` ← `authz-deny-attacker` | AuthorizationPolicy `DENY` — visualized separately from ALLOW |
| `06-l7-paths-methods.yaml` | `l7-api-server` | L7 ALLOW: HTTP methods + paths (`GET`/`POST` on `/api/*`) |
| `07-l7-hosts.yaml` | `l7-host-backend` | L7 ALLOW gated on Host header |
| `09-ports-and-l7.yaml` | `mixed-server` | Combined L4 ports + L7 paths/methods |
| `12-port-policies.yaml` | `port-client` → `port-server` | NP egress (2 app ports) + NP ingress (app port + ztunnel HBONE 15008) in ambient ns |
