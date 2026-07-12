// Cluster-issue callouts for the detail panel: the findings from /api/issues
// scoped to the selected node or edge pair, so the operator doesn't have to
// leave the panel and hunt the drawer/table for "is this thing broken".
// Reuses CulpritActions so the remediation language matches the Issues table.

import type { Issue } from '../../../data/policies';
import { SEVERITY_COLOR, mergeIssuesByPair } from '../../../data/policies';
import { TYPE_SEVERITY, ISSUE_TYPE_LABEL } from '../../FilterPanel/parts/constants';
import { EngineBadge } from './badges';
import { CulpritActions } from '../../TablesView/CulpritActions';
import { useGraphStore } from '../../../store/graphStore';
import s from '../DetailPanel.module.css';

function IssueCard({ issue }: { issue: Issue }) {
  const showReachability  = useGraphStore((store) => store.showReachability);
  const selectPolicyByRef = useGraphStore((store) => store.selectPolicyByRef);
  const color = SEVERITY_COLOR[TYPE_SEVERITY[issue.type]];
  const engines = issue.engines?.length ? issue.engines : issue.engine ? [issue.engine] : [];
  return (
    <div className={`border rounded p-2 ${s.smallText}`} style={{ borderColor: color }}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className="d-flex align-items-center gap-1 flex-wrap">
          <span className="badge text-ink-dark" style={{ background: color }}>
            {ISSUE_TYPE_LABEL[issue.type] ?? issue.type}
          </span>
          {engines.map((engine) => <EngineBadge key={engine} engine={engine} />)}
        </div>
        {issue.src && issue.dst && (
          <button
            type="button"
            className="btn btn-sm btn-outline-light py-0 px-2 fs-11 flex-shrink-0"
            onClick={() => showReachability(issue.src!, issue.dst!)}
          >
            Open reachability
          </button>
        )}
      </div>
      <div className="text-light mb-1">{issue.message}</div>
      <CulpritActions issue={issue} onOpen={selectPolicyByRef} />
    </div>
  );
}

function IssueSection({ issues }: { issues: Issue[] }) {
  if (issues.length === 0) return null;
  return (
    <div>
      <div className="text-uppercase text-secondary small fw-semibold mb-1">
        Detected issues ({issues.length})
      </div>
      <div className="d-flex flex-column gap-2">
        {issues.map((issue, index) => <IssueCard key={index} issue={issue} />)}
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
