// One row per issue — the "work through the findings" surface. Sits alongside
// Workloads + Policies tabs. Reuses the store's showReachability/setSelectedNode
// handlers so a row click lands in the same panel the drawer does.

import { useMemo, useState, type CSSProperties } from 'react';
import type { Issue, IssueType, Severity, WorkloadNode } from '../../data/policies';
import { SEVERITY_COLOR, mergeIssuesByPair } from '../../data/policies';
import { useGraphStore } from '../../store/graphStore';
import { SortHeader, type SortState, nextSort } from './SortHeader';
import { ISSUE_TYPE_LABEL, issueSeverity } from '../FilterPanel/parts/constants';
import { CulpritActions } from './CulpritActions';
import s from '../DetailPanel/DetailPanel.module.css';

// Numeric rank so severity sort has an intent order (Blocking before Warning).
const SEVERITY_RANK: Record<Severity, number> = {
  critical: 5, high: 4, warning: 3, caution: 2, info: 1, secure: 0,
};

type Col = 'severity' | 'type' | 'scope';

const endpointLabel = (node?: WorkloadNode) => node?.label ?? node?.id ?? '';

export default function IssuesTable({ issues }: { issues: Issue[] }) {
  const showReachability = useGraphStore((s) => s.showReachability);
  const setSelectedNode  = useGraphStore((s) => s.setSelectedNode);
  const selectPolicyByRef = useGraphStore((s) => s.selectPolicyByRef);

  // Default: severity desc — most urgent first, matching the paging-triage
  // workflow. Click any header to override.
  const [sort, setSort] = useState<SortState<Col>>({ col: 'severity', dir: 'desc' });
  // Partial-access rows (expected layering) live in a collapsed group below the
  // findings — dozens of green rows would pad the table, and the collapse
  // doubles as the one-click mute. Default closed.
  const [showInfo, setShowInfo] = useState(false);

  const { findings, infoRows } = useMemo(() => {
    // Fold same-pair, same-engine conflicts (one per blocked direction) into one
    // row carrying both fix groups before sorting.
    const list = mergeIssuesByPair(issues).map((issue) => ({
      issue,
      severity: issueSeverity(issue),
      isEdge: !!(issue.src && issue.dst),
    }));
    if (sort.col) {
      const sign = sort.dir === 'asc' ? 1 : -1;
      list.sort((a, b) => {
        switch (sort.col) {
          case 'severity': return sign * (SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity]);
          case 'type':     return sign * a.issue.type.localeCompare(b.issue.type);
          case 'scope': {
            const scopeKey = (row: typeof a) => row.isEdge
              ? `${endpointLabel(row.issue.src)}→${endpointLabel(row.issue.dst)}`
              : endpointLabel(row.issue.node);
            return sign * scopeKey(a).localeCompare(scopeKey(b));
          }
          default: return 0;
        }
      });
    }
    // Layering group holds only partial-access rows — other info-tier types
    // (mesh policy hygiene) are real findings, not expected layering.
    return {
      findings: list.filter((row) => row.issue.type !== 'partial access'),
      infoRows: list.filter((row) => row.issue.type === 'partial access'),
    };
  }, [issues, sort]);

  const onSort = (col: Col) => setSort((s) => nextSort(s, col));

  // Same handler as the drawer — edge-scoped → reachability panel, node-scoped
  // → select on the graph. Kept identical so users learn one interaction.
  const openIssue = (issue: Issue) => {
    if (issue.src && issue.dst) {
      showReachability(issue.src, issue.dst);
    } else if (issue.node) {
      setSelectedNode(issue.node);
    }
  };

  // Culprit chip → jump to the offending policy. Stops row-click propagation so
  // it doesn't also fire the reachability panel underneath. (The chip's separate
  // YAML button, rendered inside CulpritActions, opens the manifest directly.)
  const openPolicy = (source: string, namespace: string, name: string) =>
    selectPolicyByRef(source, namespace, name);

  if (findings.length === 0 && infoRows.length === 0) {
    return (
      <div className={`${s.tier1} ${s.body} ${s.dim} d-flex align-items-center gap-2`}>
        <span className={s.verdictTextAllow}>✓</span>
        No issues match the current filters.
      </div>
    );
  }

  const renderRow = ({ issue, severity, isEdge }: typeof findings[number], index: number) => (
    <tr
      key={`${issue.type}-${issue.engine ?? ''}-${issue.src?.id ?? ''}-${issue.dst?.id ?? ''}-${issue.node?.id ?? ''}-${index}`}
      role="button"
      onClick={() => openIssue(issue)}
      title={issue.message}
      className="cursor-pointer"
    >
      <td>
        <IssueBadge issueType={issue.type} severity={severity} />
      </td>
      <td className="text-break font-monospace fs-12">
        <ScopeCell issue={issue} isEdge={isEdge} />
      </td>
      <td>
        <CulpritActions issue={issue} onOpen={openPolicy} />
      </td>
    </tr>
  );

  return (
    <table className="table table-dark table-sm table-hover mb-0 fs-13">
      {/* Triage order, left→right: how bad + what class → which pair → why + fix.
          Type is folded into the severity-colored Issue badge (severity is a pure
          function of type, so two columns said one bit). Engine has no column of
          its own — its logo leads each fix subrow in Cause & fix, so the mark sits
          next to the culprit whose engine it names (k8s egress vs istio ingress on
          a merged pair). Message is dropped: it only restated engine + reason. */}
      <thead className="sticky-top bg-dark">
        <tr>
          <SortHeader col="severity" label="Issue"       sort={sort} onSort={onSort} />
          <SortHeader col="scope"    label="Scope"       sort={sort} onSort={onSort} />
          <th className="text-nowrap min-w-260">Cause &amp; fix</th>
        </tr>
      </thead>
      <tbody>
        {findings.map(renderRow)}
        {/* Expected-layering group: collapsed by default so info-class rows
            never pad the findings list — expanding it is the opt-in. */}
        {infoRows.length > 0 && (
          <tr
            role="button"
            className="cursor-pointer"
            onClick={() => setShowInfo((open) => !open)}
          >
            <td colSpan={3} className="text-secondary fs-12 py-2">
              <span className="me-2">{showInfo ? '▾' : '▸'}</span>
              <span className="chip-mini me-2">Expected layering ({infoRows.length})</span>
              ns-level allows narrowed by pod-level baselines — working as designed, nothing to fix
            </td>
          </tr>
        )}
        {showInfo && infoRows.map(renderRow)}
      </tbody>
    </table>
  );
}

