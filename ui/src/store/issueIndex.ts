// Issue index helpers. Given the full issues[] returned by /api/issues, build
// lookup maps so the tables view can show per-row counts without walking the
// list for every render.
//
// A workload row shows an issue if it appears in issue.src, issue.dst, or
// issue.node. A policy row shows an issue if any of the issue's culprit refs
// (ingress or egress) matches the row's engine + namespace + name.

import type { Issue, IssueType } from '../data/policies';
import { issueCulprits } from '../data/policies';

export type IssueIndex = Map<string, Issue[]>;

// Node ID → issues touching that node (src, dst, or node role).
export function indexIssuesByNode(issues: Issue[], types: Set<IssueType>): IssueIndex {
  const map: IssueIndex = new Map();
  const push = (id: string | undefined, issue: Issue) => {
    if (!id) return;
    const cur = map.get(id);
    if (cur) cur.push(issue);
    else map.set(id, [issue]);
  };
  for (const issue of issues) {
    if (types.size > 0 && !types.has(issue.type)) continue;
    push(issue.src?.id, issue);
    push(issue.dst?.id, issue);
    push(issue.node?.id, issue);
  }
  return map;
}

export function policyIssueKey(source: string, namespace: string, name: string): string {
  return `${source}|${namespace}|${name}`;
}

// Policy (engine+ns+name) → issues that cite this policy as a culprit.
export function indexIssuesByPolicy(issues: Issue[], types: Set<IssueType>): IssueIndex {
  const map: IssueIndex = new Map();
  for (const issue of issues) {
    if (types.size > 0 && !types.has(issue.type)) continue;
    // issueCulprits already dedups across directions, so no per-issue seen set.
    for (const culprit of issueCulprits(issue)) {
      const key = policyIssueKey(culprit.source, culprit.namespace, culprit.name);
      const cur = map.get(key);
      if (cur) cur.push(issue);
      else map.set(key, [issue]);
    }
  }
  return map;
}

// Total count per issue type across all issues — drives the filter dropdown chip.
export function countIssuesByType(issues: Issue[]): Record<IssueType, number> {
  const counts: Record<string, number> = {};
  for (const issue of issues) {
    counts[issue.type] = (counts[issue.type] ?? 0) + 1;
  }
  return counts as Record<IssueType, number>;
}
