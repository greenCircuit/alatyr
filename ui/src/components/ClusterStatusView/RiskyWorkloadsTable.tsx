// Top-N riskiest workloads — the "which pods should I look at first" list.
// Sorted by worst-severity status, then most issues, then largest fan-out.
// Ranked list closes the gap between "37 critical nodes" (aggregate) and the
// full Workloads table (2k rows) — dense enough to scan in seconds.

import type { WorkloadNode } from '../../data/policies';
import { STATUS_CFG, SEVERITY_COLOR } from '../../data/policies';
import type { RiskyRow } from '../../store/clusterStats';

interface RiskyWorkloadsTableProps {
  rows:        RiskyRow[];
  onShowNode:  (node: WorkloadNode) => void;
  onOpenGraph: (node: WorkloadNode) => void;
}

export default function RiskyWorkloadsTable({ rows, onShowNode, onOpenGraph }: RiskyWorkloadsTableProps) {
  if (rows.length === 0) {
    return <div className="text-secondary fs-12">No risky workloads in the current scope.</div>;
  }
  return (
    <table className="table table-dark table-sm table-hover mb-0 fs-12">
      <thead>
        <tr className="text-secondary text-uppercase fs-11">
          <th>Workload</th>
          <th>Namespace</th>
          <th>Worst status</th>
          <th className="text-end">Issues</th>
          <th className="text-end">Edges</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => {
          const color = row.worstSeverity === 'none' ? '#495057' : SEVERITY_COLOR[row.worstSeverity];
          return (
            <tr
              key={row.node.id}
              role="button"
              className="cursor-pointer"
              onClick={() => onShowNode(row.node)}
              title={`Show ${row.node.label} details`}
            >
              <td className="fw-semibold">{row.node.label}</td>
              <td>{row.node.namespace}</td>
              <td>
                <span aria-hidden="true" style={{ color }}>● </span>
                <span className="text-secondary">{row.worstSeverity}</span>
                {row.worstStatuses.length > 0 && (
                  <span className="ms-2">
                    {row.worstStatuses.map((key) => (
                      <span key={key} className="badge bg-secondary me-1" title={STATUS_CFG[key]?.description}>
                        {key}
                      </span>
                    ))}
                  </span>
                )}
              </td>
              <td className="text-end">
                {row.issueCount > 0 ? <span className="text-danger fw-bold">{row.issueCount}</span> : 0}
              </td>
              <td className="text-end">{row.edgeCount}</td>
              <td className="text-end">
                <button
                  type="button"
                  className="btn btn-sm btn-dark border border-secondary text-light py-0 px-2 fs-12"
                  onClick={(event) => { event.stopPropagation(); onOpenGraph(row.node); }}
                  title={`Open ${row.node.label} in the graph`}
                >graph →</button>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
