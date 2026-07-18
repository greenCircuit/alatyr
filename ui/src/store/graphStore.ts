import { create } from 'zustand';
import { fetchGraph, fetchClusterState, fetchNodeInfo, fetchReachability, fetchIssues, fetchMeshStatus } from '../api/client';
import type { WorkloadNode, PolicyEdge, StatusKey, NodeDetail, ReachabilityResult, Issue, IssueType, MeshMembership } from '../data/policies';
import { filteredNodes as _filteredNodes, filteredEdges as _filteredEdges, type MeshFilterValue } from './filters';

export type TablesTab = 'workloads' | 'policies' | 'issues';
export type { MeshFilterValue };

interface GraphState {
  allNodes:               WorkloadNode[];
  allEdges:               PolicyEdge[];
  availableNamespaces:    string[];
  availableStatusKeys:    StatusKey[];
  availablePolicySources: string[];
  loading:                boolean;
  error:                  string | null;

  selectedNamespaces:      Set<string>;
  selectedNodeTypes:       Set<string>;
  selectedStatuses:        Set<StatusKey>;
  selectedPolicySources:   Set<string>;
  selectedActions:         Set<number>; // 0 = allow, 1 = deny
  selectedDirections:      Set<string>; // 'ingress' | 'egress'
  selectedMeshFilters:     Set<MeshFilterValue>;
  meshStatus:              Record<string, MeshMembership>;
  showNamespaceEdges:      boolean;
  showConnectedNamespaces: boolean;
  aggregateByNamespace:    boolean;
  showEngineIcons:         boolean;
  showMeshOverlay:         boolean;
  selectedNode:            WorkloadNode | null;
  selectedEdges:           PolicyEdge[];
  nodeInfo:                NodeDetail | null;
  nodeInfoLoading:         boolean;
  searchQuery:             string;
  layoutAlgorithm:         string;
  view:                    'graph' | 'tables' | 'status';
  // Lifted out of TablesView so other views (cluster status page) can deep-link
  // into a specific tab before switching the view.
  tablesTab:               TablesTab;

  reachabilitySource:      WorkloadNode | null;
  reachability:            ReachabilityResult | null;
  reachabilityLoading:     boolean;
  reachabilityTarget:      WorkloadNode | null;

  // Reverse-direction verdict (dst → src), auto-fetched alongside the forward
  // fetch so the headline can flag bidirectional vs one-way at a glance.
  // Error is tracked separately so a failed reverse doesn't silently render as
  // "no bidirectional info" — the headline can flag "reverse unavailable".
  reachabilityReverse:        ReachabilityResult | null;
  reachabilityReverseLoading: boolean;
  reachabilityReverseError:   boolean;

  edgeReachability:        ReachabilityResult | null;
  edgeReachabilityLoading: boolean;

  issues:            Issue[];
  issuesLoading:     boolean;
  issuesDrawerOpen:  boolean;
  // Empty set = "no filter, all rows visible". Non-empty = only rows carrying
  // an issue of a selected type stay in the tables view.
  selectedIssueTypes: Set<IssueType>;

  loadClusterState:            () => Promise<void>;
  loadGraph:                   () => Promise<void>;
  loadIssues:                  () => Promise<void>;
  loadMeshStatus:              () => Promise<void>;
  toggleMeshFilter:            (value: MeshFilterValue) => void;
  setIssuesDrawerOpen:         (open: boolean) => void;
  toggleIssueType:             (type: IssueType) => void;
  loadNodeInfo:                (nodeId: string, namespace: string) => Promise<void>;
  toggleNamespace:             (ns: string) => void;
  // Drill-down: replace the namespace filter with a single namespace (cluster
  // status page row click), as opposed to toggleNamespace's add/remove.
  selectNamespaceOnly:         (ns: string) => void;
  toggleNodeType:              (type: string) => void;
  toggleStatus:                (key: StatusKey) => void;
  togglePolicySource:          (src: string) => void;
  toggleAction:                (action: number) => void;
  toggleDirection:             (direction: string) => void;
  toggleNamespaceEdges:        () => void;
  toggleConnectedNamespaces:   () => void;
  toggleAggregateByNamespace:  () => void;
  toggleEngineIcons:           () => void;
  toggleMeshOverlay:           () => void;
  setSelectedNode:             (node: WorkloadNode | null) => void;
  setSelectedEdges:            (edges: PolicyEdge[]) => void;
  setSearchQuery:              (q: string) => void;
  setLayoutAlgorithm:          (algo: string) => void;
  setView:                     (view: 'graph' | 'tables' | 'status') => void;
  setTablesTab:                (tab: TablesTab) => void;

