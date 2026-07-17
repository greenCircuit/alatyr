// Policy rows for audit. Each PolicyEdge from /api/graph is one rule produced
// by a policy; multiple rules from the same policy collapse into one row here
// — operators ask about policies, not rules. Row click pins all underlying
// edges as the selection so the graph panel can show every rule the policy
// emits.

import { useMemo, useState } from 'react';
import type { PolicyEdge, Issue } from '../../data/policies';
import { useGraphStore } from '../../store/graphStore';
import { EngineBadge } from '../../data/engineIcons';
import { DirectionBadge } from '../DetailPanel/shared/badges';
import { ManifestButton } from '../DetailPanel/shared/ManifestModal';
import { SortHeader, type SortState, nextSort } from './SortHeader';
import { IssueChip } from './WorkloadsTable';
import { policyIssueKey, type IssueIndex } from '../../store/issueIndex';
import s from '../DetailPanel/DetailPanel.module.css';

type Col = 'name' | 'engine' | 'namespace' | 'action' | 'direction' | 'rules' | 'reach' | 'issues';

// Ingress before egress before 'both' — matches DirectionDropdown order and
// the badge order shown in PolicyHeader.
const DIRECTION_ORDER = ['ingress', 'egress', 'both'];

interface PolicyRow {
  key: string;
  name: string;
  namespace: string;
  engine: string;
  action: number; // 0 allow / 1 deny
  directions: string[]; // unique, ordered per DIRECTION_ORDER
  edges: PolicyEdge[];
  srcCount: number;
  dstCount: number;
  issues: Issue[];
}

export default function PoliciesTable({ edges, policyIssues }: { edges: PolicyEdge[]; policyIssues: IssueIndex }) {
  const setSelectedEdges = useGraphStore((s) => s.setSelectedEdges);
  const setView          = useGraphStore((s) => s.setView);
  const [sort, setSort] = useState<SortState<Col>>({ col: null, dir: 'asc' });

  const rows = useMemo(() => {
    const grouped = new Map<string, PolicyRow>();
    for (const e of edges) {
      const key = `${e.policySource}|${e.namespace}|${e.policyName}|${e.action ?? 0}`;
      let row = grouped.get(key);
      if (!row) {
        row = {
          key,
          name: e.policyName,
          namespace: e.namespace,
          engine: e.policySource,
          action: e.action ?? 0,
          directions: [],
          edges: [],
          srcCount: 0,
          dstCount: 0,
          issues: policyIssues.get(policyIssueKey(e.policySource, e.namespace, e.policyName)) ?? [],
        };
        grouped.set(key, row);
      }
      row.edges.push(e);
    }
    for (const row of grouped.values()) {
      const srcs = new Set<string>();
      const dsts = new Set<string>();
      const dirs = new Set<string>();
      for (const e of row.edges) {
        srcs.add(e.source);
        dsts.add(e.target);
        dirs.add(e.direction);
      }
      row.srcCount = srcs.size;
      row.dstCount = dsts.size;
      row.directions = DIRECTION_ORDER.filter((d) => dirs.has(d));
    }
    const list = [...grouped.values()];
    if (!sort.col) return list;
    const sign = sort.dir === 'asc' ? 1 : -1;
    list.sort((a, b) => {
      switch (sort.col) {
        case 'name':      return sign * a.name.localeCompare(b.name);
        case 'engine':    return sign * a.engine.localeCompare(b.engine);
        case 'namespace': return sign * a.namespace.localeCompare(b.namespace);
        case 'action':    return sign * (a.action - b.action);
        case 'direction': return sign * a.directions.join(',').localeCompare(b.directions.join(','));
        case 'rules':     return sign * (a.edges.length - b.edges.length);
        case 'reach':     return sign * ((a.srcCount + a.dstCount) - (b.srcCount + b.dstCount));
        case 'issues':    return sign * (a.issues.length - b.issues.length);
        default:          return 0;
      }
    });
    return list;
  }, [edges, sort, policyIssues]);

  const onSort = (col: Col) => setSort((s) => nextSort(s, col));

  const selectRow = (row: PolicyRow) => setSelectedEdges(row.edges);
  const openInGraph = (row: PolicyRow, e: React.MouseEvent) => {
    e.stopPropagation();
    setSelectedEdges(row.edges);
    setView('graph');
  };

  if (rows.length === 0) {
    return <div className={`${s.tier1} ${s.dim} ${s.body}`}>No policies match the current filters.</div>;
  }

  return (
    <table className="table table-dark table-sm table-hover mb-0 fs-13">
      <thead className="sticky-top bg-dark">
        <tr>
          <SortHeader col="name"      label="Policy"     sort={sort} onSort={onSort} />
          <SortHeader col="engine"    label="Engine"     sort={sort} onSort={onSort} />
          <SortHeader col="namespace" label="Namespace"  sort={sort} onSort={onSort} />
          <SortHeader col="action"    label="Action"     sort={sort} onSort={onSort} />
          <SortHeader col="direction" label="Direction"  sort={sort} onSort={onSort} />
          <SortHeader col="rules"     label="Rules"      sort={sort} onSort={onSort} />
          <SortHeader col="reach"     label="Endpoints"  sort={sort} onSort={onSort} />
          <SortHeader col="issues"    label="Issues"     sort={sort} onSort={onSort} />
          <th />
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr
            key={row.key}
            role="button"
            onClick={() => selectRow(row)}
            className="cursor-pointer"
          >
            <td className="text-break fw-semibold">{row.name}</td>
            <td><EngineBadge engine={row.engine} /></td>
            <td>{row.namespace || '—'}</td>
            <td>
              <span className="chip-verdict" style={{ background: row.action === 1 ? 'var(--color-deny)' : 'var(--color-allow)' }}>
                {row.action === 1 ? 'deny' : 'allow'}
              </span>
            </td>
            <td>
              <div className="d-flex flex-wrap gap-1">
                {row.directions.map((d) => <DirectionBadge key={d} direction={d} />)}
              </div>
            </td>
            <td><span className="chip-count">{row.edges.length}</span></td>
            <td>
              <span className="text-secondary">src </span>
              <span className="chip-count chip-count-src">{row.srcCount}</span>
              <span className="text-secondary ms-2">dst </span>
              <span className="chip-count chip-count-dst">{row.dstCount}</span>
            </td>
            <td onClick={(e) => e.stopPropagation()}>
              <IssueChip issues={row.issues} />
            </td>
            <td className="text-end">
              <div className="d-flex gap-1 justify-content-end align-items-center">
                {/* stop row-select fire from the YAML modal trigger */}
                <span onClick={(e) => e.stopPropagation()}>
                  <ManifestButton kind={row.engine} namespace={row.namespace} name={row.name} />
                </span>
                <button
                  type="button"
                  className="btn btn-sm btn-outline-secondary text-nowrap"
                  onClick={(e) => openInGraph(row, e)}
                  title="Show this policy in the graph"
                >
                  ◉ Graph
                </button>
              </div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

