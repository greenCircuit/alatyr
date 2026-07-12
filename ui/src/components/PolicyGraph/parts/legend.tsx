// Status badge + legend overlay. StatusBadge is the small chip rendered above
// each workload node carrying its derived statuses. The Legend panel maps
// colors, arrow styles, and badges to plain-English labels — also doubles as
// a status-filter affordance (clicking a badge toggles selectedStatuses).

import { useGraphStore } from '../../../store/graphStore';
import type { StatusKey } from '../../../data/policies';
import { STATUS_CFG, SEVERITY_COLOR } from '../../../data/policies';
import { COV } from './coverage';
import s from './Legend.module.css';

export interface BadgeNode { id: string; statuses: StatusKey[] }

export function StatusBadge({ s: statusKey }: { s: StatusKey }) {
  const { symbol, severity, description } = STATUS_CFG[statusKey];
  const bg = SEVERITY_COLOR[severity];
  const title = `${severity}: ${description}`;
  return (
    <span title={title} className={`${s.statusBadge} ${s.inline}`} style={{ background: bg }}>
      {symbol}
    </span>
  );
}

function Dot({ color, label }: { color: string; label: string }) {
  return (
    <span className="d-flex align-items-center gap-1">
      <span className="swatch-dot" style={{ background: color }} />
      {label}
    </span>
  );
}

export function Legend({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  const { selectedStatuses, toggleStatus } = useGraphStore();

  return (
    <div className={`position-absolute d-flex align-items-end ${s.legendAnchor}`}>
      <button
        className={`btn btn-sm btn-outline-secondary flex-shrink-0 ${s.legendToggle}`}
        onClick={onToggle}
        title={open ? 'Hide legend' : 'Show legend'}
      >
        {open ? '‹' : '›'}
      </button>

      {/* Clip wrapper — overflow:hidden lets transform slide look clean */}
      <div className={s.legendClip}>
        <div className={`px-3 py-2 rounded d-flex flex-column gap-1 ${s.legendPanel} ${open ? s.open : s.closed}`}>
          <div className={`fw-semibold mb-1 ${s.legendEyebrow}`}>
            Policy coverage
          </div>
          <Dot color={COV.workload}  label="workload policies applied" />
          <Dot color={COV.namespace} label="namespace-selector only" />
          <Dot color={COV.none}      label="no policies (gap)" />
          <div className={`mt-1 pt-1 border-top border-secondary ${s.legendEyebrow}`}>
            Arrows
          </div>
          <Dot color="#4dabf7" label="egress (allow)" />
          <Dot color="#f783ac" label="ingress (allow)" />
          <Dot color="#a9e34b" label="both (allow)" />
          <Dot color="#e03131" label="deny" />
          <div className="d-flex gap-2 mt-1">
            <span className={s.lineLabel}>── workload</span>
            <span className={s.lineLabel}>╌╌ namespace</span>
          </div>
          <div className={`mt-1 pt-1 border-top border-secondary ${s.legendEyebrow}`}>
            Badges
          </div>
          {(Object.entries(STATUS_CFG) as [StatusKey, typeof STATUS_CFG[StatusKey]][]).map(([key, cfg]) => {
            const active = selectedStatuses.has(key);
            const dimmed = selectedStatuses.size > 0 && !active;
            const bg = SEVERITY_COLOR[cfg.severity];
            const label = `${cfg.severity}: ${cfg.description}`;
            return (
              <span
                key={key}
                className="d-flex align-items-center gap-1 cursor-pointer transition-opacity"
                title={label}
                onClick={() => toggleStatus(key)}
                style={{ opacity: dimmed ? 0.4 : 1 }}
              >
                <span
                  className={`${s.statusBadge} ${active ? s.active : ''}`}
                  style={{ background: bg }}
                >
                  {cfg.symbol}
                </span>
                <span className="fs-9">{label}</span>
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
}
