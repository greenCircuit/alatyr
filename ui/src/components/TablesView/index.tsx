// Tabular view of the same data the graph renders. Audience is SREs and
// security folks doing audit/hygiene work — questions like "list every
// workload with internet-ingress", "find policies that select nothing",
// "which workloads does NetworkPolicy foo cover". Reuses store filters so
// switching tabs doesn't lose context.

import { useMemo, useState } from 'react';
import { useGraphStore } from '../../store/graphStore';
import { filteredNodes as deriveFilteredNodes, filteredEdges as deriveFilteredEdges } from '../../store/filters';
import WorkloadsTable from './WorkloadsTable';
import PoliciesTable from './PoliciesTable';
import StatusRollup from './StatusRollup';
import EngineRollup from './EngineRollup';

type Tab = 'workloads' | 'policies';

export default function TablesView() {
  const [tab, setTab] = useState<Tab>('workloads');

  // Subscribe to primitive slices and derive in useMemo. Calling
  // store.filteredNodes() in the selector returns a fresh array per render
  // and trips Zustand's reference check → infinite re-render loop.
  const allNodes                = useGraphStore((s) => s.allNodes);
  const allEdges                = useGraphStore((s) => s.allEdges);
  const selectedNamespaces      = useGraphStore((s) => s.selectedNamespaces);
  const selectedNodeTypes       = useGraphStore((s) => s.selectedNodeTypes);
  const selectedPolicySources   = useGraphStore((s) => s.selectedPolicySources);
  const selectedActions         = useGraphStore((s) => s.selectedActions);
  const selectedDirections      = useGraphStore((s) => s.selectedDirections);
  const showNamespaceEdges      = useGraphStore((s) => s.showNamespaceEdges);
  const showConnectedNamespaces = useGraphStore((s) => s.showConnectedNamespaces);
  const searchQuery             = useGraphStore((s) => s.searchQuery);

  const filterState = useMemo(() => ({
    allNodes, allEdges,
    selectedNamespaces, selectedNodeTypes, selectedPolicySources,
    selectedActions, selectedDirections,
    showNamespaceEdges, showConnectedNamespaces, searchQuery,
  }), [
    allNodes, allEdges,
    selectedNamespaces, selectedNodeTypes, selectedPolicySources,
    selectedActions, selectedDirections,
    showNamespaceEdges, showConnectedNamespaces, searchQuery,
  ]);

  const nodes = useMemo(() => deriveFilteredNodes(filterState), [filterState]);
  const edges = useMemo(() => deriveFilteredEdges(filterState), [filterState]);

  // Namespace nodes (type === 'namespace') are excluded from selectedNodeTypes by
  // design — the graph treats them as compound parents, not togglable workload
  // types. The audit table still wants them as rows ("how many policies touch
  // ns-foo as a whole"), so include them here keyed on the namespace filter +
  // search query.
  const nodesWithNs = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    const nsRows = allNodes.filter((n) => {
      if (n.type !== 'namespace') return false;
      if (!selectedNamespaces.has(n.label)) return false;
      if (q !== '' && !n.label.toLowerCase().includes(q)) return false;
      return true;
    });
    return [...nsRows, ...nodes];
  }, [allNodes, nodes, selectedNamespaces, searchQuery]);

  // Status filter applies to the table view (hide non-matching rows) but
  // NOT to the rollup itself — chips need to stay visible so the user can
  // pivot between status keys. The graph view dims instead of hides.
  const selectedStatuses = useGraphStore((s) => s.selectedStatuses);
  const tableNodes = useMemo(() => {
    if (selectedStatuses.size === 0) return nodesWithNs;
    return nodesWithNs.filter((n) => n.statuses?.some((k) => selectedStatuses.has(k)));
  }, [nodesWithNs, selectedStatuses]);

  // Edges pre-engine-filter — used by EngineRollup so chips stay visible
  // when the user pivots. Same pattern as StatusRollup vs Workloads table.
  const availablePolicySources = useGraphStore((s) => s.availablePolicySources);
  const edgesPreEngine = useMemo(() => deriveFilteredEdges({
    ...filterState,
    selectedPolicySources: new Set(availablePolicySources),
  }), [filterState, availablePolicySources]);

  return (
    <div
      className="d-flex flex-column bg-dark text-light"
      style={{ flexGrow: 1, overflow: 'hidden' }}
    >
      <ul className="nav nav-tabs px-3 pt-2 border-secondary" role="tablist">
        <li className="nav-item">
          <button
            className={`nav-link ${tab === 'workloads' ? 'active' : ''}`}
            onClick={() => setTab('workloads')}
            type="button"
          >
            Workloads <span className="badge bg-secondary ms-1">{tableNodes.length}</span>
          </button>
        </li>
        <li className="nav-item">
          <button
            className={`nav-link ${tab === 'policies' ? 'active' : ''}`}
            onClick={() => setTab('policies')}
            type="button"
          >
            Policies <span className="badge bg-secondary ms-1">{edges.length}</span>
          </button>
        </li>
      </ul>
      {tab === 'workloads' && <StatusRollup nodes={nodes} />}
      {tab === 'policies' && <EngineRollup edges={edgesPreEngine} />}
      <div className="flex-grow-1" style={{ overflow: 'auto' }}>
        {tab === 'workloads'
          ? <WorkloadsTable nodes={tableNodes} edges={edges} />
          : <PoliciesTable edges={edges} />}
      </div>
    </div>
  );
}
