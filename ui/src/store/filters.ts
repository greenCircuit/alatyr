import type { WorkloadNode, PolicyEdge } from '../data/policies';

export interface FilterState {
  allNodes:                WorkloadNode[];
  allEdges:                PolicyEdge[];
  selectedNamespaces:      Set<string>;
  selectedNodeTypes:       Set<string>;
  showNamespaceEdges:      boolean;
  showConnectedNamespaces: boolean;
  searchQuery:             string;
}

export function filteredNodes(s: FilterState): WorkloadNode[] {
  const q = s.searchQuery.toLowerCase();

  const baseNodes = s.allNodes.filter((n) => {
    if (!(n.namespace === '' || s.selectedNamespaces.has(n.namespace))) return false;
    if (!s.selectedNodeTypes.has(n.type)) return false;
    if (q !== '' && !n.label.toLowerCase().includes(q) && !n.namespace.toLowerCase().includes(q)) return false;
    return true;
  });

  if (!s.showConnectedNamespaces) return baseNodes;

  const baseIds = new Set(baseNodes.map((n) => n.id));
  const reachableIds = new Set<string>();
  for (const e of s.allEdges) {
    if (e.level !== 'workload') continue;
    if (baseIds.has(e.source)) reachableIds.add(e.target);
    if (baseIds.has(e.target)) reachableIds.add(e.source);
  }

  const extraNodes = s.allNodes.filter(
    (n) => reachableIds.has(n.id) && !baseIds.has(n.id) && s.selectedNodeTypes.has(n.type)
  );

  return [...baseNodes, ...extraNodes];
}

export function filteredEdges(s: FilterState): PolicyEdge[] {
  const visibleWorkloadIds = new Set(filteredNodes(s).map((n) => n.id));
  const occupiedNS = new Set(
    s.allNodes.filter((n) => visibleWorkloadIds.has(n.id) && n.namespace).map((n) => n.namespace)
  );
  const nsSource = s.showConnectedNamespaces
    ? Array.from(occupiedNS)
    : Array.from(s.selectedNamespaces).filter((ns) => occupiedNS.has(ns));
  const visibleIds = new Set([
    ...visibleWorkloadIds,
    ...nsSource.map((ns) => `ns-${ns}`),
  ]);
  return s.allEdges.filter((e) => {
    if (e.level === 'namespace')
      return s.showNamespaceEdges && visibleIds.has(e.source) && visibleIds.has(e.target);
    return visibleIds.has(e.source) && visibleIds.has(e.target);
  });
}
