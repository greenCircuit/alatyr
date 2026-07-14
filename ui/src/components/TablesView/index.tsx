// Tabular view of the same data the graph renders. Audience is SREs and
// security folks doing audit/hygiene work — questions like "list every
// workload with internet-ingress", "find policies that select nothing",
// "which workloads does NetworkPolicy foo cover". Reuses store filters so
// switching tabs doesn't lose context.

import { useMemo } from 'react';
import { useGraphStore } from '../../store/graphStore';
import { filteredNodes as deriveFilteredNodes, filteredEdges as deriveFilteredEdges } from '../../store/filters';
import WorkloadsTable from './WorkloadsTable';
import PoliciesTable from './PoliciesTable';
import IssuesTable from './IssuesTable';
import StatusRollup from './StatusRollup';
import EngineRollup from './EngineRollup';
import IssueRollup from './IssueRollup';
import { indexIssuesByNode, indexIssuesByPolicy } from '../../store/issueIndex';
import type { IssueType } from '../../data/policies';

const EMPTY_ISSUE_TYPES = new Set<IssueType>();

export default function TablesView() {
  // Tab lives in the store so the cluster status page can land on a specific
  // tab (issue chip → issues tab) before switching the view.
  const tab    = useGraphStore((s) => s.tablesTab);
  const setTab = useGraphStore((s) => s.setTablesTab);

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
  const statusFilteredNodes = useMemo(() => {
    if (selectedStatuses.size === 0) return nodesWithNs;
    return nodesWithNs.filter((n) => n.statuses?.some((k) => selectedStatuses.has(k)));
  }, [nodesWithNs, selectedStatuses]);

  // Issue index built from ALL issues so table rows can render a per-row count
  // regardless of whether the filter is engaged. When the filter narrows to a
  // subset of types, a second index (nodeIssuesFiltered) drives the row-hide
  // behavior so the counts on visible rows still reflect the current filter.
  const issues              = useGraphStore((s) => s.issues);
  const selectedIssueTypes  = useGraphStore((s) => s.selectedIssueTypes);
  const nodeIssuesAll       = useMemo(() => indexIssuesByNode(issues, EMPTY_ISSUE_TYPES), [issues]);
  const policyIssuesAll     = useMemo(() => indexIssuesByPolicy(issues, EMPTY_ISSUE_TYPES), [issues]);
  const nodeIssuesFiltered  = useMemo(() => indexIssuesByNode(issues, selectedIssueTypes), [issues, selectedIssueTypes]);
  const policyIssuesFiltered = useMemo(() => indexIssuesByPolicy(issues, selectedIssueTypes), [issues, selectedIssueTypes]);

  const tableNodes = useMemo(() => {
    if (selectedIssueTypes.size === 0) return statusFilteredNodes;
    return statusFilteredNodes.filter((n) => nodeIssuesFiltered.has(n.id));
  }, [statusFilteredNodes, selectedIssueTypes, nodeIssuesFiltered]);

  // When the issue-type filter is active, narrow the policies tab to policies
  // that show up as a contributor on a matching issue. Base `edges` still drives
  // workload policy counts, so column-level chip counts stay accurate.
  const tableEdges = useMemo(() => {
    if (selectedIssueTypes.size === 0) return edges;
    return edges.filter((e) => policyIssuesFiltered.has(`${e.policySource}|${e.namespace}|${e.policyName}`));
  }, [edges, selectedIssueTypes, policyIssuesFiltered]);

  // Issues tab rows: respect the type filter so the chip row above stays
  // consistent with what's rendered. No filter → all issues.
  const tableIssues = useMemo(() => {
    if (selectedIssueTypes.size === 0) return issues;
    return issues.filter((issue) => selectedIssueTypes.has(issue.type));
  }, [issues, selectedIssueTypes]);

  // Edges pre-engine-filter — used by EngineRollup so chips stay visible
  // when the user pivots. Same pattern as StatusRollup vs Workloads table.
  const availablePolicySources = useGraphStore((s) => s.availablePolicySources);
  const edgesPreEngine = useMemo(() => deriveFilteredEdges({
    ...filterState,
    selectedPolicySources: new Set(availablePolicySources),
  }), [filterState, availablePolicySources]);

  return (
    <div className="d-flex flex-column bg-dark text-light flex-grow-1 overflow-hidden">
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
            Policies <span className="badge bg-secondary ms-1">{tableEdges.length}</span>
          </button>
        </li>
        <li className="nav-item">
          <button
            className={`nav-link ${tab === 'issues' ? 'active' : ''}`}
            onClick={() => setTab('issues')}
            type="button"
          >
            Issues <span className="badge bg-secondary ms-1">{tableIssues.length}</span>
          </button>
        </li>
      </ul>
      {tab === 'workloads' && <StatusRollup nodes={nodes} />}
      {tab === 'policies' && <EngineRollup edges={edgesPreEngine} />}
      {tab === 'issues' && <IssueRollup issues={issues} />}
      <div className="flex-grow-1 overflow-auto">
        {tab === 'workloads' && <WorkloadsTable nodes={tableNodes} edges={edges} nodeIssues={nodeIssuesAll} />}
        {tab === 'policies'  && <PoliciesTable edges={tableEdges} policyIssues={policyIssuesAll} />}
        {tab === 'issues'    && <IssuesTable issues={tableIssues} />}
      </div>
    </div>
  );
}
