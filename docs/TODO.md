● Backend-SRE take, ranked by what actually pages:
                                                                    
  1. Cross-engine contradiction banner (3am bug) — lying graph risk.
   Istio ALLOW edge src→dst:8080, k8s NetPol silently drops it.     
  Already client-side in allEdges: scan other engines for the same  
  src/dst/port; render "k8s blocks :8080" banner on ALLOW edges     
  contradicted elsewhere (and inverse for overridden DENYs). Highest
   value, zero backend work.                                   
                                                                    
  2. Selector-match identity (gap) — current Labels block shows     
  every label on the workload (policy.tsx:128). At 3am you need the 
  labels this policy's selector actually matched, so you know what
  breaks if someone edits a label. Not in PolicyEdge (graph.go:8) — 
  backend gap, flag it, don't fake it client-side.                  
                                                                    
       
   
  4. Inline reachability verdict for this pair — /api/reachable     
  already returns EngineVerdict + mesh state. User clicking an arrow
   is asking "can src talk to dst" — same question. Avoids the "edge
   says allow, mesh is STRICT, src has no cert" trap.               
   
  5. Cheap win: badge Istio root-ns policies "mesh-wide" — when     
  policySource==='istio' and p.namespace is the Istio root ns,
  operators chasing a deny in app-ns miss that the policy applies
  everywhere.                                                       
   
  Skip: chip styling, more L7 polish.                               
                                                            
  Which to tackle first? My read: #1 (highest pager value, FE-only)
  → #3 (free, makes room for #1) → #4 (FE-only, reuses existing
  endpoint) → #5 (one-liner). #2 needs backend selector exposure
  first — pair-programming territory.                               
   