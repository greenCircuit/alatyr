# Network Policy Test Data

Two namespaces: `test-netpol-a`, `test-netpol-b`

Apply: `kubectl apply -f test-data/`  
Verify: `curl "http://localhost:8080/api/graph?namespaces=test-netpol-a,test-netpol-b"`

## Test Cases

| File | Policy | Expected edge | What it detects |
|---|---|---|---|
| `01-pod-ingress.yaml` | `pod-ingress` | `pod-ingress-src → pod-ingress-dst` (ingress, workload) | pod-only ingress selector |
| `02-pod-egress.yaml` | `pod-egress` | `pod-egress-src → pod-egress-dst` (egress, workload) | pod-only egress selector |
| `03-ns-ingress.yaml` | `ns-ingress` | `test-netpol-b namespace node → ns-ingress-dst` (ingress, namespace) | namespace-only selector as ingress source — edge must target the namespace node, not individual pods |
| `04-cross-ns.yaml` | `cross-ns` | `cross-ns-src → cross-ns-dst` (ingress, workload) | both namespaceSelector+podSelector — edge must only appear for `cross-ns-src` in `test-netpol-a`, not for same-named pods in other namespaces |
| `05-ip-block.yaml` | `ip-block` | `ip-block-src → 1.1.1.1/32` (egress, workload) | IP block egress — target is the CIDR string, not a node |
| `06-pod-to-ns.yaml` | `pod-to-ns` | `pod-to-ns-src → test-netpol-b namespace node` (egress, namespace) | namespace-only selector as egress destination — edge must point at the namespace node, not individual pods in ns-b |
| `07-ns-to-ns.yaml` | `ns-to-ns` | `test-netpol-a namespace node → every pod in test-netpol-b` (ingress, namespace) | empty podSelector + namespace-only source — source must be the namespace node, not individual pods from ns-a |
