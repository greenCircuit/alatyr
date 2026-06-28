import { describe, it, expect } from 'vitest';
import { computeCoverage } from './coverage';
import type { WorkloadNode, PolicyEdge } from '../../../data/policies';

const node = (id: string, namespace: string, overrides: Partial<WorkloadNode> = {}): WorkloadNode => ({
  id,
  label:    id,
  namespace,
  type:     overrides.type ?? 'service',
  labels:   {},
  statuses: overrides.statuses,
});

const edge = (overrides: Partial<PolicyEdge>): PolicyEdge => ({
  id:           'e',
  source:       'a',
  target:       'b',
  direction:    'ingress',
  policyName:   'p',
  namespace:    'ns',
  level:        'workload',
  policySource: 'k8s',
  ...overrides,
});

describe('computeCoverage', () => {
  it('a workload edge covers both endpoints and their namespace', () => {
    const nodes = [node('a', 'ns1'), node('b', 'ns1')];
    const { ns, workload } = computeCoverage([edge({ source: 'a', target: 'b', level: 'workload' })], nodes);
    expect(workload.has('a')).toBe(true);
    expect(workload.has('b')).toBe(true);
    expect(ns.ns1).toBe('workload');
  });

  it('a namespace-level edge marks the ns covered but not its workloads', () => {
    const nodes = [node('a', 'ns1')];
    const { ns, workload } = computeCoverage(
      [edge({ source: 'ns-ns1', target: 'ns-ns2', level: 'namespace' })],
      nodes,
    );
    expect(ns.ns1).toBe('namespace');
    expect(workload.size).toBe(0);
  });

  it('no edges and no protective status is a gap (none)', () => {
    const { ns, workload } = computeCoverage([], [node('a', 'ns1')]);
    expect(ns.ns1).toBe('none');
    expect(workload.size).toBe(0);
  });

  it('a protective status covers a node; internet-* badges do not', () => {
    const nodes = [
      node('a', 'ns1', { statuses: ['air-gapped'] }),
      node('b', 'ns2', { statuses: ['internet-egress'] }),
    ];
    const { ns, workload } = computeCoverage([], nodes);
    expect(workload.has('a')).toBe(true);
    expect(ns.ns1).toBe('workload');
    expect(workload.has('b')).toBe(false);
    expect(ns.ns2).toBe('none');
  });
});
