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

interface StatusRollupProps {
  nodes: WorkloadNode[];
  // Called after a chip toggles the filter. The status page uses this to jump
  // to a view where the filter has a visible effect — a toggled chip with no
  // on-page consequence reads as a dead click.
  onToggled?: (key: StatusKey) => void;
  // Status page hosts this inside its own Section wrapper (with its own title
  // and its own spacing). `bare` drops the strip padding/border/label so the
  // rollup renders flush inside that host without double-header or fake divider.
  bare?: boolean;
}

export default function StatusRollup({ nodes, onToggled, bare = false }: StatusRollupProps) {
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
      <div className={bare ? 'text-secondary fs-12' : 'px-3 py-2 border-bottom border-secondary text-secondary fs-12'}>
        No status signals across the current filter set.
      </div>
    );
  }

  // Bare (inside a status-page card) = shared bordered chip pattern (same as
  // IssueRollup / EngineRollup) wrapped in flex slots for 3 items per row.
  // Keeps ONE chip language across the whole page — colored border, small
  // symbol block, label, `.chip-count` numeric.
  if (bare) {
    return (
      <div className={r.gridRollup}>
        {entries.map(({ key, count }) => {
          const cfg = STATUS_CFG[key];
          const bg = SEVERITY_COLOR[cfg.severity];
          const active = selectedStatuses.has(key);
          const dimmed = selectedStatuses.size > 0 && !active;
          return (
            <div key={key} className={r.gridSlot}>
              <button
                type="button"
                className={`btn btn-sm d-inline-flex align-items-center gap-1 p-1 ${r.chip} ${active ? r.active : ''} ${dimmed ? r.dimmed : ''}`}
                onClick={() => { toggleStatus(key); onToggled?.(key); }}
                title={`${cfg.severity}: ${cfg.description} — click to ${active ? 'clear filter' : 'filter to these'}`}
                style={{ border: `1px solid ${bg}` }}
              >
                <span className={r.symbol} style={{ background: bg }}>{cfg.symbol}</span>
                <span className="text-truncate">{key}</span>
                <span className="chip-count ms-auto">{count}</span>
              </button>
            </div>
          );
        })}
      </div>
    );
  }

  // Non-bare (above the Workloads table) — keep the chip strip; filter row
  // stays a compact horizontal wrap.
  return (
    <div className="d-flex align-items-center flex-wrap gap-2 px-3 py-2 border-bottom border-secondary fs-12">
      <span className="text-secondary fs-11 text-uppercase tracking-wide">Status</span>
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
            onClick={() => { toggleStatus(key); onToggled?.(key); }}
            title={`${cfg.severity}: ${cfg.description} — click to ${active ? 'clear filter' : 'filter to these'}`}
            style={{ border: `1px solid ${bg}` }}
          >
            <span className={r.symbol} style={{ background: bg }}>{cfg.symbol}</span>
            <span>{key}</span>
            <span className="chip-count ms-1">{count}</span>
          </button>
        );
      })}
    </div>
  );
}
