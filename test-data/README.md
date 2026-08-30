# Test data

Two kinds of fixture clusters, both parsed by `k8s.DemoClient` (same code path,
same YAML dialect — `kubectl apply -f` output, one or more multi-doc files):

| Dir | What | How to load |
|---|---|---|
| `demo/` | The full curated demo cluster — 6 namespaces, all engines, the prospect-facing story | `DEMO_MODE=true go run main.go` (embedded in the binary) |
| `scenarios/<name>/` | Small single-purpose clusters, 1–3 namespaces each, for e2e tests | `go run main.go -f test-data/scenarios/<name>` |

`DemoClient` walks the directory tree it is handed, so **point `-f` at exactly
one scenario dir** — `-f test-data/scenarios` merges every scenario into one
cluster. Only `demo/` is embedded (`embed.go`); scenarios load from disk.

## Scenarios

| Dir | Namespaces | Exercises |
|---|---|---|
| `01-k8s-baseline` | `front`, `back` | k8s NetworkPolicy only: internet/lan ipBlock badges, air-gapped, ns-scoped ingress, one cross-ns arrow, one no-policy coverage gap (`front/scratch`) |
| `02-istio-l7` | `svc`, `clients`, `legacy`, `istio-system` | Istio ALLOW with L7 (methods/paths, hosts), DENY as a separate edge, root-namespace policy, k8s+istio status intersection |
| `03-mtls-matrix` | `mesh`, `outside` | Every mTLS verdict (strict / permissive / disable / unset), out-of-mesh workload, duplicate PeerAuth at one scope, DISABLE→STRICT mismatch edge |
| `04-calico-globals` | `edge`, `infra` | Calico GNP tier/order precedence, ns-selector fan-out, `cidr scope mismatch` against a narrower k8s NP, calico-denies-what-k8s-allows |
| `05-workload-kinds` | `kinds` | One workload of every node type — deployment, statefulset, daemonset, cronjob, job, bare pod, CIDR peer, ns node — plus a Succeeded pod that must not render |

### Fixture rules

- Job and bare-pod nodes come from live **Pods**, not from the controller
  object (`AssembleNsIndex` reads the Jobs list for owner lookup only). Those
  fixtures spell out a Pod with an explicit `metadata.uid` and
  `status.phase: Running` — `DemoClient` does not synthesize pod UIDs, and node
  IDs key on that UID.
- Deployment / StatefulSet / DaemonSet / CronJob nodes come from the controller
  object; no pod fixture needed. `DemoClient` synthesizes their UIDs.
- Pods in `Succeeded` / `Failed` phase are dropped by the graph layer — keep one
  around when a scenario needs to prove it.
- Keep an Istio ALLOW off any workload whose k8s NP grants internet-egress: a
  same-namespace Istio source reads as egress-transparent in the intersection
  and can mask the egress badges. Pair Istio ALLOW with an air-gapped-shaped k8s
  policy instead.

## Adding a scenario

1. `mkdir test-data/scenarios/06-<name>` and write `cluster.yaml` (multi-doc,
   namespaces first, policies after the workloads they select).
2. Header comment: the arrow diagram and the expected badge per workload. That
   comment is the spec the e2e assertions are written against.
3. Add a row to the table above.