  pinReachabilitySource:       (node: WorkloadNode) => void;
  showReachability:            (src: WorkloadNode, dst: WorkloadNode) => void;
  clearReachability:           () => void;
  // Select every edge produced by a specific policy so the DetailPanel opens
  // the bundle/edge view for it. Called from IssuesPopover → "View policy".
  // Returns false when the policy has no edges (callers fall back to manifest).
  selectPolicyByRef:           (source: string, namespace: string, name: string) => boolean;

  filteredNodes: () => WorkloadNode[];
  filteredEdges: () => PolicyEdge[];
}

const ALL_TYPES = ['service', 'deployment', 'headless', 'external', 'cronjob'];
const ALL_ACTIONS = [0, 1];
const ALL_DIRECTIONS = ['ingress', 'egress'];

export const useGraphStore = create<GraphState>((set, get) => ({
  allNodes:               [],
  allEdges:               [],
  availableNamespaces:    [],
  availableStatusKeys:    [],
  availablePolicySources: [],
  loading:                false,
  error:                  null,

  selectedNamespaces:      new Set<string>(),
  selectedNodeTypes:       new Set(ALL_TYPES),
  selectedStatuses:        new Set<StatusKey>(),
  selectedPolicySources:   new Set<string>(),
  selectedActions:         new Set<number>(ALL_ACTIONS),
  selectedDirections:      new Set<string>(ALL_DIRECTIONS),
  selectedMeshFilters:     new Set<MeshFilterValue>(),
  meshStatus:              {},
  showNamespaceEdges:      true,
  showConnectedNamespaces: false,
  aggregateByNamespace:    false,
  showEngineIcons:         false,
  showMeshOverlay:         false,
  selectedNode:            null,
  selectedEdges:           [],
  nodeInfo:                null,
  nodeInfoLoading:         false,
  searchQuery:             '',
  layoutAlgorithm:         'dagre',
  view:                    'graph',
  tablesTab:               'workloads',

  reachabilitySource:      null,
  reachability:            null,
  reachabilityLoading:     false,
  reachabilityTarget:      null,

  reachabilityReverse:        null,
  reachabilityReverseLoading: false,
  reachabilityReverseError:   false,

  edgeReachability:        null,
  edgeReachabilityLoading: false,

  issues:             [],
  issuesLoading:      false,
  issuesDrawerOpen:   false,
  selectedIssueTypes: new Set<IssueType>(),

  loadClusterState: async () => {
    try {
      const state = await fetchClusterState();
      // Sort namespaces + policy sources alphabetically so the filter
      // dropdown and any downstream consumer get a stable, scannable order.
      const sortedNs      = [...state.availableNs].sort();
      const sortedSources = [...state.policySources].sort();
      set({
        availableNamespaces:    sortedNs,
        availableStatusKeys:    state.statusKeys,
        availablePolicySources: sortedSources,
        selectedNamespaces:     new Set(sortedNs),
        selectedPolicySources:  new Set(sortedSources),
      });
    } catch (e) {
      set({ error: String(e) });
    }
  },

  loadGraph: async () => {
    set({ loading: true, error: null });
    try {
      const data = await fetchGraph();
      set({
        allNodes: data.nodes,
        allEdges: data.edges,
        loading:  false,
      });
      // Re-fetch detail for the currently-selected node so the panel reflects
      // the refreshed cache (mesh state, issues, rules).
      const selected = get().selectedNode;
      if (selected?.namespace) {
        get().loadNodeInfo(selected.id, selected.namespace);
      }
      // Whole-cluster conflict scan runs against the now-fresh server cache.
      // Chained here so both initial load and the refresh button trigger it,
      // after the graph fetch resolves — never against stale data.
      get().loadIssues();
      // Mesh membership map hydrates alongside graph — filter panel + node
      // decorations depend on it being present shortly after the graph loads.
      get().loadMeshStatus();
    } catch (e) {
      set({ loading: false, error: String(e) });
    }
  },

  loadMeshStatus: async () => {
    try {
      const data = await fetchMeshStatus();
      set({ meshStatus: data.nodes ?? {} });
    } catch (e) {
      set({ error: String(e) });
    }
  },

  toggleMeshFilter: (value) =>
    set((state) => {
      const next = new Set(state.selectedMeshFilters);
      next.has(value) ? next.delete(value) : next.add(value);
      return { selectedMeshFilters: next, selectedNode: null, selectedEdges: [] };
    }),

  loadIssues: async () => {
    set({ issuesLoading: true });
    try {
      const data = await fetchIssues();
      set({ issues: data ?? [], issuesLoading: false });
    } catch (e) {
      set({ issuesLoading: false, error: String(e) });
    }
  },
  setIssuesDrawerOpen: (open) => set({ issuesDrawerOpen: open }),

  toggleIssueType: (type) =>
    set((state) => {
      const next = new Set(state.selectedIssueTypes);
      next.has(type) ? next.delete(type) : next.add(type);
      return { selectedIssueTypes: next };
    }),

  toggleNamespace: (ns) =>
    set((state) => {
      const next = new Set(state.selectedNamespaces);
      next.has(ns) ? next.delete(ns) : next.add(ns);
      return { selectedNamespaces: next, selectedNode: null, selectedEdges: [] };
    }),

  selectNamespaceOnly: (ns) =>
    set({ selectedNamespaces: new Set([ns]), selectedNode: null, selectedEdges: [] }),

  toggleNodeType: (type) =>
    set((state) => {
      const next = new Set(state.selectedNodeTypes);
      next.has(type) ? next.delete(type) : next.add(type);
      return { selectedNodeTypes: next, selectedNode: null, selectedEdges: [] };
    }),

  toggleStatus: (key) =>
    set((state) => {
      const next = new Set(state.selectedStatuses);
      next.has(key) ? next.delete(key) : next.add(key);
      return { selectedStatuses: next, selectedNode: null, selectedEdges: [] };
    }),

  togglePolicySource: (src) =>
    set((state) => {
      const next = new Set(state.selectedPolicySources);
      next.has(src) ? next.delete(src) : next.add(src);
      return { selectedPolicySources: next, selectedNode: null, selectedEdges: [] };
    }),

  toggleAction: (action) =>
    set((state) => {
      const next = new Set(state.selectedActions);
      next.has(action) ? next.delete(action) : next.add(action);
      return { selectedActions: next, selectedNode: null, selectedEdges: [] };
    }),

  toggleDirection: (direction) =>
    set((state) => {
      const next = new Set(state.selectedDirections);
      next.has(direction) ? next.delete(direction) : next.add(direction);
      return { selectedDirections: next, selectedNode: null, selectedEdges: [] };
    }),

  toggleNamespaceEdges: () =>
    set((s) => ({ showNamespaceEdges: !s.showNamespaceEdges })),

  toggleConnectedNamespaces: () =>
    set((s) => ({ showConnectedNamespaces: !s.showConnectedNamespaces })),

  toggleAggregateByNamespace: () =>
    set((s) => ({ aggregateByNamespace: !s.aggregateByNamespace })),

  toggleEngineIcons: () =>
    set((s) => ({ showEngineIcons: !s.showEngineIcons })),

  toggleMeshOverlay: () =>
    set((s) => ({ showMeshOverlay: !s.showMeshOverlay })),

  setSelectedNode: (node) => {
    const src = get().reachabilitySource;
    // If a reachability source is pinned and the next click is on a different
    // node, treat the click as "check reach from src → this node" and fire
    // /api/reachable instead of overwriting the selection.
    if (src && node && node.id !== src.id) {
      set({
        reachabilityTarget: node,
        reachabilityLoading: true, reachability: null,
        reachabilityReverseLoading: true, reachabilityReverse: null, reachabilityReverseError: false,
      });
      fetchReachability(src.id, src.namespace || src.id, node.id, node.namespace || node.id)
        .then((result) => {
          // bail if source was unpinned or target changed mid-fetch
          if (get().reachabilitySource?.id !== src.id || get().reachabilityTarget?.id !== node.id) return;
          set({ reachability: result, reachabilityLoading: false });
        })
        .catch(() => set({ reachabilityLoading: false }));
      fetchReachability(node.id, node.namespace || node.id, src.id, src.namespace || src.id)
        .then((result) => {
          if (get().reachabilitySource?.id !== src.id || get().reachabilityTarget?.id !== node.id) return;
          set({ reachabilityReverse: result, reachabilityReverseLoading: false });
        })
        .catch(() => {
          if (get().reachabilitySource?.id !== src.id || get().reachabilityTarget?.id !== node.id) return;
          set({ reachabilityReverseLoading: false, reachabilityReverseError: true });
        });
      return;
    }
    set({ selectedNode: node, selectedEdges: [], nodeInfo: null });
    if (node && node.namespace) {
      get().loadNodeInfo(node.id, node.namespace);
    }
  },
  setSelectedEdges: (edges) => {
    set({ selectedEdges: edges, selectedNode: null, nodeInfo: null, edgeReachability: null });
    // Edges in a bundle share src/dst, so one /api/reachable call covers them
    // all. Skip namespace-scoped edges — the matcher expects workload IDs and
    // a "ns → ns" verdict isn't a meaningful answer for the user.
    if (edges.length === 0) return;
    const edge = edges[0];
    if (edge.level === 'namespace') return;
    const nodes = get().allNodes;
    const src = nodes.find((n) => n.id === edge.source);
    const dst = nodes.find((n) => n.id === edge.target);
    if (!src || !dst) return;
    set({ edgeReachabilityLoading: true });
    fetchReachability(src.id, src.namespace || src.id, dst.id, dst.namespace || dst.id)
      .then((result) => {
        // Bail if user moved on mid-fetch — selectedEdges replaced or cleared.
        const current = get().selectedEdges;
        if (current.length === 0 || current[0].id !== edge.id) return;
        set({ edgeReachability: result, edgeReachabilityLoading: false });
      })
      .catch(() => set({ edgeReachabilityLoading: false }));
  },

  pinReachabilitySource: (node) =>
    set({ reachabilitySource: node, reachability: null, reachabilityTarget: null }),

  // Open the reachability panel directly for a known src→dst pair (e.g. an
  // issue row). Pins source + target and fetches, reusing ReachabilityView.
  showReachability: (src, dst) => {
    set({
      selectedNode: null, selectedEdges: [], nodeInfo: null,
      reachabilitySource: src, reachabilityTarget: dst,
      reachability: null, reachabilityLoading: true,
      reachabilityReverse: null, reachabilityReverseLoading: true, reachabilityReverseError: false,
    });
    fetchReachability(src.id, src.namespace || src.id, dst.id, dst.namespace || dst.id)
      .then((result) => {
        // bail if the user moved on mid-fetch
        if (get().reachabilitySource?.id !== src.id || get().reachabilityTarget?.id !== dst.id) return;
        set({ reachability: result, reachabilityLoading: false });
      })
      .catch(() => set({ reachabilityLoading: false }));
    fetchReachability(dst.id, dst.namespace || dst.id, src.id, src.namespace || src.id)
      .then((result) => {
        if (get().reachabilitySource?.id !== src.id || get().reachabilityTarget?.id !== dst.id) return;
        set({ reachabilityReverse: result, reachabilityReverseLoading: false });
      })
      .catch(() => {
        if (get().reachabilitySource?.id !== src.id || get().reachabilityTarget?.id !== dst.id) return;
        set({ reachabilityReverseLoading: false, reachabilityReverseError: true });
      });
  },

  clearReachability: () =>
    set({
      reachabilitySource: null, reachability: null, reachabilityTarget: null, reachabilityLoading: false,
      reachabilityReverse: null, reachabilityReverseLoading: false, reachabilityReverseError: false,
    }),

  selectPolicyByRef: (source, namespace, name) => {
    const matching = get().allEdges.filter((e) =>
      e.policySource === source && e.namespace === namespace && e.policyName === name);
    // No edge to select (default-deny / lockout policies emit none). Report it
    // so callers can fall back instead of silently no-oping the click.
    if (matching.length === 0) return false;
    // Reuse the same selection path a table-row click uses so DetailPanel
    // resolves the right view (EdgeView for single-pair, PolicyBundleView for
    // multi-pair) without any duplicated dispatch logic.
    get().setSelectedEdges(matching);
    return true;
  },

  loadNodeInfo: async (nodeId, namespace) => {
    set({ nodeInfoLoading: true });
    try {
      const data = await fetchNodeInfo(nodeId, namespace);
      // bail if user moved on before fetch completed
      if (get().selectedNode?.id !== nodeId) return;
      set({ nodeInfo: data, nodeInfoLoading: false });
    } catch {
      set({ nodeInfoLoading: false });
    }
  },
  setSearchQuery:       (q)     => set({ searchQuery: q }),
  setLayoutAlgorithm:   (algo)  => set({ layoutAlgorithm: algo }),
  setView:              (view)  => set({ view }),
  setTablesTab:         (tab)   => set({ tablesTab: tab }),

  filteredNodes: () => _filteredNodes(get()),
  filteredEdges: () => _filteredEdges(get()),
}));
