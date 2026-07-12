// Status rollup strip above the Workloads table. Live count per status key
// over the currently-filtered workload set; clicking a chip toggles that key
// in `selectedStatuses`. Same toggle behavior as the legend on the graph —
// audit mode reuses the existing status-filter machinery so the two views
// stay in sync.

import { useMemo } from 'react';
import type { WorkloadNode, StatusKey } from '../../data/policies';
import { STATUS_CFG, SEVERITY_COLOR } from '../../data/policies';
import { useGraphStore } from '../../store/graphStore';
import r from './Rollup.module.css';

export default function StatusRollup({ nodes }: { nodes: WorkloadNode[] }) {
  const selectedStatuses = useGraphStore((s) => s.selectedStatuses);
  const toggleStatus = useGraphStore((s) => s.toggleStatus);

  const counts = useMemo(() => {
    const out = new Map<StatusKey, number>();
    for (const n of nodes) {
      for (const k of n.statuses ?? []) {
        out.set(k, (out.get(k) ?? 0) + 1);
      }
    }
    return out;
  }, [nodes]);

  // Preserve STATUS_CFG insertion order to match the legend; drop zero-count
  // keys so the row doesn't fill with noise on small clusters.
  const entries = (Object.keys(STATUS_CFG) as StatusKey[])
    .map((key) => ({ key, count: counts.get(key) ?? 0 }))
    .filter((e) => e.count > 0);

  if (entries.length === 0) {
    return (
      <div className="px-3 py-2 border-bottom border-secondary text-secondary fs-12">
        No status signals across the current filter set.
      </div>
    );
  }

  return (
    <div className="d-flex align-items-center flex-wrap gap-2 px-3 py-2 border-bottom border-secondary fs-12">
      <span className="text-secondary fs-11 text-uppercase tracking-wide">
        Status
      </span>
      {entries.map(({ key, count }) => {
        const cfg = STATUS_CFG[key];
        const bg = SEVERITY_COLOR[cfg.severity];
        const active = selectedStatuses.has(key);
        const dimmed = selectedStatuses.size > 0 && !active;
        return (
          <button
            key={key}
            type="button"
            className={`btn btn-sm d-inline-flex align-items-center gap-1 p-1 ${r.chip} ${active ? r.active : ''} ${dimmed ? r.dimmed : ''}`}
            onClick={() => toggleStatus(key)}
            title={`${cfg.severity}: ${cfg.description} — click to ${active ? 'clear filter' : 'filter to these'}`}
            style={{ border: `1px solid ${bg}` }}
          >
            <span className={r.symbol} style={{ background: bg }}>
              {cfg.symbol}
            </span>
            <span>{key}</span>
            <span className="badge bg-secondary ms-1">{count}</span>
          </button>
        );
      })}
    </div>
  );
}
