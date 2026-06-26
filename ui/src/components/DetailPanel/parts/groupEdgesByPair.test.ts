import { describe, it, expect } from 'vitest';
import { groupEdgesByPair } from './policy';
import type { PolicyEdge } from '../../../data/policies';

const edge = (id: string, source: string, target: string): PolicyEdge => ({
  id,
  source,
  target,
  direction:    'ingress',
  policyName:   `p-${id}`,
  namespace:    'ns',
  level:        'workload',
  policySource: 'k8s',
});

describe('groupEdgesByPair', () => {
  it('collapses a graph-edge bundle (all share one pair) into one group', () => {
    // What the user gets when they click a single arrow on the canvas: many
    // policies, same src→dst — the panel should show one pair, many policies.
    const pairs = groupEdgesByPair([
      edge('1', 'a', 'b'),
      edge('2', 'a', 'b'),
      edge('3', 'a', 'b'),
    ]);
    expect(pairs).toHaveLength(1);
    expect(pairs[0].src).toBe('a');
    expect(pairs[0].dst).toBe('b');
    expect(pairs[0].edges.map((e) => e.id)).toEqual(['1', '2', '3']);
  });

  it('splits a Policies-table bundle (one policy, many pairs) into per-pair groups', () => {
    // This is the case the multiPair branch in DetailPanel keys off — if
    // groupEdgesByPair starts collapsing distinct pairs the panel will lie
    // with src/dst from edges[0] only.
    const pairs = groupEdgesByPair([
      edge('1', 'a', 'b'),
      edge('2', 'a', 'c'),
      edge('3', 'd', 'c'),
    ]);
    expect(pairs).toHaveLength(3);
    expect(pairs.map((p) => `${p.src}→${p.dst}`)).toEqual(['a→b', 'a→c', 'd→c']);
  });

  it("treats source/target swap as a distinct pair (direction matters)", () => {
    // a→b ingress and b→a egress can both exist as separate edges of the
    // same policy; they're different rows in the affected-pairs list.
    const pairs = groupEdgesByPair([
      edge('1', 'a', 'b'),
      edge('2', 'b', 'a'),
    ]);
    expect(pairs).toHaveLength(2);
  });
});
