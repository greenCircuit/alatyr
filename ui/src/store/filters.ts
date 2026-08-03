import type { WorkloadNode, PolicyEdge, MeshMembership, MeshScope } from '../data/policies';

// Flat OR-set of mesh membership + mtls verdict tags. Empty set = no mesh
// filter applied. Node passes when it matches ANY selected tag.
export type MeshFilterValue =
  | 'in-mesh'
  | 'out-of-mesh'
  | 'mtls-strict'
  | 'mtls-permissive'
  | 'mtls-disable'
  | 'mtls-unset';

export interface FilterState {
  allNodes:                WorkloadNode[];
  allEdges:                PolicyEdge[];
  selectedNamespaces:      Set<string>;
  selectedNodeTypes:       Set<string>;
  selectedPolicySources:   Set<string>;
  selectedActions:         Set<number>;
  selectedDirections:      Set<string>;
  selectedMeshFilters:     Set<MeshFilterValue>;
  meshStatus:              Record<string, MeshMembership>;
  showNamespaceEdges:      boolean;
  showConnectedNamespaces: boolean;
  searchQuery:             string;
}

export function nodeMatchesMeshFilter(
  node: WorkloadNode,
  meshStatus: Record<string, MeshMembership>,
  selected: Set<MeshFilterValue>,
): boolean {
  if (selected.size === 0) return true;
  const membership = meshStatus[node.id];
  const inMesh = membership?.inMesh ?? false;
  const verdict: MeshScope = membership?.mtls?.verdict ?? 'unset';
  for (const tag of selected) {
    if (tag === 'in-mesh'         && inMesh)                                 return true;
    if (tag === 'out-of-mesh'     && !inMesh)                                return true;
    if (tag === 'mtls-strict'     && inMesh && verdict === 'strict')         return true;
    if (tag === 'mtls-permissive' && inMesh && verdict === 'permissive')     return true;
    if (tag === 'mtls-disable'    && inMesh && verdict === 'disable')        return true;
    if (tag === 'mtls-unset'      && inMesh && verdict === 'unset')          return true;
  }
  return false;
}

export function filteredNodes(s: FilterState): WorkloadNode[] {
  const q = s.searchQuery.toLowerCase();

  const baseNodes = s.allNodes.filter((n) => {
    if (!(n.namespace === '' || s.selectedNamespaces.has(n.namespace))) return false;
    if (!s.selectedNodeTypes.has(n.type)) return false;
    if (q !== '' && !n.label.toLowerCase().includes(q) && !n.namespace.toLowerCase().includes(q)) return false;
    if (!nodeMatchesMeshFilter(n, s.meshStatus, s.selectedMeshFilters)) return false;
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
    (n) => reachableIds.has(n.id)
      && !baseIds.has(n.id)
      && s.selectedNodeTypes.has(n.type)
      && nodeMatchesMeshFilter(n, s.meshStatus, s.selectedMeshFilters)
  );

  return [...baseNodes, ...extraNodes];
}

// Endpoint-independent half of the edge filter: engine, action, direction.
// Split out so views that source edges by a different visibility rule (blanket
// rules carry only one endpoint, so they can't pass the src+dst check) still
// honour the same chip selections.
export function edgeMatchesPolicyFilters(s: FilterState, edge: PolicyEdge): boolean {
  if (!s.selectedPolicySources.has(edge.policySource)) return false;
  if (!s.selectedActions.has(edge.action ?? 0)) return false;
  // 'both' edges pass when either direction is selected.
  return edge.direction === 'both'
    ? s.selectedDirections.size !== 0
    : s.selectedDirections.has(edge.direction);
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
    if (!edgeMatchesPolicyFilters(s, e)) return false;
    if (e.level === 'namespace')
      return s.showNamespaceEdges && visibleIds.has(e.source) && visibleIds.has(e.target);
    return visibleIds.has(e.source) && visibleIds.has(e.target);
  });
}
