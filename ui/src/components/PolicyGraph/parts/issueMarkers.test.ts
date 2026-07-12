import { describe, it, expect } from 'vitest';
import { buildIssueMarks, pairKey } from './issueMarkers';
import type { Issue, WorkloadNode } from '../../../data/policies';

const node = (id: string, namespace: string): WorkloadNode =>
  ({ id, label: id, namespace, type: 'deployment', labels: {} } as WorkloadNode);

const webA = node('web-a', 'frontend');
const apiB = node('api-b', 'backend');
const apiC = node('api-c', 'backend');

const issue = (partial: Partial<Issue>): Issue =>
  ({ type: 'policy conflict', message: '', ...partial } as Issue);

describe('buildIssueMarks', () => {
  it('marks pairs for edge issues, nodes for node issues, skips info tier, folds tiers', () => {
    const issues: Issue[] = [
      // Same pair blocked by two engines — mergeIssuesByPair folds to one mark.
      issue({ engine: 'k8s',   src: webA, dst: apiB }),
      issue({ engine: 'istio', src: webA, dst: apiB }),
      // Warning-tier edge issue on another pair.
      issue({ type: 'no dns', src: apiC, dst: apiB }),
      // Node-scoped lockout.
      issue({ type: 'node lockout', node: apiC }),
      // Info tier — must not mark anything.
      issue({ type: 'partial access', src: webA, dst: apiC }),
    ];

    const marks = buildIssueMarks(issues, false);

    expect(marks.pairs.get(pairKey('web-a', 'api-b'))).toEqual({ tier: 'blocking', count: 1 });
    expect(marks.pairs.get(pairKey('api-c', 'api-b'))).toEqual({ tier: 'warning', count: 1 });
    expect(marks.pairs.has(pairKey('web-a', 'api-c'))).toBe(false);
    expect(marks.nodes.get('api-c')).toEqual({ tier: 'blocking', count: 1 });
    // Edge issues never double-mark endpoint nodes.
    expect(marks.nodes.has('web-a')).toBe(false);
  });

  it('aggregated mode rolls endpoints up to ns nodes; intra-ns pairs become node marks', () => {
    const issues: Issue[] = [
      issue({ src: webA, dst: apiB }),                    // cross-ns → ns pair
      issue({ type: 'no dns', src: apiC, dst: apiB }),    // same ns → node mark, warning
      issue({ type: 'node lockout', node: apiC }),        // node → ns node, blocking wins
    ];

    const marks = buildIssueMarks(issues, true);

    expect(marks.pairs.get(pairKey('ns-frontend', 'ns-backend'))).toEqual({ tier: 'blocking', count: 1 });
    expect(marks.nodes.get('ns-backend')).toEqual({ tier: 'blocking', count: 2 });
  });
});
