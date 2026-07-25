// Status badges shown on the workload card + per-engine breakdown. Single
// source of truth for color/symbol presentation alongside on-node badges in
// PolicyGraph.

import type { StatusKey } from '../../../data/policies';
import { STATUS_CFG, SEVERITY_COLOR } from '../../../data/policies';
import s from '../DetailPanel.module.css';

export function StatusBadges({ keys }: { keys: StatusKey[] }) {
  if (keys.length === 0) return null;
  return (
    <div className="d-flex flex-wrap gap-1">
      {keys.map((key) => {
        const cfg = STATUS_CFG[key];
        const bg = SEVERITY_COLOR[cfg.severity];
        const title = `${cfg.severity}: ${cfg.description}`;
        return (
          <span
            key={key}
            title={title}
            className={`${s.statusBadge}`}
            style={{ background: bg }}
          >
            <span className={s.statusBadgeSymbol}>{cfg.symbol}</span>
            <span>{key}</span>
          </span>
        );
      })}
    </div>
  );
}
