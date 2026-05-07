import { create } from 'zustand';
import { fetchGraph } from '../api/client';
import type { WorkloadNode, PolicyEdge, StatusKey } from '../data/policies';

interface GraphState {
  allNodes:            WorkloadNode[];
  allEdges:            PolicyEdge[];
  availableNamespaces: string[];
  loading:             boolean;
  error:               string | null;

  selectedNamespaces: Set<string>;
  selectedNodeTypes:  Set<string>;
  selectedStatuses:   Set<StatusKey>;
  showNamespaceEdges: boolean;
  selectedNode:       WorkloadNode | null;
  selectedEdges:      PolicyEdge[];
  searchQuery:        string;
  layoutAlgorithm:    string;

  loadGraph:            () => Promise<void>;
  toggleNamespace:      (ns: string) => void;
  toggleNodeType:       (type: string) => void;
  toggleStatus:         (key: StatusKey) => void;
  toggleNamespaceEdges: () => void;
  setSelectedNode:      (node: WorkloadNode | null) => void;
  setSelectedEdges:     (edges: PolicyEdge[]) => void;
  setSearchQuery:       (q: string) => void;
  setLayoutAlgorithm:   (algo: string) => void;

  filteredNodes: () => WorkloadNode[];
  filteredEdges: () => PolicyEdge[];
}

const ALL_TYPES = ['service', 'deployment', 'headless', 'external'];

export const useGraphStore = create<GraphState>((set, get) => ({
  allNodes:            [],
  allEdges:            [],
  availableNamespaces: [],
  loading:             false,
  error:               null,

  selectedNamespaces: new Set<string>(),
  selectedNodeTypes:  new Set(ALL_TYPES),
  selectedStatuses:   new Set<StatusKey>(),
  showNamespaceEdges: true,
  selectedNode:       null,
  selectedEdges:      [],
  searchQuery:        '',
  layoutAlgorithm:    'dagre',

  loadGraph: async () => {
    set({ loading: true, error: null });
    try {
      const data = await fetchGraph();
      const derived = [...new Set(data.nodes.map((n) => n.namespace).filter(Boolean))];
      set({
        allNodes:            data.nodes,
        allEdges:            data.edges,
        availableNamespaces: derived,
        selectedNamespaces:  new Set(derived),
        loading:             false,
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

  setSelectedNode:      (node)  => set({ selectedNode: node,  selectedEdges: [] }),
  setSelectedEdges:     (edges) => set({ selectedEdges: edges, selectedNode: null }),
  setSearchQuery:       (q)     => set({ searchQuery: q }),
  setLayoutAlgorithm:   (algo)  => set({ layoutAlgorithm: algo }),

  filteredNodes: () => {
    const { allNodes, selectedNamespaces, selectedNodeTypes, searchQuery, selectedStatuses } = get();
    const q = searchQuery.toLowerCase();
    return allNodes.filter((n) => {
      if (!(n.namespace === '' || selectedNamespaces.has(n.namespace))) return false;
      if (!selectedNodeTypes.has(n.type)) return false;
      if (q !== '' && !n.label.toLowerCase().includes(q) && !n.namespace.toLowerCase().includes(q)) return false;
      if (selectedStatuses.size > 0 && n.type !== 'external') {
        if (!n.statuses?.some((s) => selectedStatuses.has(s))) return false;
      }
      return true;
    });
  },

  filteredEdges: () => {
    const { allEdges, selectedNamespaces, showNamespaceEdges } = get();
    const visibleWorkloadIds = new Set(get().filteredNodes().map((n) => n.id));
    const occupiedNS = new Set(
      get().allNodes.filter((n) => visibleWorkloadIds.has(n.id) && n.namespace).map((n) => n.namespace)
    );
    const visibleIds = new Set([
      ...visibleWorkloadIds,
      ...Array.from(selectedNamespaces).filter((ns) => occupiedNS.has(ns)).map((ns) => `ns-${ns}`),
    ]);
    return allEdges.filter((e) => {
      if (e.level === 'namespace')
        return showNamespaceEdges && visibleIds.has(e.source) && visibleIds.has(e.target);
      return visibleIds.has(e.source) && visibleIds.has(e.target);
    });
  },
}));
