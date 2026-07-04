import { create } from 'zustand';
import { fetchGraph, fetchClusterState, fetchNodeInfo, fetchReachability } from '../api/client';
import type { WorkloadNode, PolicyEdge, StatusKey, NodeDetail, ReachabilityResult } from '../data/policies';
import { filteredNodes as _filteredNodes, filteredEdges as _filteredEdges } from './filters';

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
  showNamespaceEdges:      boolean;
  showConnectedNamespaces: boolean;
  aggregateByNamespace:    boolean;
  showEngineIcons:         boolean;
  selectedNode:            WorkloadNode | null;
  selectedEdges:           PolicyEdge[];
  nodeInfo:                NodeDetail | null;
  nodeInfoLoading:         boolean;
  searchQuery:             string;
  layoutAlgorithm:         string;
  view:                    'graph' | 'tables';

  reachabilitySource:      WorkloadNode | null;
  reachability:            ReachabilityResult | null;
  reachabilityLoading:     boolean;
  reachabilityTarget:      WorkloadNode | null;

  edgeReachability:        ReachabilityResult | null;
  edgeReachabilityLoading: boolean;

  loadClusterState:            () => Promise<void>;
  loadGraph:                   () => Promise<void>;
  loadNodeInfo:                (nodeId: string, namespace: string) => Promise<void>;
  toggleNamespace:             (ns: string) => void;
  toggleNodeType:              (type: string) => void;
  toggleStatus:                (key: StatusKey) => void;
  togglePolicySource:          (src: string) => void;
  toggleAction:                (action: number) => void;
  toggleDirection:             (direction: string) => void;
  toggleNamespaceEdges:        () => void;
  toggleConnectedNamespaces:   () => void;
  toggleAggregateByNamespace:  () => void;
  toggleEngineIcons:           () => void;
  setSelectedNode:             (node: WorkloadNode | null) => void;
  setSelectedEdges:            (edges: PolicyEdge[]) => void;
  setSearchQuery:              (q: string) => void;
  setLayoutAlgorithm:          (algo: string) => void;
  setView:                     (view: 'graph' | 'tables') => void;

  pinReachabilitySource:       (node: WorkloadNode) => void;
  clearReachability:           () => void;

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
  showNamespaceEdges:      true,
  showConnectedNamespaces: false,
  aggregateByNamespace:    false,
  showEngineIcons:         false,
  selectedNode:            null,
  selectedEdges:           [],
  nodeInfo:                null,
  nodeInfoLoading:         false,
  searchQuery:             '',
  layoutAlgorithm:         'dagre',
  view:                    'graph',

  reachabilitySource:      null,
  reachability:            null,
  reachabilityLoading:     false,
  reachabilityTarget:      null,

  edgeReachability:        null,
  edgeReachabilityLoading: false,

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
    } catch (e) {
      set({ loading: false, error: String(e) });
    }
  },

  toggleNamespace: (ns) =>
    set((state) => {
      const next = new Set(state.selectedNamespaces);
      next.has(ns) ? next.delete(ns) : next.add(ns);
      return { selectedNamespaces: next, selectedNode: null, selectedEdges: [] };
    }),

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

  setSelectedNode: (node) => {
    const src = get().reachabilitySource;
    // If a reachability source is pinned and the next click is on a different
    // node, treat the click as "check reach from src → this node" and fire
    // /api/reachable instead of overwriting the selection.
    if (src && node && node.id !== src.id) {
      set({ reachabilityTarget: node, reachabilityLoading: true, reachability: null });
      fetchReachability(src.id, src.namespace || src.id, node.id, node.namespace || node.id)
        .then((result) => {
          // bail if source was unpinned or target changed mid-fetch
          if (get().reachabilitySource?.id !== src.id || get().reachabilityTarget?.id !== node.id) return;
          set({ reachability: result, reachabilityLoading: false });
        })
        .catch(() => set({ reachabilityLoading: false }));
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
  clearReachability: () =>
    set({ reachabilitySource: null, reachability: null, reachabilityTarget: null, reachabilityLoading: false }),

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

  filteredNodes: () => _filteredNodes(get()),
  filteredEdges: () => _filteredEdges(get()),
}));
