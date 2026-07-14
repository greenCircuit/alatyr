// Per-namespace drill-in table, sorted worst-first (unprotected on top).
// Row click narrows the namespace filter to that ns and jumps to the graph.

import type { CovType } from '../PolicyGraph/parts/coverage';
import { COV } from '../PolicyGraph/parts/coverage';
import type { NamespaceStat } from '../../store/clusterStats';

const PROTECTION_LABEL: Record<CovType, string> = {
  workload:  'workload',
  namespace: 'ns-only',
  none:      'none',
};

interface NamespaceTableProps {
  rows:     NamespaceStat[];
  onSelect: (ns: string) => void;
}

export default function NamespaceTable({ rows, onSelect }: NamespaceTableProps) {
  if (rows.length === 0) {
    return <div className="text-secondary fs-12">No workloads in the current scope.</div>;
  }
  return (
    <table className="table table-dark table-sm table-hover mb-0 fs-12">
      <thead>
        <tr className="text-secondary text-uppercase fs-11">
          <th>Namespace</th>
          <th className="text-end">Workloads</th>
          <th>Protection</th>
          <th className="text-end">Policies</th>
          <th className="text-end">Issues</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr
            key={row.namespace}
            role="button"
            onClick={() => onSelect(row.namespace)}
            title={`Drill into ${row.namespace} — filters the graph to this namespace`}
          >
            <td>{row.namespace}</td>
            <td className="text-end">{row.workloads}</td>
            <td>
              <span aria-hidden="true" style={{ color: COV[row.protection] }}>● </span>
              {PROTECTION_LABEL[row.protection]}
            </td>
            <td className="text-end">{row.policies}</td>
            <td className="text-end">
              {row.issues > 0 ? <span className="text-danger fw-bold">{row.issues}</span> : 0}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
