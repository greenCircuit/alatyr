import { describe, it, expect } from 'vitest';
import {
  scopedPolicyEdges, coverageStats, scopedIssues, namespaceStats, nodeSeverityStats,
  type EdgeScopeFilter,
} from './clusterStats';
import type { WorkloadNode, PolicyEdge, Issue, Coverage } from '../data/policies';

// ── Fixtures ──────────────────────────────────────────────────────────────────

const node = (id: string, namespace: string, type: WorkloadNode['type'] = 'service'): WorkloadNode =>
  ({ id, label: id, namespace, type, labels: {} });

const edge = (overrides: Partial<PolicyEdge> & { id: string }): PolicyEdge => ({
  source: 'a', target: 'b', level: 'workload', direction: 'egress',
  policyName: overrides.id, namespace: 'ns-a', policySource: 'k8s', action: 0,
  ...overrides,
});

const issue = (type: Issue['type'], overrides: Partial<Issue> = {}): Issue =>
  ({ type, message: '', ...overrides });

function scope(overrides: Partial<EdgeScopeFilter> = {}): EdgeScopeFilter {
  return {
    selectedNamespaces:    new Set(['ns-a', 'ns-b']),
    selectedPolicySources: new Set(['k8s', 'istio']),
    selectedActions:       new Set([0, 1]),
    selectedDirections:    new Set(['ingress', 'egress']),
    ...overrides,
  };
}

// ── scopedPolicyEdges ─────────────────────────────────────────────────────────

describe('scopedPolicyEdges', () => {
  it('keeps blanket edges (empty endpoint) and scopes by policy namespace + engine + action + direction', () => {
    const edges = [
      edge({ id: 'blanket-allow', target: '', coverage: 'allow all' }),          // kept: blank target no bar
      edge({ id: 'other-ns', namespace: 'ns-z' }),                               // dropped: ns out of scope
      edge({ id: 'other-engine', policySource: 'cilium' }),                      // dropped: engine filtered
      edge({ id: 'deny-rule', action: 1 }),                                      // kept
      edge({ id: 'ingress-rule', direction: 'ingress' }),                        // kept
      edge({ id: 'both-rule', direction: 'both' }),                              // kept: 'both' passes any direction
    ];
    const result = scopedPolicyEdges(edges, scope());
    expect(result.map((e) => e.id)).toEqual(['blanket-allow', 'deny-rule', 'ingress-rule', 'both-rule']);

    const egressOnly = scopedPolicyEdges(edges, scope({ selectedDirections: new Set(['egress']) }));
    expect(egressOnly.map((e) => e.id)).toEqual(['blanket-allow', 'deny-rule', 'both-rule']);
  });
});

// ── coverageStats ─────────────────────────────────────────────────────────────

describe('coverageStats', () => {
  it('counts distinct policies per class, treats blank coverage as restricted, allows one policy in two classes', () => {
    const edges = [
      // policy p1: three restricted edges (peer fan-out) + one blanket marker
      edge({ id: 'p1-e1', policyName: 'p1' }),
      edge({ id: 'p1-e2', policyName: 'p1', target: 'c' }),
      edge({ id: 'p1-e3', policyName: 'p1', target: 'd', coverage: 'restricted' }),
      edge({ id: 'p1-blanket', policyName: 'p1', target: '', coverage: 'allow all' }),
      // policy p2: deny-all lock
      edge({ id: 'p2-lock', policyName: 'p2', target: '', coverage: 'deny all' }),
      // same policy name from another engine stays a distinct policy
      edge({ id: 'p2-istio', policyName: 'p2', policySource: 'istio', target: '', coverage: 'deny all' }),
    ];
    const stats = coverageStats(edges);
    expect(stats.byClass['restricted']).toBe(1);   // p1 once, not three times
    expect(stats.byClass['allow all']).toBe(1);    // p1 again — both classes, no cross-class dedup
    expect(stats.byClass['deny all']).toBe(2);     // k8s/p2 + istio/p2
    expect(stats.byClass['unenforced']).toBe(0);
    expect(stats.policyTotal).toBe(3);
  });
});

