  1. Root-ns mesh-wide rules don't fan out. Global policies feed PolicyStatus only. buildRules
  selects workloads by the policy's own namespace, so a root-ns policy with selector app=foo
  won't produce rules in other namespaces. TODO: separate fan-out path.
  2. Coverage stub. Coverage() returns DefaultAllow always — no mesh-membership detection
  (sidecar injection check). Out-of-mesh workloads currently treated as gated by Istio, which
  is wrong. 
  3. expandRules quirks (pre-existing, untouched):
    - L7 rule emitted even with empty L7Match — appends an L3 ingress without a port
    - expandToOperation line 122: l7Match.Methods = append(l7Match.Hosts, method) — copies
  Hosts into Methods (typo, will corrupt L7 data)
    - Multiple rule.From blocks: only last one's matchWorkloads survive (overwrite, not
  accumulate)
    - Multiple rule.To blocks: same overwrite issue
  4. Deny bucket in EvaluationResult is populated but the graph builder (buildGraph.go:72) only
   consumes result.Allow. Deny edges won't render until BuildGraph reads result.Deny too.
  5. Copylocks warnings: istio proto types embed sync.Mutex. Strict fix =
  6. Static demo for portfolio (GitHub Pages, no backend). Goal: live demo link
  for LinkedIn — recruiters don't clone, they click. Steps:
    - Bake demo data: run backend in DEMO_MODE, capture /api/graph +
      /api/cluster-state responses, commit as ui/public/demo/graph.json +
      cluster-state.json
    - Frontend fallback: loadGraph/loadClusterState fetch the static JSON when
      VITE_DEMO=1 (or prod build) instead of /api
    - vite.config.ts: set base: '/<repo>/' (Pages serves from subpath)
    - .github/workflows/pages.yml: build ui/dist, upload-pages-artifact,
      deploy-pages
    - Repo Settings -> Pages -> Source: GitHub Actions
    Gate: static-JSON data path is the only hard step; rest is boilerplate.