import type { WorkloadNode, PolicyEdge, StatusKey } from '../data/policies';

export interface Graph {
  nodes: WorkloadNode[];
  edges: PolicyEdge[];
}

export interface ClusterState {
  availableNs: string[];
  statusKeys:  StatusKey[];
}

export function fetchGraph(namespaces?: string[]): Promise<Graph> {
  const params = namespaces?.length ? `?namespaces=${namespaces.join(',')}` : '';
  return fetch(`/api/graph${params}`).then((r) => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return r.json() as Promise<Graph>;
  });
}

export function fetchClusterState(): Promise<ClusterState> {
  return fetch('/api/cluster-state').then((r) => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return r.json() as Promise<ClusterState>;
  });
}
