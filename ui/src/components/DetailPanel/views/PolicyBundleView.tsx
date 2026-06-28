// Policy-bundle view: one policy spanning many (src, dst) pairs — the bundle
// produced by a Policies-table row click. Shared identity rendered once via
// PolicyHeader; AffectedPairsList lets the user narrow back to a single pair
// (which re-selects as a normal EdgeView).

import type { PolicyEdge, WorkloadNode } from '../../../data/policies';
import { PolicyHeader, AffectedPairsList } from '../shared/edge-composites';
import { groupEdgesByPair } from '../shared/groupEdgesByPair';

export function PolicyBundleView({ edges, nodes, onSelect }: {
  edges:    PolicyEdge[];
  nodes:    WorkloadNode[];
  onSelect: (edges: PolicyEdge[]) => void;
}) {
  const pairs = groupEdgesByPair(edges);
  return (
    <>
      <PolicyHeader edges={edges} />
      <AffectedPairsList pairs={pairs} nodes={nodes} onSelect={onSelect} />
    </>
  );
}
