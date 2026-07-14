// Cluster status page: operator-facing at-a-glance posture view. Section order
// is decision order — anything broken (issues)? is enforcement sane (coverage)?
// what's exposed (statuses)? where do I drill (namespaces)?
//
// Respects the toolbar filters (namespaces, engines, actions, directions,
// search) so an operator can scope the numbers; a banner flags narrowed scope
// so a headline number never silently lies about being whole-cluster.

import { useMemo } from 'react';
import type { ReactNode } from 'react';
import { useGraphStore } from '../../store/graphStore';
import { filteredNodes as deriveFilteredNodes, filteredEdges as deriveFilteredEdges } from '../../store/filters';
import {
  scopedPolicyEdges, coverageStats, protectionStats, scopedIssues, namespaceStats,
  nodeSeverityStats, SEVERITY_TIERS,
} from '../../store/clusterStats';
import { mergeIssuesByPair, SEVERITY_COLOR } from '../../data/policies';
import { COV } from '../PolicyGraph/parts/coverage';
import { issueTier } from '../FilterPanel/parts/constants';
import StatusRollup from '../TablesView/StatusRollup';
import IssueRollup from '../TablesView/IssueRollup';
import EngineRollup from '../TablesView/EngineRollup';
import StatCards from './StatCards';
import CoverageBar from './CoverageBar';
import ProportionBar from './ProportionBar';
import NamespaceTable from './NamespaceTable';

// Statuses overlap per node, so the bar shows each node's WORST severity —
// a true partition — while the chips below stay per-key. Gray = no signals.
const NO_SIGNAL_COLOR = '#495057';

function Section({ title, link, onLink, children }: {
  title: string; link?: string; onLink?: () => void; children: ReactNode;
}) {
  return (
    <div className="d-flex flex-column gap-2">
      <div className="d-flex align-items-baseline justify-content-between">
        <span className="text-secondary text-uppercase fs-11 tracking-wide fw-bold">{title}</span>
        {link && (
          <button type="button" className="btn btn-link btn-sm p-0 fs-12" onClick={onLink}>
            {link}
          </button>
        )}
      </div>
      {children}
    </div>
  );
}

