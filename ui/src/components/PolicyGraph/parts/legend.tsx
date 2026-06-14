// Status badge + legend overlay. StatusBadge is the small chip rendered above
// each workload node carrying its derived statuses. The Legend panel maps
// colors, arrow styles, and badges to plain-English labels — also doubles as
// a status-filter affordance (clicking a badge toggles selectedStatuses).

import { useGraphStore } from '../../../store/graphStore';
import type { StatusKey } from '../../../data/policies';
import { STATUS_CFG, SEVERITY_COLOR } from '../../../data/policies';
import { COV } from './coverage';

export interface BadgeNode { id: string; statuses: StatusKey[] }

export function StatusBadge({ s }: { s: StatusKey }) {
  const { symbol, severity, description } = STATUS_CFG[s];
  const bg = SEVERITY_COLOR[severity];
  const title = `${severity}: ${description}`;
  return (
    <span
      title={title}
      style={{
        display: 'inline-block',
        background: bg,
        color: '#fff',
        fontSize: 7,
        fontWeight: 700,
        lineHeight: 1,
        padding: '2px 3px',
        borderRadius: 3,
        letterSpacing: '0.02em',
        cursor: 'default',
        userSelect: 'none',
        whiteSpace: 'nowrap',
      }}
    >
      {symbol}
    </span>
  );
}

function Dot({ color, label }: { color: string; label: string }) {
  return (
    <span className="d-flex align-items-center gap-1">
      <span style={{ width: 10, height: 10, borderRadius: '50%', background: color, display: 'inline-block', flexShrink: 0 }} />
      {label}
    </span>
  );
}

export function Legend({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  const { selectedStatuses, toggleStatus } = useGraphStore();

  return (
    <div
      className="position-absolute d-flex align-items-end"
      style={{ bottom: 12, left: 12, zIndex: 10, gap: 6 }}
    >
      <button
        className="btn btn-sm btn-outline-secondary flex-shrink-0"
        onClick={onToggle}
        title={open ? 'Hide legend' : 'Show legend'}
        style={{ alignSelf: 'flex-end' }}
      >
        {open ? '‹' : '›'}
      </button>

      {/* Clip wrapper — overflow:hidden lets transform slide look clean */}
      <div style={{ overflow: 'hidden', maxHeight: 'calc(100% - 24px)' }}>
        <div
          className="px-3 py-2 rounded d-flex flex-column gap-1"
          style={{
            background: '#1a1e22ee', fontSize: 10, color: '#adb5bd', lineHeight: 1.6,
            maxHeight: 'calc(100vh - 80px)', overflowY: 'auto', boxSizing: 'border-box',
            transform: open ? 'translateX(0)' : 'translateX(calc(-100% - 6px))',
            transition: 'transform 0.25s ease',
          }}
        >
          <div className="fw-semibold mb-1" style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6c757d' }}>
            Policy coverage
          </div>
          <Dot color={COV.workload}  label="workload policies applied" />
          <Dot color={COV.namespace} label="namespace-selector only" />
          <Dot color={COV.none}      label="no policies (gap)" />
          <div className="mt-1 pt-1 border-top border-secondary" style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6c757d' }}>
            Arrows
          </div>
          <Dot color="#4dabf7" label="egress (allow)" />
          <Dot color="#f783ac" label="ingress (allow)" />
          <Dot color="#a9e34b" label="both (allow)" />
          <Dot color="#e03131" label="deny" />
          <div className="d-flex gap-2 mt-1">
            <span style={{ color: '#adb5bd' }}>── workload</span>
            <span style={{ color: '#adb5bd' }}>╌╌ namespace</span>
          </div>
          <div className="mt-1 pt-1 border-top border-secondary" style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6c757d' }}>
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
                className="d-flex align-items-center gap-1"
                title={label}
                onClick={() => toggleStatus(key)}
                style={{ cursor: 'pointer', opacity: dimmed ? 0.4 : 1, transition: 'opacity 0.15s' }}
              >
                <span style={{
                  background: bg, color: '#fff',
                  fontSize: 7, fontWeight: 700, padding: '2px 3px', borderRadius: 3, flexShrink: 0,
                  outline: active ? '1.5px solid #fff' : 'none',
                  outlineOffset: 1,
                }}>
                  {cfg.symbol}
                </span>
                <span style={{ fontSize: 9 }}>{label}</span>
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
}
