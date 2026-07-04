import { describe, it, expect } from 'vitest';
import { groupNeighborsByPolicy, distinctPeers } from './groupNeighborsByPolicy';
import type { NeighborRef, Rule, WorkloadNode } from '../../../data/policies';

const workload = (id: string, label: string): WorkloadNode => ({
  id,
  label,
  namespace: 'observability',
  type:      'deployment',
  labels:    {},
});

const ref = (peerId: string, rule: Partial<Rule> = {}): NeighborRef => ({
  Workload: workload(peerId, peerId),
  Rule: {
    srcId:     peerId,
    dstId:     'prometheus',
    ports:     [],
    direction: 'ingress',
    action:    0,
    contributor: { source: 'k8s', namespace: 'observability', name: 'prometheus-ingress', ruleIndex: 0 },
    ...rule,
  },
});

describe('groupNeighborsByPolicy', () => {
  it('folds many peers of one policy into a single group', () => {
    // The bloat case: prometheus-ingress admits four sources → four refs that
    // share policy/direction/ports. One header, four peers.
    const groups = groupNeighborsByPolicy([
      ref('grafana'), ref('operator'), ref('gatus'), ref('uptime-kuma'),
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0].rule.contributor?.name).toBe('prometheus-ingress');
    expect(groups[0].peers.map((p) => p.Rule.srcId))
      .toEqual(['grafana', 'operator', 'gatus', 'uptime-kuma']);
  });

  it('splits by policy, ports, and L7 so unlike rules never share a header', () => {
    const groups = groupNeighborsByPolicy([
      ref('a'),
      ref('b', { contributor: { source: 'k8s', namespace: 'observability', name: 'other', ruleIndex: 0 } }),
      ref('c', { ports: [{ port: 9090, protocol: 'TCP' }] }),
      ref('d', { l7Match: { methods: ['GET'] } }),
    ]);
    expect(groups).toHaveLength(4);
  });

  it('keeps contributor-less refs (raw CIDR / unresolved) as standalone groups', () => {
    const groups = groupNeighborsByPolicy([
      ref('a', { contributor: undefined }),
      ref('b', { contributor: undefined }),
    ]);
    expect(groups).toHaveLength(2);
  });
});

describe('distinctPeers', () => {
  it('counts a peer once even when several policies admit it', () => {
    // web-app admitted by two policies → two groups, one distinct peer.
    const groups = groupNeighborsByPolicy([
      ref('web-app'),
      ref('web-app', { contributor: { source: 'k8s', namespace: 'observability', name: 'other', ruleIndex: 0 } }),
      ref('grafana'),
    ]);
    const peers = distinctPeers(groups, false);
    expect(peers.map((p) => p.id).sort()).toEqual(['grafana', 'web-app']);
  });
});
