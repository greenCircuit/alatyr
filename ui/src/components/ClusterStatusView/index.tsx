// Cluster status page: operator-facing at-a-glance posture view. Section order
// follows scan flow — a red callout for public-facing pods with no policy
// (page-worthy), broken (issues)? what enforces (engines)? are rules tight
// (rule coverage)? what exposure results (node statuses)? which workloads to
// look at first (top risky)? drill (namespaces). Toolbar filters scope
// everything — the nav bar carries the scope indicator.

import { useMemo } from 'react';
import type { ReactNode } from 'react';
import { useGraphStore } from '../../store/graphStore';
import { filteredNodes as deriveFilteredNodes, filteredEdges as deriveFilteredEdges } from '../../store/filters';
import {
  scopedPolicyEdges, coverageStats, protectionStats, scopedIssues, namespaceStats,
  nodeSeverityStats, exposedUnpolicedNodes, singleEngineCoveredCount, topRiskyWorkloads,
  SEVERITY_TIERS,
} from '../../store/clusterStats';
import { mergeIssuesByPair, SEVERITY_COLOR } from '../../data/policies';
import { issueTier } from '../FilterPanel/parts/constants';
import StatusRollup from '../TablesView/StatusRollup';
import IssueRollup from '../TablesView/IssueRollup';
import EngineRollup from '../TablesView/EngineRollup';
import StatCards from './StatCards';
import CoverageBar from './CoverageBar';
import ProportionBar from './ProportionBar';
import NamespaceTable from './NamespaceTable';
import ExposedCallout from './ExposedCallout';
import RiskyWorkloadsTable from './RiskyWorkloadsTable';

// Statuses overlap per node, so the bar shows each node's WORST severity —
// a true partition — while the chips below stay per-key. Gray = no signals.
const NO_SIGNAL_COLOR = '#495057';

// Section header: quieter than uppercase eyebrow — `fs-13` semibold title
// + optional `hint` for context (e.g. "click row for detail"). Trailing link
// uses `text-secondary` to match the rest of the app's action-link convention.
function Section({ title, hint, link, onLink, children }: {
  title: string; hint?: string; link?: string; onLink?: () => void; children: ReactNode;
}) {
  return (
    <div className="d-flex flex-column gap-2">
      <div className="d-flex align-items-baseline justify-content-between">
        <div className="d-flex align-items-baseline gap-2">
          <span className="text-light fs-13 fw-semibold">{title}</span>
          {hint && <span className="text-secondary fs-12">{hint}</span>}
        </div>
        {link && (
          <button type="button" className="btn btn-link btn-sm p-0 fs-12 text-secondary" onClick={onLink}>
            {link}
          </button>
        )}
      </div>
      {children}
    </div>
  );
}

// Zone divider: bigger visual break between groups of related sections
// (Posture / Coverage / Drilldowns). Gives the page rhythm so an operator
// can zone in on the group that matches their current task.
function ZoneDivider({ label }: { label: string }) {
  return (
    <div className="d-flex align-items-center gap-3">
      <span className="text-secondary text-uppercase fs-11 tracking-wider fw-bold">{label}</span>
      <span className="flex-grow-1 border-top border-secondary opacity-25" />
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
  const setSelectedNode         = useGraphStore((s) => s.setSelectedNode);

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

  const nodeSeverity = useMemo(() => nodeSeverityStats(nodes), [nodes]);

  const nsRows = useMemo(
    () => namespaceStats(nodes, postureEdges, issuesInScope, protection.ns),
    [nodes, postureEdges, issuesInScope, protection.ns],
  );

  const exposedNodes = useMemo(
    () => exposedUnpolicedNodes(nodes, renderedEdges),
    [nodes, renderedEdges],
  );
  const singleEngineCount = useMemo(
    () => singleEngineCoveredCount(nodes, availablePolicySources),
    [nodes, availablePolicySources],
  );
  const riskyRows = useMemo(
    () => topRiskyWorkloads(nodes, renderedEdges, issuesInScope, 10),
    [nodes, renderedEdges, issuesInScope],
  );

  const openTables = (tab: 'workloads' | 'policies' | 'issues') => () => {
    setTablesTab(tab);
    setView('tables');
  };

  return (
    <div className="flex-grow-1 overflow-auto bg-dark text-light">
      <div className="d-flex flex-column gap-4 p-3 mx-auto container-lg">
        {loading && <div className="text-secondary fs-12">Refreshing cluster data…</div>}

        <ZoneDivider label="Posture" />

        <StatCards
          workloads={nodes.length}
          policies={coverage.policyTotal}
          namespacesSelected={selectedNamespaces.size}
          namespacesTotal={availableNamespaces.length}
          issues={actionableIssues.length}
          issuesBlocking={actionableIssues.filter((issue) => issueTier(issue.type) === 'blocking').length}
          exposedCount={exposedNodes.length}
          exposedNamespaces={new Set(exposedNodes.map((node) => node.namespace)).size}
        />

        <ExposedCallout nodes={exposedNodes} onSeeAll={openTables('workloads')} />

        <ZoneDivider label="Coverage" />

        <Section title="Issues" hint="what's broken right now" link="Open in tables →" onLink={openTables('issues')}>
          <IssueRollup issues={issuesInScope} bare />
        </Section>

        <Section title="Policies by engine" link="Open in tables →" onLink={openTables('policies')}>
          <EngineRollup edges={preEngineEdges} bare />
          {singleEngineCount > 0 && availablePolicySources.length > 1 && (
            <div
              className="d-inline-flex align-items-center gap-2 text-secondary fs-12"
              title="Workloads carrying status keys from only one of the enabled engines — defense-in-depth gap"
            >
              <span className="swatch-dot" style={{ background: SEVERITY_COLOR.warning }} />
              {singleEngineCount} workload{singleEngineCount === 1 ? '' : 's'} covered by only one engine
            </div>
          )}
        </Section>

        <Section title="Rule coverage" link="Open in tables →" onLink={openTables('policies')}>
          <CoverageBar stats={coverage} />
        </Section>

        <Section title="Node statuses" hint="worst-severity per workload" link="Open in tables →" onLink={openTables('workloads')}>
          {nodes.length === 0 ? (
            <div className="text-secondary fs-12">No workloads in the current scope.</div>
          ) : (
            <>
              <ProportionBar
                height={8}
                segments={[
                  ...SEVERITY_TIERS.map((tier) => ({
                    key: `worst: ${tier}`, count: nodeSeverity[tier], color: SEVERITY_COLOR[tier],
                  })),
                  { key: 'no status signals', count: nodeSeverity.none, color: NO_SIGNAL_COLOR },
                ]}
              />
              <StatusRollup nodes={nodes} bare />
            </>
          )}
        </Section>

        <ZoneDivider label="Drilldowns" />

        <Section title="Top risky workloads" hint="ranked worst-first · click row for detail">
          <RiskyWorkloadsTable
            rows={riskyRows}
            onShowNode={(node) => setSelectedNode(node)}
            onOpenGraph={(node) => { setSelectedNode(node); setView('graph'); }}
          />
        </Section>

        <Section title="Namespaces" hint="click row to drill into graph">
          <NamespaceTable
            rows={nsRows}
            onSelect={(ns) => { selectNamespaceOnly(ns); setView('graph'); }}
          />
        </Section>
      </div>
    </div>
  );
}
