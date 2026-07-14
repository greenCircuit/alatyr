// Pure derive functions for the cluster status page. Same pattern as
// filters.ts: take plain data + filter snapshot, return counts — no store
// access, unit-testable.
//
// Coverage counts come from allEdges, NOT filteredEdges: blanket rules
// (deny-all / allow-all / unenforced) arrive with an empty endpoint and are
// dropped by filteredEdges' visibility check, but they're exactly the posture
// signal this page exists to count. Scoping here filters by the policy's own
// namespace + the engine/action/direction selections instead.

import type { WorkloadNode, PolicyEdge, Issue, Coverage, Severity } from '../data/policies';
import { mergeIssuesByPair, STATUS_CFG } from '../data/policies';
import type { CovType } from '../components/PolicyGraph/parts/coverage';
import { computeCoverage } from '../components/PolicyGraph/parts/coverage';
import { issueTier } from '../components/FilterPanel/parts/constants';

// Display order: locked-down first, open postures in the middle, no-ops last.
export const COVERAGE_CLASSES: Coverage[] = [
  'restricted', 'deny all', 'allow all ns', 'allow all', 'unenforced', 'audit',
];

export interface EdgeScopeFilter {
  selectedNamespaces:    Set<string>;
  selectedPolicySources: Set<string>;
  selectedActions:       Set<number>;
  selectedDirections:    Set<string>;
}

// Edges in scope for posture counting. Namespace scoping keys on the policy's
// own namespace (edge.namespace), not endpoint visibility — a blanket rule has
// no drawable endpoint but its policy still lives somewhere.
export function scopedPolicyEdges(edges: PolicyEdge[], filter: EdgeScopeFilter): PolicyEdge[] {
  return edges.filter((edge) => {
    if (!filter.selectedNamespaces.has(edge.namespace)) return false;
    if (!filter.selectedPolicySources.has(edge.policySource)) return false;
    if (!filter.selectedActions.has(edge.action ?? 0)) return false;
    // 'both' edges pass when either direction is selected — same rule as filters.ts.
    if (edge.direction === 'both'
      ? filter.selectedDirections.size === 0
      : !filter.selectedDirections.has(edge.direction)) return false;
    return true;
  });
}

export interface CoverageStats {
  byClass:     Record<Coverage, number>;
  policyTotal: number;
}

const policyKey = (edge: PolicyEdge) => `${edge.policySource}|${edge.namespace}|${edge.policyName}`;

// Count DISTINCT POLICIES per coverage class, not edges — edges are
// render-grouped per peer pair, so counting them would inflate a policy by its
// fan-out. A policy appearing in two classes (restricted edges + a blanket
// marker) counts in both; that's honest, the policy really does both.
export function coverageStats(edges: PolicyEdge[]): CoverageStats {
  const byClassSeen = new Map<Coverage, Set<string>>();
  const allPolicies = new Set<string>();
  for (const edge of edges) {
    // Blank coverage = normal peer rule = restricted.
    const coverageClass: Coverage = edge.coverage || 'restricted';
    let seen = byClassSeen.get(coverageClass);
    if (!seen) {
      seen = new Set();
      byClassSeen.set(coverageClass, seen);
    }
    const key = policyKey(edge);
    seen.add(key);
    allPolicies.add(key);
  }
  const byClass = {} as Record<Coverage, number>;
  for (const coverageClass of COVERAGE_CLASSES) {
    byClass[coverageClass] = byClassSeen.get(coverageClass)?.size ?? 0;
  }
  return { byClass, policyTotal: allPolicies.size };
}

export interface ProtectionStats {
  counts: Record<CovType, number>;   // namespaces per protection level
  ns:     Record<string, CovType>;   // per-namespace, feeds the namespace table
}

// Namespace-level protection rollup from the existing graph classifier.
export function protectionStats(nodes: WorkloadNode[], edges: PolicyEdge[]): ProtectionStats {
  const { ns } = computeCoverage(edges, nodes);
  const counts: Record<CovType, number> = { workload: 0, namespace: 0, none: 0 };
  for (const level of Object.values(ns)) counts[level] += 1;
  return { counts, ns };
}