export default function ClusterStatusView() {
  // Subscribe to primitive slices and derive in useMemo — same reasoning as
  // TablesView (fresh arrays from store getters trip Zustand's reference check).
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
  const availableNamespaces     = useGraphStore((s) => s.availableNamespaces);
  const availablePolicySources  = useGraphStore((s) => s.availablePolicySources);
  const issues                  = useGraphStore((s) => s.issues);
  const loading                 = useGraphStore((s) => s.loading);
  const setView                 = useGraphStore((s) => s.setView);
  const setTablesTab            = useGraphStore((s) => s.setTablesTab);
  const selectNamespaceOnly     = useGraphStore((s) => s.selectNamespaceOnly);

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

  const nodes         = useMemo(() => deriveFilteredNodes(filterState), [filterState]);
  const renderedEdges = useMemo(() => deriveFilteredEdges(filterState), [filterState]);

  // Coverage counts come from allEdges via their own scoping — filteredEdges
  // drops blanket rules (empty endpoint), which are the posture signal here.
  const postureEdges = useMemo(
    () => scopedPolicyEdges(allEdges, { selectedNamespaces, selectedPolicySources, selectedActions, selectedDirections }),
    [allEdges, selectedNamespaces, selectedPolicySources, selectedActions, selectedDirections],
  );
  const coverage   = useMemo(() => coverageStats(postureEdges), [postureEdges]);

  // Edges scoped WITHOUT the engine filter — the engine rollup's chips are the
  // engine filter, so applying it here would make a clicked-off chip count to
  // zero and vanish, with no way to toggle it back. Same pattern as TablesView.
  const preEngineEdges = useMemo(
    () => scopedPolicyEdges(allEdges, {
      selectedNamespaces, selectedPolicySources: new Set(availablePolicySources),
      selectedActions, selectedDirections,
    }),
    [allEdges, selectedNamespaces, availablePolicySources, selectedActions, selectedDirections],
  );
  const protection = useMemo(() => protectionStats(nodes, renderedEdges), [nodes, renderedEdges]);

  const issuesInScope = useMemo(() => scopedIssues(issues, selectedNamespaces), [issues, selectedNamespaces]);
  const mergedIssues  = useMemo(() => mergeIssuesByPair(issuesInScope), [issuesInScope]);
  // Info-tier findings (partial access) are expected layering, not problems —
  // the headline counts only actionable rows. The chips still list all types.
  const actionableIssues = useMemo(
    () => mergedIssues.filter((issue) => issueTier(issue.type) !== 'info'),
    [mergedIssues],
  );
  const hasBlocking = actionableIssues.some((issue) => issueTier(issue.type) === 'blocking');

  const nodeSeverity = useMemo(() => nodeSeverityStats(nodes), [nodes]);

  const nsRows = useMemo(
    () => namespaceStats(nodes, postureEdges, issuesInScope, protection.ns),
    [nodes, postureEdges, issuesInScope, protection.ns],
  );

  // Scope banner facts — only the filters that change what the numbers mean.
  const scopeParts: string[] = [];
  if (selectedNamespaces.size < availableNamespaces.length)
    scopeParts.push(`${selectedNamespaces.size}/${availableNamespaces.length} namespaces`);
  if (selectedPolicySources.size < availablePolicySources.length)
    scopeParts.push(`engines: ${[...selectedPolicySources].join(', ') || 'none'}`);
  if (selectedActions.size < 2) scopeParts.push(selectedActions.has(0) ? 'allow rules only' : 'deny rules only');
  if (selectedDirections.size < 2) scopeParts.push(`${[...selectedDirections].join('') || 'no'} direction only`);
  if (searchQuery.trim() !== '') scopeParts.push(`search “${searchQuery.trim()}”`);

  const openTables = (tab: 'workloads' | 'policies' | 'issues') => () => {
    setTablesTab(tab);
    setView('tables');
  };

  return (
    <div className="flex-grow-1 overflow-auto bg-dark text-light">
      <div className="d-flex flex-column gap-4 p-3 mx-auto container-lg">
        {loading && <div className="text-secondary fs-12">Refreshing cluster data…</div>}
        {scopeParts.length > 0 && (
          <div className="alert alert-secondary bg-dark border-warning text-warning py-2 px-3 mb-0 fs-12">
            Scoped view — {scopeParts.join(' · ')}. Counts reflect current filters, not the whole cluster.
          </div>
        )}

        <StatCards
          workloads={nodes.length}
          policies={coverage.policyTotal}
          namespacesSelected={selectedNamespaces.size}
          namespacesTotal={availableNamespaces.length}
          issues={actionableIssues.length}
          issuesBlocking={hasBlocking}
        />

        <Section title="Issues" link="Open in tables →" onLink={openTables('issues')}>
          <IssueRollup issues={issuesInScope} onToggled={openTables('issues')} />
        </Section>

        <Section title="Workload protection" link="Open in graph →" onLink={() => setView('graph')}>
          <div className="d-flex align-items-center flex-wrap gap-3 fs-12">
            <span title="namespaces with workload-level policies">
              <span aria-hidden="true" style={{ color: COV.workload }}>● </span>
              workload policies <span className="badge bg-secondary">{protection.counts.workload}</span>
            </span>
            <span title="namespaces covered by namespace-selector policies only">
              <span aria-hidden="true" style={{ color: COV.namespace }}>● </span>
              ns-only <span className="badge bg-secondary">{protection.counts.namespace}</span>
            </span>
            <span title="namespaces with no policy coverage at all">
              <span aria-hidden="true" style={{ color: COV.none }}>● </span>
              unprotected <span className="badge bg-secondary">{protection.counts.none}</span>
            </span>
          </div>
        </Section>

        <Section title="Rule coverage" link="Open in tables →" onLink={openTables('policies')}>
          <CoverageBar stats={coverage} />
        </Section>

        <Section title="Node statuses" link="Open in tables →" onLink={openTables('workloads')}>
          <ProportionBar segments={[
            ...SEVERITY_TIERS.map((tier) => ({
              key: `worst: ${tier}`, count: nodeSeverity[tier], color: SEVERITY_COLOR[tier],
            })),
            { key: 'no status signals', count: nodeSeverity.none, color: NO_SIGNAL_COLOR },
          ]} />
          <StatusRollup nodes={nodes} onToggled={openTables('workloads')} />
        </Section>

        <Section title="Policies by engine" link="Open in tables →" onLink={openTables('policies')}>
          <EngineRollup edges={preEngineEdges} />
        </Section>

        <Section title="Namespaces">
          <NamespaceTable
            rows={nsRows}
            onSelect={(ns) => { selectNamespaceOnly(ns); setView('graph'); }}
          />
        </Section>
      </div>
    </div>
  );
}
