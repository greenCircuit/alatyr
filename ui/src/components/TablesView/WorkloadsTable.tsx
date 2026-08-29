// Workload rows for the audit view. Columns picked for what SREs ask in incidents:
// "which workloads have internet ingress", "what's in the broken ns", "who has
// no policy at all". Row click drops back to the graph with the workload selected.

import { useMemo, useRef, useState } from 'react';
import type { WorkloadNode, PolicyEdge, StatusKey, Issue, MeshMembership, MtlsScope } from '../../data/policies';
import { SEVERITY_COLOR, meshBadgeMeta } from '../../data/policies';
import { MtlsChip } from '../DetailPanel/shared/MtlsChip';
import { issueTier, type IssueTier } from '../FilterPanel/parts/constants';
import { useGraphStore } from '../../store/graphStore';
import { EngineBadge } from '../../data/engineIcons';
import { STATUS_CFG } from '../../data/policies';
import { STATUS_LABELS } from '../FilterPanel/parts/constants';
import fp from '../FilterPanel/FilterPanel.module.css';
import { SortHeader, type SortState, nextSort } from './SortHeader';
import type { IssueIndex } from '../../store/issueIndex';
import { IssuesPopover } from './IssuesPopover';
import s from '../DetailPanel/DetailPanel.module.css';

type Col = 'name' | 'namespace' | 'type' | 'statuses' | 'policies' | 'issues' | 'mesh';

const MESH_SORT_ORDER: Record<string, number> = {
  strict: 0, permissive: 1, disable: 2, unset: 3, out: 4,
};

function meshSortKey(membership: MeshMembership | undefined): number {
  if (!membership || !membership.inMesh) return MESH_SORT_ORDER.out;
  const verdict = membership.mtls?.verdict ?? 'unset';
  return MESH_SORT_ORDER[verdict] ?? MESH_SORT_ORDER.unset;
}

// Tier weights: blocking dominates warning dominates info. Count-agnostic
// across tiers — one blocking issue outranks any number of warnings.
const TIER_WEIGHT: Record<IssueTier, number> = { blocking: 1_000_000, warning: 1_000, info: 1 };

function issueSortKey(issues: Issue[]): number {
  let key = 0;
  for (const issue of issues) key += TIER_WEIGHT[issueTier(issue.type)];
  return key;
}

// Sort within a row so the popover leads with blocking, then warning, then
// info — matches the tier dots on the chip left-to-right.
const TIER_ORDER: Record<IssueTier, number> = { blocking: 0, warning: 1, info: 2 };

function sortIssuesBySeverity(issues: Issue[]): Issue[] {
  return [...issues].sort((a, b) => TIER_ORDER[issueTier(a.type)] - TIER_ORDER[issueTier(b.type)]);
}

