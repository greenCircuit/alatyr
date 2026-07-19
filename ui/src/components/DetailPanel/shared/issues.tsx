// Cluster-issue callouts for the detail panel: the findings from /api/issues
// scoped to the selected node or edge pair, so the operator doesn't have to
// leave the panel and hunt the drawer/table for "is this thing broken".
// Reuses CulpritActions so the remediation language matches the Issues table.

import type { CSSProperties } from 'react';
import type { Issue, IssueType } from '../../../data/policies';
import { SEVERITY_COLOR, mergeIssuesByPair } from '../../../data/policies';
import { TYPE_SEVERITY, ISSUE_TYPE_LABEL, ALL_ISSUE_TYPES } from '../../FilterPanel/parts/constants';
import { CulpritActions } from '../../TablesView/CulpritActions';
import { useGraphStore } from '../../../store/graphStore';
import s from '../DetailPanel.module.css';

// Per-row: type chip suppressed — group header already names the type.
function IssueRow({ issue }: { issue: Issue }) {
  const showReachability  = useGraphStore((store) => store.showReachability);
  const selectPolicyByRef = useGraphStore((store) => store.selectPolicyByRef);
  return (
    <div className="d-flex flex-column gap-1">
      <div className={s.policyMetaRow}>
        <span className={`${s.body} ${s.flexFill}`}>{issue.message}</span>
        {issue.src && issue.dst && (
          <button
            type="button"
            className={`${s.ghostButton} flex-shrink-0`}
            onClick={() => showReachability(issue.src!, issue.dst!)}
          >
            Reachability →
          </button>
        )}
      </div>
      <CulpritActions issue={issue} onOpen={selectPolicyByRef} />
    </div>
  );
}

// One type bucket: header (sev dot + type label + count) + body.
// Always expanded — collapse hides signal the operator came here for.
function IssueGroup({ type, issues }: { type: IssueType; issues: Issue[] }) {
  const color = SEVERITY_COLOR[TYPE_SEVERITY[type]];
  return (
    <div className={s.issueGroup}>
      <div className={s.issueGroupSummary}>
        <span className={s.sevDot} style={{ '--sev': color } as unknown as CSSProperties} aria-label={TYPE_SEVERITY[type]}>●</span>
        <span className={s.findingKind}>{ISSUE_TYPE_LABEL[type] ?? type}</span>
        <span className={s.issueGroupCount}>{issues.length}</span>
      </div>
      <div className={s.issueGroupBody}>
        {issues.map((issue, index) => (
          <IssueRow key={index} issue={issue} />
        ))}
      </div>
    </div>
  );
}

function IssueSection({ issues }: { issues: Issue[] }) {
  if (issues.length === 0) return null;
  const byType = new Map<IssueType, Issue[]>();
  for (const issue of issues) {
    const bucket = byType.get(issue.type) ?? [];
    bucket.push(issue);
    byType.set(issue.type, bucket);
  }
  // ALL_ISSUE_TYPES enforces a stable, severity-desc-adjacent order (blocking
  // classes first) so groups never reshuffle as findings flow in.
  const groups = ALL_ISSUE_TYPES.filter((type) => byType.has(type))
    .map((type) => ({ type, items: byType.get(type)! }));

  return (
    <div className={s.semanticWarn}>
      <div className={s.section}>
        {issues.length} finding{issues.length === 1 ? '' : 's'}
      </div>
      <div className="d-flex flex-column gap-2">
        {groups.map(({ type, items }) => (
          <IssueGroup key={type} type={type} issues={items} />
        ))}
      </div>
    </div>
  );
}

// Issues touching one node — as an endpoint of a broken pair or node-scoped.
// Merged by pair first so a path blocked by two engines reads as one finding,
// same identity the Issues table and graph markers use.
export function NodeIssues({ nodeId }: { nodeId: string }) {
  const issues = useGraphStore((store) => store.issues);
  const scoped = mergeIssuesByPair(issues).filter((issue) =>
    issue.node?.id === nodeId || issue.src?.id === nodeId || issue.dst?.id === nodeId,
  );
  return <IssueSection issues={scoped} />;
}

// Issues on one (src, dst) pair, either orientation — the edge click case.
export function PairIssues({ srcId, dstId }: { srcId?: string; dstId?: string }) {
  const issues = useGraphStore((store) => store.issues);
  if (!srcId || !dstId) return null;
  const scoped = mergeIssuesByPair(issues).filter((issue) =>
    (issue.src?.id === srcId && issue.dst?.id === dstId) ||
    (issue.src?.id === dstId && issue.dst?.id === srcId),
  );
  return <IssueSection issues={scoped} />;
}
