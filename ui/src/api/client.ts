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

export interface PolicyManifest {
  kind:      string;
  namespace: string;
  name:      string;
  yaml:      string;
}

// fetchManifest surfaces the backend's {error} string and HTTP status so the
// modal can frame a 404 as "stale graph" rather than a generic failure.
export async function fetchManifest(
  kind: string, namespace: string, name: string,
): Promise<PolicyManifest> {
  const params = new URLSearchParams({ kind, namespace, name });
  const response = await fetch(`/api/manifest?${params.toString()}`);
  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const body = await response.json();
      if (body?.error) message = body.error;
    } catch {
      // non-JSON error body — keep the status-code message
    }
    const error = new Error(message) as Error & { status?: number };
    error.status = response.status;
    throw error;
  }
  return response.json() as Promise<PolicyManifest>;
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