// One badge carries both facts: severity by fill color (urgency scan), issue
// class by label. Severity is a pure function of type, so a separate Severity
// column would only restate the color. Tooltip spells out Blocking/Warning for
// non-color readers.
function IssueBadge({ issueType, severity }: { issueType: IssueType; severity: Severity }) {
  const isHigh = SEVERITY_RANK[severity] >= SEVERITY_RANK.high;
  // Three tiers, not two — info-class issues (partial access) are expected
  // behavior, and labeling them "Warning" frames design as a fault.
  const severityTitle = isHigh ? 'Blocking'
    : SEVERITY_RANK[severity] >= SEVERITY_RANK.warning ? 'Warning'
    : 'Informational — expected behavior';
  // Chip itself is severity-coded — tinted bg + text hue, no leading dot.
  return (
    <span
      className={s.findingKindSev}
      style={{ '--sev': SEVERITY_COLOR[severity] } as unknown as CSSProperties}
      title={severityTitle}
    >
      {ISSUE_TYPE_LABEL[issueType]}
    </span>
  );
}

// Endpoint kind badge — answers blast radius at a glance: "partial access on
// observability" reads differently when it's the whole namespace vs one cronjob.
function KindBadge({ node }: { node?: WorkloadNode }) {
  if (!node?.type) return null;
  const isNamespace = node.type === 'namespace';
  return (
    <span className={`${isNamespace ? s.crossNsChip : s.typeChip} ms-1`}>
      {node.type}
    </span>
  );
}

function ScopeCell({ issue, isEdge }: { issue: Issue; isEdge: boolean }) {
  if (isEdge) {
    const srcNs = issue.src?.namespace;
    const dstNs = issue.dst?.namespace;
    return (
      <div className="d-flex flex-column">
        <span>
          {endpointLabel(issue.src)}<KindBadge node={issue.src} />{' '}
          <span className="text-secondary">→</span>{' '}
          {endpointLabel(issue.dst)}<KindBadge node={issue.dst} />
        </span>
        <span className="text-secondary fs-11">
          {srcNs === dstNs ? srcNs : `${srcNs ?? '?'} → ${dstNs ?? '?'}`}
        </span>
      </div>
    );
  }
  return (
    <div className="d-flex flex-column">
      <span>{endpointLabel(issue.node)}<KindBadge node={issue.node} /></span>
      {issue.node?.namespace && (
        <span className="text-secondary fs-11">{issue.node.namespace}</span>
      )}
    </div>
  );
}
