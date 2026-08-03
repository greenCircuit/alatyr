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
  const aggregated = Math.max(...edges.map((e) => e.aggregatedFrom ?? 0));
  return (
    <>
      <EdgeReachabilityBanner result={reachability} loading={reachabilityLoading} />
      <div className={`${s.endpointRow} mb-3`}>
        <EndpointCard role="src" node={src} coverage={first.coverage} />
        <span className={s.endpointArrow} aria-hidden>→</span>
        <EndpointCard role="dst" node={dst} coverage={first.coverage} />
      </div>
      {/* Aggregated arrow: a cluster-wide policy resolved identically for every
          workload in the namespace, so N per-workload rules render as one line.
          Say it out loud — otherwise this reads as a namespace-scoped policy. */}
      {aggregated > 0 && (
        <div className={`${s.card} ${s.smallText} mb-3`}>
          <span className={s.dim}>
            Stands for {aggregated} per-workload rules — the policy selects every workload
            in this namespace, and all {aggregated} resolved this same verdict.
          </span>
        </div>
      )}
      {/* Findings on this pair — why an allow edge may still not carry traffic. */}
      <div className="mb-3">
        <PairIssues srcId={first.source} dstId={first.target} />
      </div>
      <div className={`${s.eyebrow} mb-2`}>
        Policies ({edges.length})
      </div>
      <div className={edges.length > 1 ? s.policyGrid : 'd-flex flex-column gap-2'}>
        {edges.map((p) => <PolicyRow key={p.id} p={p} />)}
      </div>
    </>
  );
}