// Issues touching any selected namespace via src, dst, or node. Namespace-less
// issues (whole-cluster findings) always pass — hiding them because a filter
// is active would be a quiet lie.
export function scopedIssues(issues: Issue[], selectedNamespaces: Set<string>): Issue[] {
  return issues.filter((issue) => {
    const namespaces = [issue.src?.namespace, issue.dst?.namespace, issue.node?.namespace]
      .filter((ns): ns is string => Boolean(ns));
    if (namespaces.length === 0) return true;
    return namespaces.some((ns) => selectedNamespaces.has(ns));
  });
}

// Worst-first — a node folds to the highest tier any of its statuses hits.
export const SEVERITY_TIERS: Severity[] = ['critical', 'high', 'warning', 'caution', 'info', 'secure'];

export type NodeSeverityBucket = Severity | 'none';

// Status keys overlap (one node carries several), so counting keys can't feed
// a proportion bar. Folding each node to its WORST severity yields a partition:
// every node in exactly one bucket, buckets sum to the node count. 'none' =
// no status signals at all.
export function nodeSeverityStats(nodes: WorkloadNode[]): Record<NodeSeverityBucket, number> {
  const rank = new Map(SEVERITY_TIERS.map((tier, position) => [tier, position]));
  const counts = { critical: 0, high: 0, warning: 0, caution: 0, info: 0, secure: 0, none: 0 };
  for (const node of nodes) {
    let worst: Severity | undefined;
    for (const key of node.statuses ?? []) {
      const severity = STATUS_CFG[key]?.severity;
      if (!severity) continue;
      if (!worst || rank.get(severity)! < rank.get(worst)!) worst = severity;
    }
    counts[worst ?? 'none'] += 1;
  }
  return counts;
}

export interface NamespaceStat {
  namespace:  string;
  workloads:  number;
  protection: CovType;
  policies:   number;   // distinct policies living in this namespace
  issues:     number;   // merged issue rows touching this namespace
}

export function namespaceStats(
  nodes: WorkloadNode[],
  edges: PolicyEdge[],
  issues: Issue[],
  protection: Record<string, CovType>,
): NamespaceStat[] {
  const workloadsByNs = new Map<string, number>();
  for (const node of nodes) {
    if (node.type === 'namespace' || !node.namespace) continue;
    workloadsByNs.set(node.namespace, (workloadsByNs.get(node.namespace) ?? 0) + 1);
  }

  const policiesByNs = new Map<string, Set<string>>();
  for (const edge of edges) {
    if (!edge.namespace) continue;
    let seen = policiesByNs.get(edge.namespace);
    if (!seen) {
      seen = new Set();
      policiesByNs.set(edge.namespace, seen);
    }
    seen.add(policyKey(edge));
  }

  const issuesByNs = new Map<string, number>();
  for (const issue of mergeIssuesByPair(issues)) {
    // Info-tier findings (partial access) are expected policy layering, not
    // problems — counting them as "issues" per namespace inflates the column.
    if (issueTier(issue.type) === 'info') continue;
    // one increment per namespace per issue, even when src+dst share a ns
    const touched = new Set(
      [issue.src?.namespace, issue.dst?.namespace, issue.node?.namespace]
        .filter((ns): ns is string => Boolean(ns)),
    );
    for (const ns of touched) issuesByNs.set(ns, (issuesByNs.get(ns) ?? 0) + 1);
  }

  const rows = [...workloadsByNs.keys()].map((ns) => ({
    namespace:  ns,
    workloads:  workloadsByNs.get(ns) ?? 0,
    protection: protection[ns] ?? 'none',
    policies:   policiesByNs.get(ns)?.size ?? 0,
    issues:     issuesByNs.get(ns) ?? 0,
  }));
  // Worst-first: most issues on top, then weakest protection.
  const protectionRank: Record<CovType, number> = { none: 0, namespace: 1, workload: 2 };
  rows.sort((left, right) =>
    right.issues - left.issues
    || protectionRank[left.protection] - protectionRank[right.protection]
    || left.namespace.localeCompare(right.namespace));
  return rows;
}
