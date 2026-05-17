import { create } from 'zustand';
import { fetchGraph, fetchClusterState } from '../api/client';
import type { WorkloadNode, PolicyEdge, StatusKey } from '../data/policies';
import { filteredNodes as _filteredNodes, filteredEdges as _filteredEdges } from './filters';

interface GraphState {
  allNodes:             WorkloadNode[];
  allEdges:             PolicyEdge[];
  availableNamespaces:  string[];
  availableStatusKeys:  StatusKey[];
  loading:              boolean;
  error:                string | null;

  selectedNamespaces:      Set<string>;
  selectedNodeTypes:       Set<string>;
  selectedStatuses:        Set<StatusKey>;
  showNamespaceEdges:      boolean;
  showConnectedNamespaces: boolean;
  aggregateByNamespace:    boolean;
  selectedNode:            WorkloadNode | null;
  selectedEdges:           PolicyEdge[];
  searchQuery:             string;
  layoutAlgorithm:         string;

  loadClusterState:            () => Promise<void>;
  loadGraph:                   () => Promise<void>;
  toggleNamespace:             (ns: string) => void;
  toggleNodeType:              (type: string) => void;
  toggleStatus:                (key: StatusKey) => void;
  toggleNamespaceEdges:        () => void;
  toggleConnectedNamespaces:   () => void;
  toggleAggregateByNamespace:  () => void;
  setSelectedNode:             (node: WorkloadNode | null) => void;
  setSelectedEdges:            (edges: PolicyEdge[]) => void;
  setSearchQuery:              (q: string) => void;
  setLayoutAlgorithm:          (algo: string) => void;

  filteredNodes: () => WorkloadNode[];
  filteredEdges: () => PolicyEdge[];
}

const ALL_TYPES = ['service', 'deployment', 'headless', 'external', 'cronjob'];

export const useGraphStore = create<GraphState>((set, get) => ({
  allNodes:            [],
  allEdges:            [],
  availableNamespaces:  [],
  availableStatusKeys:  [],
  loading:              false,
  error:                null,

  selectedNamespaces:      new Set<string>(),
  selectedNodeTypes:       new Set(ALL_TYPES),
  selectedStatuses:        new Set<StatusKey>(),
  showNamespaceEdges:      true,
  showConnectedNamespaces: false,
  aggregateByNamespace:    false,
  selectedNode:            null,
  selectedEdges:           [],
  searchQuery:             '',
  layoutAlgorithm:         'dagre',

  loadClusterState: async () => {
    try {
      const state = await fetchClusterState();
      set({
        availableNamespaces: state.availableNs,
        availableStatusKeys: state.statusKeys,
        selectedNamespaces:  new Set(state.availableNs),
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

  toggleNamespaceEdges: () =>
    set((s) => ({ showNamespaceEdges: !s.showNamespaceEdges })),

  toggleConnectedNamespaces: () =>
    set((s) => ({ showConnectedNamespaces: !s.showConnectedNamespaces })),

  toggleAggregateByNamespace: () =>
    set((s) => ({ aggregateByNamespace: !s.aggregateByNamespace })),

  setSelectedNode:      (node)  => set({ selectedNode: node,  selectedEdges: [] }),
  setSelectedEdges:     (edges) => set({ selectedEdges: edges, selectedNode: null }),
  setSearchQuery:       (q)     => set({ searchQuery: q }),
  setLayoutAlgorithm:   (algo)  => set({ layoutAlgorithm: algo }),

  filteredNodes: () => _filteredNodes(get()),
  filteredEdges: () => _filteredEdges(get()),
}));
