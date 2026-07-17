// Per-namespace drill-in table, sorted worst-first (unprotected on top).
// Row click narrows the namespace filter to that ns and jumps to the graph.
//
// Row layout: leading protection stripe (green/amber/red = workload/ns-only/
// none) → namespace name + gap/stale dot-badges → workloads → policies →
// issues (tabular). Stripe carries the coverage signal so we don't need a
// separate Protection column.

import type { CovType } from '../PolicyGraph/parts/coverage';
import { COV } from '../PolicyGraph/parts/coverage';
import type { NamespaceStat } from '../../store/clusterStats';
import styles from './status-table.module.css';
import s from '../DetailPanel/DetailPanel.module.css';

const PROTECTION_HINT: Record<CovType, string> = {
  workload:  'workload-level policies present',
  namespace: 'namespace-selector policies only',
  none:      'no policies',
};

interface NamespaceTableProps {
  rows:     NamespaceStat[];
  onSelect: (ns: string) => void;
}

export default function NamespaceTable({ rows, onSelect }: NamespaceTableProps) {
  if (rows.length === 0) {
    return <div className={`${s.tier1} ${s.dim} ${s.body}`}>No workloads in the current scope.</div>;
  }
  return (
    <table className={`table table-dark table-sm table-hover mb-0 tnum ${styles.hoverTable} ${s.smallText}`}>
      <thead>
        <tr>
          <th className={styles.stripe} />
          <th className={s.eyebrow}>Namespace</th>
          <th className={`${s.eyebrow} text-end`}>Workloads</th>
          <th className={`${s.eyebrow} text-end`}>Policies</th>
          <th className={`${s.eyebrow} text-end`}>Issues</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr
            key={row.namespace}
            onClick={() => onSelect(row.namespace)}
            title={`Drill into ${row.namespace} — filters the graph to this namespace`}
          >
            <td
              className={styles.stripe}
              style={{ background: COV[row.protection] }}
              title={PROTECTION_HINT[row.protection]}
            />
            <td>
              <span className={`${s.section} ${s.mono}`}>{row.namespace}</span>
              {row.gap && (
                <span
                  className={`${s.reasonChip} ${s.reasonDeny} ms-2`}
                  title="Internet-exposed workloads here but no policy covers anything"
                >gap</span>
              )}
              {row.stale && (
                <span
                  className={`${s.reasonChip} ${s.reasonWarn} ms-2`}
                  title="Policies exist here but no workloads — likely stale config"
                >stale</span>
              )}
            </td>
            <td className="text-end">{row.workloads}</td>
            <td className="text-end">{row.policies}</td>
            <td className="text-end">
              {row.issues > 0
                ? <span className={`${s.countChip} ${s.countChipDeny}`}>{row.issues}</span>
                : <span className={s.dim}>0</span>}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
