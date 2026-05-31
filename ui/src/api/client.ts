import type { WorkloadNode, PolicyEdge, StatusKey, NodeInfo, ReachabilityResult } from '../data/policies';

export interface Graph {
  nodes: WorkloadNode[];
  edges: PolicyEdge[];
}

export interface ClusterState {
  availableNs:   string[];
  statusKeys:    StatusKey[];
  policySources: string[];
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

export function fetchNodeInfo(nodeId: string, namespace: string): Promise<NodeInfo> {
  const params = new URLSearchParams({ nodeId, namespace });
  return fetch(`/api/node-info?${params.toString()}`).then((r) => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return r.json() as Promise<NodeInfo>;
  });
}

export function fetchReachability(
  srcId: string, srcNs: string, dstId: string, dstNs: string,
): Promise<ReachabilityResult> {
  const params = new URLSearchParams({ srcId, srcNs, dstId, dstNs });
  return fetch(`/api/reachable?${params.toString()}`).then((r) => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return r.json() as Promise<ReachabilityResult>;
  });
}
