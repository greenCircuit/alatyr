// Edge-detail view: a single (src, dst) pair — the common case from a graph
// arrow click. Cross-engine reachability banner, both endpoint cards, then one
// PolicyRow per policy on the edge (grid when more than one).

import type { PolicyEdge, WorkloadNode, ReachabilityResult } from '../../../data/policies';
import s from '../DetailPanel.module.css';
import { EndpointCard, EdgeReachabilityBanner } from '../shared/edge-composites';
import { PolicyRow } from '../shared/rows';
import { PairIssues } from '../shared/issues';

export function EdgeView({ edges, nodes, reachability, reachabilityLoading }: {
  edges:               PolicyEdge[];
  nodes:               WorkloadNode[];
  reachability:        ReachabilityResult | null;
  reachabilityLoading: boolean;
}) {
  const first = edges[0];
  // For ns-level edges, first.source/target are the `ns-<name>` nodes —
  // resolved here so EndpointCard can render labels and ns labels just like a
  // workload edge. Without this the panel showed neither side.
  const src = nodes.find((n) => n.id === first.source);
  const dst = nodes.find((n) => n.id === first.target);
  return (
    <>
      <EdgeReachabilityBanner result={reachability} loading={reachabilityLoading} />
      <div className="d-flex flex-column gap-1 mb-3">
        <EndpointCard role="src" node={src} coverage={first.coverage} />
        <EndpointCard role="dst" node={dst} coverage={first.coverage} />
      </div>
      {/* Findings on this pair — why an allow edge may still not carry traffic. */}
      <div className="mb-3">
        <PairIssues srcId={first.source} dstId={first.target} />
      </div>
      <div className={`text-secondary mb-2 ${s.smallText}`}>
        Policies ({edges.length})
      </div>
      <div className={edges.length > 1 ? s.policyGrid : ''}>
        {edges.map((p) => <PolicyRow key={p.id} p={p} />)}
      </div>
    </>
  );
}
