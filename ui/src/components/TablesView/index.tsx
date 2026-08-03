// Tabular view of the same data the graph renders. Audience is SREs and
// security folks doing audit/hygiene work — questions like "list every
// workload with internet-ingress", "find policies that select nothing",
// "which workloads does NetworkPolicy foo cover". Reuses store filters so
// switching tabs doesn't lose context.

import { useMemo } from 'react';
import { useGraphStore } from '../../store/graphStore';
import { filteredNodes as deriveFilteredNodes, filteredEdges as deriveFilteredEdges, nodeMatchesMeshFilter } from '../../store/filters';
import { policyTableEdges } from '../../store/policyTableEdges';
import WorkloadsTable from './WorkloadsTable';
import PoliciesTable from './PoliciesTable';
import IssuesTable from './IssuesTable';
import StatusRollup from './StatusRollup';
import EngineRollup from './EngineRollup';
import IssueRollup from './IssueRollup';
import { indexIssuesByNode, indexIssuesByPolicy } from '../../store/issueIndex';
import type { IssueType } from '../../data/policies';
import s from '../DetailPanel/DetailPanel.module.css';

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
  const selectedMeshFilters     = useGraphStore((s) => s.selectedMeshFilters);
  const meshStatus              = useGraphStore((s) => s.meshStatus);

  const filterState = useMemo(() => ({
    allNodes, allEdges,
    selectedNamespaces, selectedNodeTypes, selectedPolicySources,
    selectedActions, selectedDirections,
    selectedMeshFilters, meshStatus,
    showNamespaceEdges, showConnectedNamespaces, searchQuery,
  }), [
    allNodes, allEdges,
    selectedNamespaces, selectedNodeTypes, selectedPolicySources,
    selectedActions, selectedDirections,
    selectedMeshFilters, meshStatus,
    showNamespaceEdges, showConnectedNamespaces, searchQuery,
  ]);

  const nodes = useMemo(() => deriveFilteredNodes(filterState), [filterState]);
  const edges = useMemo(() => deriveFilteredEdges(filterState), [filterState]);

  const availableNamespaces = useGraphStore((s) => s.availableNamespaces);
  const allNamespaceSet = useMemo(() => new Set(availableNamespaces), [availableNamespaces]);
  // Policies tab only — the workloads tab counts policies per workload, where a
  // cluster-wide row from another namespace would be noise.
  const policyEdges = useMemo(
    () => policyTableEdges(filterState, allNamespaceSet),
    [filterState, allNamespaceSet],
  );

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
      // Namespace nodes carry their own mesh membership — apply the same mesh
      // filter so "in mesh" / mtls selections don't leak out-of-mesh ns rows.
      if (!nodeMatchesMeshFilter(n, meshStatus, selectedMeshFilters)) return false;
      return true;
    });
    return [...nsRows, ...nodes];
  }, [allNodes, nodes, selectedNamespaces, searchQuery, meshStatus, selectedMeshFilters]);

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
    if (selectedIssueTypes.size === 0) return policyEdges;
    return policyEdges.filter((e) => policyIssuesFiltered.has(`${e.policySource}|${e.namespace}|${e.policyName}`));
  }, [policyEdges, selectedIssueTypes, policyIssuesFiltered]);

  // Badge counts distinct policy manifests, not raw edges. A Calico GNP with
  // Allow + Deny rules emits multiple edges but shows as one row in
  // PoliciesTable — badge must track the row count or operators distrust it.
  const policyRowCount = useMemo(() => {
    const keys = new Set<string>();
    for (const edge of tableEdges) {
      keys.add(`${edge.policySource}|${edge.namespace}|${edge.policyName}`);
    }
    return keys.size;
  }, [tableEdges]);

  // Issues tab rows: respect the type filter so the chip row above stays
  // consistent with what's rendered. No filter → all issues.
  const tableIssues = useMemo(() => {
    if (selectedIssueTypes.size === 0) return issues;
    return issues.filter((issue) => selectedIssueTypes.has(issue.type));
  }, [issues, selectedIssueTypes]);

  // Edges pre-engine-filter — used by EngineRollup so chips stay visible
  // when the user pivots. Same pattern as StatusRollup vs Workloads table.
  const availablePolicySources = useGraphStore((s) => s.availablePolicySources);
  const edgesPreEngine = useMemo(() => policyTableEdges({
    ...filterState,
    selectedPolicySources: new Set(availablePolicySources),
  }, allNamespaceSet), [filterState, availablePolicySources, allNamespaceSet]);

  return (
    <div className="d-flex flex-column bg-dark text-light flex-grow-1 overflow-hidden">
      <div className={s.tabStrip} role="tablist">
        <button
          type="button"
          className={`${s.tabButton} ${tab === 'workloads' ? s.tabButtonActive : ''}`}
          onClick={() => setTab('workloads')}
        >
          Workloads <span className={s.countChip}>{tableNodes.length}</span>
        </button>
        <button
          type="button"
          className={`${s.tabButton} ${tab === 'policies' ? s.tabButtonActive : ''}`}
          onClick={() => setTab('policies')}
        >
          Policies <span className={s.countChip}>{policyRowCount}</span>
        </button>
        <button
          type="button"
          className={`${s.tabButton} ${tab === 'issues' ? s.tabButtonActive : ''}`}
          onClick={() => setTab('issues')}
        >
          Issues <span className={s.countChip}>{tableIssues.length}</span>
        </button>
      </div>
      {tab === 'workloads' && <StatusRollup nodes={nodes} />}
      {tab === 'policies' && <EngineRollup edges={edgesPreEngine} />}
      {tab === 'issues' && <IssueRollup issues={issues} />}
      <div className="flex-grow-1 overflow-auto">
        {tab === 'workloads' && <WorkloadsTable nodes={tableNodes} edges={edges} nodeIssues={nodeIssuesAll} meshStatus={meshStatus} />}
        {tab === 'policies'  && <PoliciesTable edges={tableEdges} policyIssues={policyIssuesAll} />}
        {tab === 'issues'    && <IssuesTable issues={tableIssues} />}
      </div>
    </div>
  );
}
