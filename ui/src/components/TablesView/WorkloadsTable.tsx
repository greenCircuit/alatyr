// Workload rows for the audit view. Columns picked for what SREs ask in incidents:
// "which workloads have internet ingress", "what's in the broken ns", "who has
// no policy at all". Row click drops back to the graph with the workload selected.

import { useMemo, useState } from 'react';
import type { WorkloadNode, PolicyEdge, StatusKey } from '../../data/policies';
import { useGraphStore } from '../../store/graphStore';
import { engineMeta } from '../../data/engines';
import { EngineLogo } from '../../data/engineIcons';
import { StatusBadge } from '../PolicyGraph/parts/legend';
import { SortHeader, type SortState, nextSort } from './SortHeader';

type Col = 'name' | 'namespace' | 'type' | 'statuses' | 'policies';

export default function WorkloadsTable({
  nodes,
  edges,
}: {
  nodes: WorkloadNode[];
  edges: PolicyEdge[];
}) {
  const setSelectedNode = useGraphStore((s) => s.setSelectedNode);
  const setView = useGraphStore((s) => s.setView);
  const [sort, setSort] = useState<SortState<Col>>({ col: null, dir: 'asc' });

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
        default:          return 0;
      }
    });
    return list;
  }, [nodes, policyCountByNode, sort]);

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
    return <div className="text-secondary p-3">No workloads match the current filters.</div>;
  }

  return (
    <table className="table table-dark table-sm table-hover mb-0" style={{ fontSize: 13 }}>
      <thead className="sticky-top bg-dark">
        <tr>
          <SortHeader col="name"      label="Workload"   sort={sort} onSort={onSort} />
          <SortHeader col="namespace" label="Namespace"  sort={sort} onSort={onSort} />
          <SortHeader col="type"      label="Type"       sort={sort} onSort={onSort} />
          <SortHeader col="statuses"  label="Status"     sort={sort} onSort={onSort} />
          <SortHeader col="policies"  label="Policies"   sort={sort} onSort={onSort} />
          <th style={{ whiteSpace: 'nowrap' }}>Labels</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {rows.map(({ node, policyCounts, policyTotal }) => (
          <tr
            key={node.id}
            role="button"
            onClick={() => selectRow(node)}
            style={{ cursor: 'pointer' }}
          >
            <td className="text-break fw-semibold">{node.label}</td>
            <td>{node.namespace || '—'}</td>
            <td>
              <span
                className={`badge ${node.type === 'namespace' ? 'bg-warning text-dark' : 'bg-secondary'}`}
              >
                {node.type}
              </span>
            </td>
            <td><StatusCompact keys={node.statuses ?? []} /></td>
            <td><PolicyCountChips total={policyTotal} byEngine={policyCounts} /></td>
            <td><LabelChips labels={node.labels} /></td>
            <td className="text-end">
              <button
                type="button"
                className="btn btn-sm btn-outline-secondary"
                onClick={(e) => openInGraph(node, e)}
                title="Show this workload in the graph"
                style={{ whiteSpace: 'nowrap' }}
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
  if (keys.length === 0) return <span className="text-secondary">—</span>;
  return (
    <div className="d-flex flex-wrap gap-1">
      {keys.map((key) => <StatusBadge key={key} s={key} />)}
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
    return <span className="badge bg-danger" title="No policies touch this workload">0</span>;
  }
  return (
    <div className="d-flex flex-wrap gap-1">
      {Object.entries(byEngine).map(([engine, count]) => {
        const { label, color } = engineMeta(engine);
        return (
          <span
            key={engine}
            className="badge d-inline-flex align-items-center gap-1"
            title={label}
            style={{ background: '#11151a', border: `1px solid ${color}`, color: '#e9ecef' }}
          >
            <EngineLogo engine={engine} size={12} /> {engine}: {count}
          </span>
        );
      })}
    </div>
  );
}

function LabelChips({ labels }: { labels: Record<string, string> | null | undefined }) {
  const entries = Object.entries(labels ?? {});
  if (entries.length === 0) return <span className="text-secondary">—</span>;
  const shown = entries.slice(0, 3);
  const hidden = entries.length - shown.length;
  return (
    <div className="d-flex flex-wrap gap-1">
      {shown.map(([k, v]) => (
        <span key={k} className="badge bg-secondary" style={{ fontWeight: 'normal' }}>
          {k}={v}
        </span>
      ))}
      {hidden > 0 && (
        <span className="badge bg-secondary" title={entries.slice(3).map(([k, v]) => `${k}=${v}`).join(' ')}>
          +{hidden}
        </span>
      )}
    </div>
  );
}
