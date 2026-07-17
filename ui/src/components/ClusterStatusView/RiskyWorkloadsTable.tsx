// Top-N riskiest workloads — "which pods should I look at first" list.
// Sorted by worst-severity status, then most issues, then largest fan-out.
// Ranked list closes the gap between "37 critical nodes" (aggregate) and the
// full Workloads table (2k rows) — dense enough to scan in seconds.
//
// Row layout: leading severity stripe (color = worstSeverity) → workload +
// namespace → severity word (colored + semibold, primary signal) + status
// key(s) muted → issues (tabular, red when nonzero) → edges → always-visible
// `graph →` action.

import { type CSSProperties } from 'react';
import type { WorkloadNode } from '../../data/policies';
import { STATUS_CFG, SEVERITY_COLOR } from '../../data/policies';
import type { RiskyRow } from '../../store/clusterStats';
import styles from './status-table.module.css';
import s from '../DetailPanel/DetailPanel.module.css';

interface RiskyWorkloadsTableProps {
  rows:        RiskyRow[];
  onShowNode:  (node: WorkloadNode) => void;
  onOpenGraph: (node: WorkloadNode) => void;
}

const NO_SIGNAL_COLOR = '#495057';

export default function RiskyWorkloadsTable({ rows, onShowNode, onOpenGraph }: RiskyWorkloadsTableProps) {
  if (rows.length === 0) {
    return <div className={`${s.tier1} ${s.dim} ${s.body}`}>No risky workloads in the current scope.</div>;
  }
  return (
    <table className={`table table-dark table-sm table-hover mb-0 tnum ${styles.hoverTable} ${s.smallText}`}>
      <thead>
        <tr>
          <th className={styles.stripe} />
          <th className={s.eyebrow}>Workload</th>
          <th className={s.eyebrow}>Namespace</th>
          <th className={s.eyebrow}>Worst status</th>
          <th className={`${s.eyebrow} text-end`}>Issues</th>
          <th className={`${s.eyebrow} text-end`}>Edges</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => {
          const color = row.worstSeverity === 'none' ? NO_SIGNAL_COLOR : SEVERITY_COLOR[row.worstSeverity];
          return (
            <tr
              key={row.node.id}
              onClick={() => onShowNode(row.node)}
              title={`Show ${row.node.label} details`}
            >
              <td className={styles.stripe} style={{ background: color }} title={row.worstSeverity} />
              <td className={`${s.section} ${s.mono}`}>{row.node.label}</td>
              <td className={`${s.dim} ${s.mono}`}>{row.node.namespace}</td>
              <td>
                <span className="d-inline-flex align-items-center gap-1">
                  <span
                    className={s.sevDot}
                    style={{ '--sev': color } as unknown as CSSProperties}
                    aria-hidden="true"
                  >●</span>
                  <span className={s.section}>{row.worstSeverity}</span>
                </span>
                {row.worstStatuses.length > 0 && (
                  <span className="ms-2 d-inline-flex flex-wrap gap-1">
                    {row.worstStatuses.map((key) => (
                      <span key={key} className={`${s.miniChip} ${s.miniChipDim}`} title={STATUS_CFG[key]?.description}>
                        {key}
                      </span>
                    ))}
                  </span>
                )}
              </td>
              <td className="text-end">
                {row.issueCount > 0
                  ? <span className={`${s.countChip} ${s.countChipDeny}`}>{row.issueCount}</span>
                  : <span className={s.dim}>0</span>}
              </td>
              <td className="text-end">{row.edgeCount}</td>
              <td className="text-end">
                <button
                  type="button"
                  className={s.iconButton}
                  onClick={(event) => { event.stopPropagation(); onOpenGraph(row.node); }}
                  title={`Open ${row.node.label} in the graph`}
                >→</button>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
