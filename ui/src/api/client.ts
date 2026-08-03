import type { WorkloadNode, PolicyEdge, StatusKey, NodeDetail, ReachabilityResult, Issue, MeshMembership, ClusterMetrics } from '../data/policies';

export interface Graph {
  nodes: WorkloadNode[];
  edges: PolicyEdge[];
}

export interface ClusterState {
  availableNs:   string[];
  statusKeys:    StatusKey[];
  policySources: string[];
}

// getJson fetches and unwraps a JSON response. Error responses carry the
// backend's own message in {"error": "..."} — surface it verbatim, since it
// names the policy and rule that failed to decode. Falling back to the bare
// status code hides the only actionable detail the operator has.
async function getJson<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (response.ok) return response.json() as Promise<T>;

  const body = await response.text().catch(() => '');
  let message = body;
  try {
    message = (JSON.parse(body) as { error?: string }).error ?? body;
  } catch {
    // non-JSON error body (proxy/gateway) — use the raw text
  }
  throw new Error(message ? `HTTP ${response.status}: ${message}` : `HTTP ${response.status}`);
}

export function fetchGraph(namespaces?: string[]): Promise<Graph> {
  const params = namespaces?.length ? `?namespaces=${namespaces.join(',')}` : '';
  return getJson<Graph>(`/api/graph${params}`);
}

export function fetchClusterState(): Promise<ClusterState> {
  return getJson<ClusterState>('/api/cluster-state');
}

export function fetchNodeInfo(nodeId: string): Promise<NodeDetail> {
  const params = new URLSearchParams({ nodeId });
  return getJson<NodeDetail>(`/api/node-info?${params.toString()}`);
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
  return getJson<ReachabilityResult>(`/api/reachable?${params.toString()}`);
}

// fetchIssues runs the whole-cluster conflict scan. Recomputed server-side per
// request; the UI calls it on initial load and on refresh.
export function fetchIssues(): Promise<Issue[]> {
  return getJson<Issue[]>('/api/issues');
}

export interface MeshStatusResponse {
  nodes: Record<string, MeshMembership>;
}

// fetchMeshStatus pulls the cache-wide mesh membership map. Cheap read —
// backend serves cache.MeshMembership as-is, no compute.
export function fetchMeshStatus(): Promise<MeshStatusResponse> {
  return getJson<MeshStatusResponse>('/api/mesh-status');
}

// fetchClusterMetrics returns cluster totals + nested mesh posture counters.
// Denominators come from server cache sizes; unaffected by UI filter scope.
export function fetchClusterMetrics(): Promise<ClusterMetrics> {
  return getJson<ClusterMetrics>('/api/cluster-metrics');
}
