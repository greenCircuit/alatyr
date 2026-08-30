// Scenario deep links: resolve a registry entry (data/scenarios.ts) against the
// loaded graph and restore its landing state in one shot.
//
// Resolution is by issue type first — every scenario lands on a finding, not on
// a picture — with an optional workload hint to disambiguate a type that fires
// on several workloads. When nothing resolves, the caller is told so: the
// caption card degrades to "not present in this snapshot" instead of narrating
// a finding no panel is showing.

import type { Issue, WorkloadNode } from '../data/policies';
import type { Scenario, ScenarioTarget, WorkloadRef } from '../data/scenarios';
import { DEFAULT_SCENARIO, scenarioById } from '../data/scenarios';
import { useGraphStore } from './graphStore';

const SCENARIO_PARAM = 's';

export function findWorkload(nodes: WorkloadNode[], ref: WorkloadRef): WorkloadNode | null {
  return nodes.find((node) => node.namespace === ref.namespace && node.label === ref.label) ?? null;
}

function issueTouchesWorkload(issue: Issue, ref: WorkloadRef): boolean {
  return [issue.src, issue.dst, issue.node].some(
    (node) => node?.namespace === ref.namespace && node?.label === ref.label,
  );
}

// Exact match (type + named workload) wins; a type-only match is the fallback
// so a scenario keeps working when fixtures move the finding to a sibling
// workload. Returns null when the type itself stopped firing — that is a real
// regression and the card says so.
export function findIssue(issues: Issue[], target: ScenarioTarget | undefined): Issue | null {
  if (!target?.issueType) return null;
  const ofType = issues.filter((issue) => issue.type === target.issueType);
  if (ofType.length === 0) return null;
  if (target.workload) {
    const exact = ofType.find((issue) => issueTouchesWorkload(issue, target.workload!));
    if (exact) return exact;
  }
  return ofType[0];
}

export function scenarioIdFromSearch(search: string): string | null {
  return new URLSearchParams(search).get(SCENARIO_PARAM);
}

// Rewrites `?s=` in place, keeping the rest of the URL (base path, hash) so the
// short link stays shareable from wherever the visitor happens to be.
export function pushScenarioToUrl(id: string): void {
  const url = new URL(window.location.href);
  url.searchParams.set(SCENARIO_PARAM, id);
  window.history.replaceState(null, '', url.toString());
}

export type ScenarioResolution = 'resolved' | 'unresolved' | 'no-target';

// Applies the landing state, then resolves the target. Namespace/view writes
// happen unconditionally: even an unresolved target should leave the visitor in
// the right neighbourhood rather than on the default cluster-wide view.
export function applyScenario(scenario: Scenario): ScenarioResolution {
  const store = useGraphStore.getState();

  store.clearReachability();
  store.setSelectedNode(null);
  store.setSelectedEdges([]);

  store.setSelectedNamespaces(
    scenario.namespaces.length > 0 ? scenario.namespaces : store.availableNamespaces,
  );
  store.setView(scenario.view);
  if (scenario.tablesTab) store.setTablesTab(scenario.tablesTab);
  store.setSelectedIssueTypes(scenario.issueFilter ? [scenario.issueFilter] : []);

  if (!scenario.target) return 'no-target';

  const issue = findIssue(store.issues, scenario.target);
  // Tables landings stop at the filtered list — the card's instruction is to
  // open the row, and auto-opening a panel over it would answer the question
  // the visitor was asked to answer. The target is still resolved so the card
  // can flag a finding that stopped firing.
  if (scenario.view === 'tables') return issue ? 'resolved' : 'unresolved';

  if (issue?.src && issue.dst) {
    // Edge-scoped finding — open the verdict panel for the pair, which is where
    // the cross-engine AND is spelled out.
    store.showReachability(issue.src, issue.dst);
    return 'resolved';
  }
  const node = issue?.node
    ?? (scenario.target.workload ? findWorkload(store.allNodes, scenario.target.workload) : null);
  if (node) {
    store.setSelectedNode(node);
    return issue ? 'resolved' : 'unresolved';
  }
  return 'unresolved';
}

// Scenario named in the URL, falling back to the differentiated scenario so a
// single-click visitor still lands on the contradiction.
export function initialScenarioId(search: string): string {
  return scenarioIdFromSearch(search) ?? DEFAULT_SCENARIO;
}

export function applyScenarioById(id: string): ScenarioResolution | null {
  const scenario = scenarioById(id);
  if (!scenario) return null;
  pushScenarioToUrl(scenario.id);
  return applyScenario(scenario);
}