export default function WorkloadsTable({
  nodes,
  edges,
  nodeIssues,
  meshStatus,
}: {
  nodes: WorkloadNode[];
  edges: PolicyEdge[];
  nodeIssues: IssueIndex;
  meshStatus: Record<string, MeshMembership>;
}) {
  const setSelectedNode = useGraphStore((s) => s.setSelectedNode);
  const setView         = useGraphStore((s) => s.setView);
  // Default: issues desc — worst rows on top, matches the paging-triage
  // reflex ("what's on fire") without requiring a click.
  const [sort, setSort] = useState<SortState<Col>>({ col: 'issues', dir: 'desc' });

  // Edge count per workload, split by engine so a row can answer "how many
  // k8s rules vs istio rules touch me" at a glance.
  const policyCountByNode = useMemo(() => {
    const map = new Map<string, Record<string, number>>();
    for (const e of edges) {
      for (const id of [e.source, e.target]) {
        const cur = map.get(id) ?? {};
        cur[e.policySource] = (cur[e.policySource] ?? 0) + 1;
        map.set(id, cur);
      }
    }
    return map;
  }, [edges]);

  const rows = useMemo(() => {
    const list = nodes.map((n) => ({
      node: n,
      policyCounts: policyCountByNode.get(n.id) ?? {},
      policyTotal: Object.values(policyCountByNode.get(n.id) ?? {}).reduce((a, b) => a + b, 0),
      issues: sortIssuesBySeverity(nodeIssues.get(n.id) ?? []),
    }));
    if (!sort.col) return list;
    const sign = sort.dir === 'asc' ? 1 : -1;
    list.sort((a, b) => {
      switch (sort.col) {
        case 'name':      return sign * a.node.label.localeCompare(b.node.label);
        case 'namespace': return sign * (a.node.namespace || '').localeCompare(b.node.namespace || '');
        case 'type':      return sign * a.node.type.localeCompare(b.node.type);
        case 'statuses':  return sign * ((a.node.statuses?.length ?? 0) - (b.node.statuses?.length ?? 0));
        case 'policies':  return sign * (a.policyTotal - b.policyTotal);
        case 'issues':    return sign * (issueSortKey(a.issues) - issueSortKey(b.issues));
        case 'mesh':      return sign * (meshSortKey(meshStatus[a.node.id]) - meshSortKey(meshStatus[b.node.id]));
        default:          return 0;
      }
    });
    return list;
  }, [nodes, policyCountByNode, nodeIssues, sort, meshStatus]);

  const onSort = (col: Col) => setSort((s) => nextSort(s, col));

  // Row click → select only; detail panel updates in place. Trailing button
  // → explicit jump to graph so the user controls when to switch views.
  const selectRow = (n: WorkloadNode) => setSelectedNode(n);
  const openInGraph = (n: WorkloadNode, e: React.MouseEvent) => {
    e.stopPropagation();
    setSelectedNode(n);
    setView('graph');
  };

  if (rows.length === 0) {
    return <div className={`${s.tier1} ${s.dim} ${s.body}`}>No workloads match the current filters.</div>;
  }

  return (
    <table className="table table-dark table-sm table-hover mb-0 fs-13">
      <thead className="sticky-top bg-dark">
        <tr>
          <SortHeader col="name"      label="Workload"   sort={sort} onSort={onSort} />
          <SortHeader col="namespace" label="Namespace"  sort={sort} onSort={onSort} />
          <SortHeader col="type"      label="Type"       sort={sort} onSort={onSort} />
          <SortHeader col="statuses"  label="Status"     sort={sort} onSort={onSort} />
          <SortHeader col="policies"  label="Policies"   sort={sort} onSort={onSort} />
          <SortHeader col="issues"    label="Issues"     sort={sort} onSort={onSort} />
          <SortHeader col="mesh"      label="Mesh"       sort={sort} onSort={onSort} />
          <th className="text-nowrap">Labels</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {rows.map(({ node, policyCounts, policyTotal, issues }) => (
          <tr
            key={node.id}
            role="button"
            onClick={() => selectRow(node)}
            className="cursor-pointer"
          >
            <td className={`text-break ${s.section}`}>{node.label}</td>
            <td className={s.mono}>{node.namespace}</td>
            <td>
              <span className={node.type === 'namespace' ? s.crossNsChip : s.typeChip}>
                {node.type}
              </span>
            </td>
            <td><StatusCompact keys={node.statuses ?? []} /></td>
            <td><PolicyCountChips total={policyTotal} byEngine={policyCounts} /></td>
            <td onClick={(e) => e.stopPropagation()}>
              <IssueChip issues={issues} />
            </td>
            <td><MeshCell membership={meshStatus[node.id]} /></td>
            <td><LabelChips labels={node.labels} /></td>
            <td className="text-end">
              <button
                type="button"
                className="btn btn-sm btn-outline-secondary text-nowrap"
                onClick={(e) => openInGraph(node, e)}
                title="Show this workload in the graph"
              >
                ◉ Graph
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function StatusCompact({ keys }: { keys: StatusKey[] }) {
  // Blank, not a placeholder glyph — see LabelChips.
  if (keys.length === 0) return null;
  return (
    <div className="d-flex flex-column gap-1">
      {keys.map((key) => {
        const cfg = STATUS_CFG[key];
        return (
          <span key={key} className="d-inline-flex align-items-center gap-2" title={cfg.description}>
            <span
              className={fp.statusBadge}
              style={{ background: SEVERITY_COLOR[cfg.severity] }}
            >
              {cfg.symbol}
            </span>
            <span className="fs-12">{STATUS_LABELS[key]}</span>
          </span>
        );
      })}
    </div>
  );
}

// Per-engine policy count. Same branded badge as the Policies tab + an
// integer count appended — keeps engine provenance scannable when a row has
// rules from multiple engines.
function PolicyCountChips({
  total,
  byEngine,
}: {
  total: number;
  byEngine: Record<string, number>;
}) {
  if (total === 0) {
    return <span className="chip-count chip-count-deny" title="No policies touch this workload">0</span>;
  }
  return (
    <div className="d-flex flex-wrap gap-1">
      {Object.entries(byEngine).map(([engine, count]) => (
        <EngineBadge key={engine} engine={engine} suffix={`: ${count}`} />
      ))}
    </div>
  );
}

const TIER_META = [
  { tier: 'blocking' as const, color: SEVERITY_COLOR.high,    label: 'blocking' },
  { tier: 'warning'  as const, color: SEVERITY_COLOR.warning, label: 'warning' },
  { tier: 'info'     as const, color: SEVERITY_COLOR.info,    label: 'info (expected behavior)' },
];

// Per-row conflict summary. Zero = grey dash so the eye passes over clean rows;
// nonzero = a button with per-severity-tier counts (colored dots) so ten
// info-class layering rows don't read as ten fires. Danger border only when
// something actually blocks. One click still opens the single popover with
// every issue. The chevron rotates when the popover is open — that plus
// aria-expanded gives the state cue for both sighted and assistive users.
export function IssueChip({ issues }: { issues: Issue[] }) {
  const ref = useRef<HTMLButtonElement>(null);
  const [anchor, setAnchor] = useState<DOMRect | null>(null);
  // Numeric column: a dimmed 0 keeps the digit column continuous and says
  // "checked, clean" — blank here would read as "not evaluated".
  if (issues.length === 0) return <span className="text-secondary fmono tnum">0</span>;
  const counts = { blocking: 0, warning: 0, info: 0 };
  for (const issue of issues) counts[issueTier(issue.type)] += 1;
  const tiers = TIER_META.filter(({ tier }) => counts[tier] > 0);
  const hasBlocking = counts.blocking > 0;
  const summary = tiers.map(({ tier, label }) => `${counts[tier]} ${label}`).join(', ');
  const open = !!anchor;
  return (
    <>
      <button
        ref={ref}
        type="button"
        className={`btn btn-sm btn-dark border ${hasBlocking ? 'border-danger' : 'border-secondary'} py-0 px-2 d-inline-flex align-items-center gap-3 fs-11 fw-semibold leading-tight ${open ? 'active' : ''}`}
        aria-expanded={open}
        aria-haspopup="dialog"
        title={summary}
        onClick={() => setAnchor(open ? null : ref.current?.getBoundingClientRect() ?? null)}
      >
        {/* Hue carries severity. No separator glyph — mono + tabular digits have
            fixed side-bearing, so adjacent counts read as discrete tokens
            ("3 1 2", not "312"); the wider gap does the rest. */}
        {tiers.map(({ tier, color }) => (
          <span key={tier} className="fmono tnum" style={{ color }}>{counts[tier]}</span>
        ))}
        <span
          aria-hidden="true"
          className="rotate-flip fs-9"
          style={{ transform: open ? 'rotate(180deg)' : 'none' }}
        >
          ▾
        </span>
      </button>
      {anchor && (
        <IssuesPopover anchor={anchor} issues={issues} onClose={() => setAnchor(null)} />
      )}
    </>
  );
}

function MeshCell({ membership }: { membership: MeshMembership | undefined }) {
  const meta = meshBadgeMeta(membership);
  const scope: MtlsScope | 'unknown' = membership?.inMesh ? (membership.mtls?.verdict ?? 'unset') : 'unknown';
  return (
    <div className="d-flex flex-column" title={meta.tooltip}>
      {meta.providerLabel && (
        <span className="text-secondary fs-10">{meta.providerLabel}</span>
      )}
      <span className="d-inline-flex align-items-center gap-1 fs-11">
        <MtlsChip scope={scope} variant="dot" />
        {meta.long}
      </span>
    </div>
  );
}

function LabelChips({ labels }: { labels: Record<string, string> | null | undefined }) {
  const entries = Object.entries(labels ?? {});
  // Text column: render nothing. A placeholder glyph is ink with no data behind
  // it, and it lines up into a false column down a long table.
  if (entries.length === 0) return null;
  const shown = entries.slice(0, 3);
  const hidden = entries.length - shown.length;
  return (
    <div className="d-flex flex-wrap gap-1">
      {shown.map(([k, v]) => (
        <span key={k} className="chip-label">
          <span className="chip-label-key">{k}</span>
          <span className="chip-label-eq">=</span>
          <span className="chip-label-value">{v}</span>
        </span>
      ))}
      {hidden > 0 && (
        <span className="chip-count" title={entries.slice(3).map(([k, v]) => `${k}=${v}`).join(' ')}>
          +{hidden}
        </span>
      )}
    </div>
  );
}
