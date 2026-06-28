// Pure helper: group selected edges by (source, target). Bundles produced by
// the Policies table span pairs; graph-click bundles always share a single
// pair. Kept in its own module (with the adjacent test) so the edge-composite
// component file exports components only.

import type { PolicyEdge } from '../../../data/policies';

export interface PairGroup {
  key: string;
  src: PolicyEdge['source'];
  dst: PolicyEdge['target'];
  edges: PolicyEdge[];
}

export function groupEdgesByPair(edges: PolicyEdge[]): PairGroup[] {
  const map = new Map<string, PairGroup>();
  for (const e of edges) {
    const key = `${e.source}|${e.target}`;
    let g = map.get(key);
    if (!g) {
      g = { key, src: e.source, dst: e.target, edges: [] };
      map.set(key, g);
    }
    g.edges.push(e);
  }
  return [...map.values()];
}