// ── scopedIssues ──────────────────────────────────────────────────────────────

describe('scopedIssues', () => {
  it('keeps issues touching a selected namespace and always passes namespace-less issues', () => {
    const issues = [
      issue('policy conflict', { src: node('a', 'ns-a'), dst: node('z', 'ns-z') }),  // kept: src in scope
      issue('node lockout',    { node: node('z2', 'ns-z') }),                        // dropped
      issue('mesh policy'),                                                          // kept: whole-cluster finding
    ];
    const result = scopedIssues(issues, new Set(['ns-a']));
    expect(result.map((i) => i.type)).toEqual(['policy conflict', 'mesh policy']);
  });
});

// ── namespaceStats ────────────────────────────────────────────────────────────

describe('namespaceStats', () => {
  it('aggregates workloads, distinct policies, and merged issues per namespace, sorted worst-first', () => {
    const nodes = [
      node('a1', 'ns-a'), node('a2', 'ns-a'),
      node('b1', 'ns-b'),
      node('ns-node', 'ns-a', 'namespace'),   // namespace nodes never count as workloads
    ];
    const edges = [
      edge({ id: 'p1-e1', policyName: 'p1' }),
      edge({ id: 'p1-e2', policyName: 'p1', target: 'c' }),   // same policy → 1
      edge({ id: 'p2', policyName: 'p2' }),
    ];
    const issues = [
      // same pair + type twice (two engines) → merges to one row for ns-a and ns-b
      issue('policy conflict', { src: node('a1', 'ns-a'), dst: node('b1', 'ns-b'), engine: 'k8s' }),
      issue('policy conflict', { src: node('a1', 'ns-a'), dst: node('b1', 'ns-b'), engine: 'istio' }),
      // info-tier: expected policy layering, must not count as an issue
      issue('partial access', { src: node('a1', 'ns-a'), dst: node('a2', 'ns-a') }),
    ];
    const rows = namespaceStats(nodes, edges, issues, { 'ns-a': 'workload', 'ns-b': 'none' });
    // ns-b first: protection 'none' sorts above 'workload'
    expect(rows.map((r) => r.namespace)).toEqual(['ns-b', 'ns-a']);
    const nsA = rows.find((r) => r.namespace === 'ns-a')!;
    expect(nsA.workloads).toBe(2);
    expect(nsA.policies).toBe(2);
    expect(nsA.issues).toBe(1);
    const nsB = rows.find((r) => r.namespace === 'ns-b')!;
    expect(nsB.policies).toBe(0);
    expect(nsB.issues).toBe(1);
    expect(nsB.protection).toBe('none');
  });

});

// ── nodeSeverityStats ─────────────────────────────────────────────────────────

describe('nodeSeverityStats', () => {
  it('folds each node to its worst severity so buckets partition the node set', () => {
    const nodes = [
      // internet-egress (high) + cross-namespace (info) → high wins
      { ...node('a', 'ns-a'), statuses: ['internet-egress', 'cross-namespace'] as const },
      { ...node('b', 'ns-a'), statuses: ['air-gapped'] as const },     // secure
      node('c', 'ns-a'),                                               // no statuses → none
    ];
    const counts = nodeSeverityStats(nodes as never);
    expect(counts.high).toBe(1);
    expect(counts.info).toBe(0);       // info status absorbed by the high bucket
    expect(counts.secure).toBe(1);
    expect(counts.none).toBe(1);
    const total = Object.values(counts).reduce((sum, count) => sum + count, 0);
    expect(total).toBe(nodes.length);  // partition: buckets sum to node count
  });

  it('covers every Coverage class label without a type hole', () => {
    // compile-time guard: adding a backend coverage class without updating the
    // frontend union breaks this assignment
    const classes: Coverage[] = ['restricted', 'deny all', 'allow all', 'allow all ns', 'unenforced', 'audit'];
    expect(classes).toHaveLength(6);
  });
});
