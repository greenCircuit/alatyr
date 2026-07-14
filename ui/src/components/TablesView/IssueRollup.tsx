// Issue-type rollup strip above the Issues table. Live count per issue type over
// the merged rows; clicking a chip toggles that type in `selectedIssueTypes` —
// the same store filter the FilterPanel drives, so graph + every tab stay in
// sync. Mirrors StatusRollup (workloads) and EngineRollup (policies): each tab
// gets one always-visible quick filter over its primary triage dimension.

import { useMemo } from 'react';
import type { Issue, IssueType } from '../../data/policies';
import { SEVERITY_COLOR, mergeIssuesByPair } from '../../data/policies';
import { ISSUE_TYPE_LABEL, TYPE_SEVERITY } from '../FilterPanel/parts/constants';
import { useGraphStore } from '../../store/graphStore';
import r from './Rollup.module.css';

// Blocking classes first — same left-to-right triage order as the table's default
// severity-desc sort. Info-class (partial access) trails last.
const TYPE_ORDER: IssueType[] = [
  'policy conflict', 'mesh conflict', 'node lockout', 'mesh policy', 'no dns', 'partial access',
];

interface IssueRollupProps {
  issues: Issue[];
  // Called after a chip toggles the filter — see StatusRollup for rationale.
  onToggled?: (type: IssueType) => void;
}

export default function IssueRollup({ issues, onToggled }: IssueRollupProps) {
  const selectedIssueTypes = useGraphStore((s) => s.selectedIssueTypes);
  const toggleIssueType = useGraphStore((s) => s.toggleIssueType);

  // Count merged rows (what the table actually renders), not raw issues, so a
  // chip's count equals the rows filtering to that type would leave.
  const counts = useMemo(() => {
    const out = new Map<IssueType, number>();
    for (const issue of mergeIssuesByPair(issues)) {
      out.set(issue.type, (out.get(issue.type) ?? 0) + 1);
    }
    return out;
  }, [issues]);

  // Show every IssueType the backend can emit, including zero-count. A 0 is a
  // positive signal — "this detector ran, found nothing" — so operators know the
  // class is monitored, not silently absent. TYPE_ORDER mirrors models.IssueType.
  const entries = TYPE_ORDER.map((type) => ({ type, count: counts.get(type) ?? 0 }));

  return (
    <div className="d-flex align-items-center flex-wrap gap-2 px-3 py-2 border-bottom border-secondary fs-12">
      <span className="text-secondary fs-11 text-uppercase tracking-wide">
        Issue
      </span>
      {entries.map(({ type, count }) => {
        const color = SEVERITY_COLOR[TYPE_SEVERITY[type]];
        const active = selectedIssueTypes.has(type);
        const dimmed = selectedIssueTypes.size > 0 && !active;
        return (
          <button
            key={type}
            type="button"
            className={`btn btn-sm d-inline-flex align-items-center gap-1 p-1 ${r.chip} ${active ? r.active : ''} ${dimmed ? r.dimmed : ''}`}
            onClick={() => { toggleIssueType(type); onToggled?.(type); }}
            title={`click to ${active ? 'clear filter' : 'filter to these'}`}
            style={{ border: `1px solid ${color}` }}
          >
            <span aria-hidden="true" style={{ color }}>●</span>
            <span>{ISSUE_TYPE_LABEL[type]}</span>
            <span className="badge bg-secondary ms-1">{count}</span>
          </button>
        );
      })}
    </div>
  );
}
