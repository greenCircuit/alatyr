import { describe, it, expect } from 'vitest';
import {
  bundleEdges,
  aggregateDirection,
  edgeEngines,
  edgeLabel,
  bundleLabel,
} from './bundling';
import type { PolicyEdge, Port, L7Match } from '../../../data/policies';

const edge = (overrides: Partial<PolicyEdge> = {}): PolicyEdge => ({
  id:           overrides.id           ?? 'e',
  source:       overrides.source       ?? 'a',
  target:       overrides.target       ?? 'b',
  direction:    overrides.direction    ?? 'ingress',
  policyName:   overrides.policyName   ?? 'p',
  namespace:    overrides.namespace    ?? 'ns',
  level:        overrides.level        ?? 'workload',
  policySource: overrides.policySource ?? 'k8s',
  ports:        overrides.ports,
  l7Matches:    overrides.l7Matches,
  action:       overrides.action,
});

const port = (p: number, protocol = 'TCP', extras: Partial<Port> = {}): Port =>
  ({ port: p, protocol, ...extras });

describe('aggregateDirection', () => {
  it("collapses mixed ingress+egress to 'both'", () => {
    expect(aggregateDirection([
      edge({ direction: 'ingress' }),
      edge({ direction: 'egress' }),
    ])).toBe('both');
  });

  it("returns 'both' when any policy already carries 'both'", () => {
    expect(aggregateDirection([
      edge({ direction: 'ingress' }),
      edge({ direction: 'both' }),
    ])).toBe('both');
  });

  it('preserves single direction when all policies agree', () => {
    expect(aggregateDirection([edge({ direction: 'egress' })])).toBe('egress');
    expect(aggregateDirection([
      edge({ direction: 'ingress' }),
      edge({ direction: 'ingress' }),
    ])).toBe('ingress');
  });
});

describe('bundleEdges', () => {
  it('groups policies by (source, target, action)', () => {
    const edges: PolicyEdge[] = [
      edge({ id: '1', source: 'a', target: 'b', policyName: 'allow-1', action: 0 }),
      edge({ id: '2', source: 'a', target: 'b', policyName: 'allow-2', action: 0 }),
      edge({ id: '3', source: 'a', target: 'b', policyName: 'deny-1',  action: 1 }),
      edge({ id: '4', source: 'c', target: 'd', policyName: 'allow-3', action: 0 }),
    ];

    const bundles = bundleEdges(edges);

    expect(bundles).toHaveLength(3);
    const allowAB = bundles.find((b) => b.source === 'a' && b.target === 'b' && b.action === 0);
    const denyAB  = bundles.find((b) => b.source === 'a' && b.target === 'b' && b.action === 1);
    const allowCD = bundles.find((b) => b.source === 'c' && b.target === 'd');
    expect(allowAB?.policies).toHaveLength(2);
    expect(denyAB?.policies).toHaveLength(1);
    expect(allowCD?.policies).toHaveLength(1);
  });

  it('treats missing action as allow (0)', () => {
    const bundles = bundleEdges([
      edge({ source: 'a', target: 'b', action: undefined }),
      edge({ source: 'a', target: 'b', action: 0 }),
    ]);
    expect(bundles).toHaveLength(1);
    expect(bundles[0].action).toBe(0);
  });

  it('flags hasNS when any constituent edge is namespace-level', () => {
    const bundle = bundleEdges([
      edge({ source: 'a', target: 'b', level: 'workload' }),
      edge({ source: 'a', target: 'b', level: 'namespace' }),
    ])[0];
    expect(bundle.hasNS).toBe(true);
  });

  it('derives bundle id from source/target/action', () => {
    const bundle = bundleEdges([edge({ source: 'a', target: 'b', action: 1 })])[0];
    expect(bundle.id).toBe('bnd-a-b-1');
  });
});

describe('edgeEngines', () => {
  it('returns distinct engines in first-seen order', () => {
    expect(edgeEngines([
      edge({ policySource: 'istio' }),
      edge({ policySource: 'k8s' }),
      edge({ policySource: 'istio' }),
    ])).toEqual(['istio', 'k8s']);
  });
});

describe('edgeLabel', () => {
  it("returns 'all ports' when no ports and no L7", () => {
    expect(edgeLabel(edge())).toBe('all ports');
  });

  it("treats {port: 0} sentinel as no port restriction", () => {
    // Backend emits {port: 0, protocol: 'TCP'} for "any port" rules; UI
    // must strip these, not render '0'.
    expect(edgeLabel(edge({ ports: [port(0)] }))).toBe('all ports');
  });

  it('joins ports + L7 with a separator', () => {
    const l7: L7Match[] = [{ methods: ['GET'] }];
    expect(edgeLabel(edge({ ports: [port(80)], l7Matches: l7 })))
      .toBe('80, L7: GET');
  });

  it('omits ports half when only L7 is present', () => {
    const l7: L7Match[] = [{ paths: ['/health'] }];
    expect(edgeLabel(edge({ l7Matches: l7 }))).toBe('L7: /health');
  });
});

describe('bundleLabel', () => {
  it("renders 'N policies' when no edge has L7", () => {
    expect(bundleLabel([edge(), edge(), edge()])).toBe('3 policies');
  });

  it("appends '(L7)' when any edge has L7 matchers", () => {
    expect(bundleLabel([
      edge(),
      edge({ l7Matches: [{ methods: ['POST'] }] }),
    ])).toBe('2 policies (L7)');
  });
});
