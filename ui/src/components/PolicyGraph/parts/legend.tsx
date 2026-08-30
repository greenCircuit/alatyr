// Status badge + legend overlay. StatusBadge is the small chip rendered above
// each workload node carrying its derived statuses. The Legend panel maps
// colors, arrow styles, and badges to plain-English labels — also doubles as
// a status-filter affordance (clicking a badge toggles selectedStatuses).

import { useGraphStore } from '../../../store/graphStore';
import type { MeshFilterValue } from '../../../store/filters';
import type { StatusKey, MtlsScope } from '../../../data/policies';
import { STATUS_CFG, SEVERITY_COLOR, MTLS_COLOR } from '../../../data/policies';
import { COV } from './coverage';
import { GRAPH_TOKENS } from '../../../style/graphTokens';
import s from './Legend.module.css';
import g from '../PolicyGraph.module.css';

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

// Mesh vocabulary for the legend. Order mirrors the graph chip precedence:
// filled in-mesh states (strict → permissive → disable → unset) followed by
// the outlined out-of-mesh chip. Copy is one-line operator-speak, verdict
// name up front so it doubles as the filter chip label.
const MESH_LEGEND: {
  filter:  MeshFilterValue;
  short:   string;
  scope?:  MtlsScope;         // set for filled variants
  outline?: boolean;           // out-of-mesh
  desc:    string;
}[] = [
  { filter: 'mtls-strict',     short: 'mTLS',    scope: 'strict',     desc: 'strict — mTLS required for peer traffic' },
  { filter: 'mtls-permissive', short: 'PERM',    scope: 'permissive', desc: 'permissive — mTLS accepted, plaintext also allowed' },
  { filter: 'mtls-disable',    short: 'PLAIN',   scope: 'disable',    desc: 'disable — plaintext only' },
  { filter: 'mtls-unset',      short: 'mesh?',   scope: 'unset',      desc: 'unset — no PA in scope, inherits mesh default' },
  { filter: 'out-of-mesh',     short: 'no-mesh', outline: true,       desc: 'not enrolled in the mesh dataplane' },
];

// Node silhouettes — mirror the cytoscape shapes in styles.ts. Points are in a
// 20x14 viewBox so every glyph lines up on the same baseline.
const SHAPE_LEGEND: { label: string; points?: string; ellipse?: boolean }[] = [
  { label: 'Deployment' },
  { label: 'StatefulSet', points: '3,0 17,0 20,3 20,11 17,14 3,14 0,11 0,3' },
  { label: 'DaemonSet',   points: '10,0 20,5 16,14 4,14 0,5' },
  { label: 'CronJob',     points: '5,0 15,0 20,7 15,14 5,14 0,7' },
  { label: 'Job',         points: '5,0 15,0 20,7 15,14 5,14 0,7 4,7' },
  { label: 'Pod',         ellipse: true },
  { label: 'CIDR range',  points: '2,0 18,0 20,7 18,14 2,14 0,7' },
];

function ShapeMark({ shape }: { shape: typeof SHAPE_LEGEND[number] }) {
  const fill = 'none';
  const stroke = '#adb5bd';
  return (
    <svg width="20" height="14" viewBox="0 0 20 14" aria-hidden="true">
      {shape.ellipse
        ? <ellipse cx="10" cy="7" rx="9.5" ry="6.5" fill={fill} stroke={stroke} />
        : shape.points
          ? <polygon points={shape.points} fill={fill} stroke={stroke} />
          : <rect x="0.5" y="0.5" width="19" height="13" fill={fill} stroke={stroke} />}
    </svg>
  );
}

export function Legend({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  const { selectedStatuses, toggleStatus, selectedMeshFilters, toggleMeshFilter } = useGraphStore();

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
          <Dot color={GRAPH_TOKENS.except} label="except (allow with carve-out)" />
          <Dot color="#e03131" label="deny" />
          <div className="d-flex gap-2 mt-1">
            <span className={s.lineLabel}>── workload</span>
            <span className={s.lineLabel}>╌╌ namespace</span>
          </div>
          <div className={`mt-1 pt-1 border-top border-secondary ${s.legendEyebrow}`}>
            Status badges
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

          <div className={`mt-1 pt-1 border-top border-secondary ${s.legendEyebrow}`}>
            Node shapes
          </div>
          {SHAPE_LEGEND.map((shape) => (
            <span key={shape.label} className="d-flex align-items-center gap-1">
              <ShapeMark shape={shape} />
              <span className="fs-9">{shape.label}</span>
            </span>
          ))}

          <div className={`mt-1 pt-1 border-top border-secondary ${s.legendEyebrow}`}>
            Mesh (mTLS)
          </div>
          {MESH_LEGEND.map((row) => {
            const active = selectedMeshFilters.has(row.filter);
            const dimmed = selectedMeshFilters.size > 0 && !active;
            const outClass    = row.outline ? g.meshMarkOut    : '';
            const activeClass = active      ? g.meshMarkActive : '';
            const stripe      = row.scope ? { borderLeftColor: MTLS_COLOR[row.scope] } : undefined;
            return (
              <span
                key={row.filter}
                className="d-flex align-items-center gap-1 cursor-pointer transition-opacity"
                title={row.desc}
                onClick={() => toggleMeshFilter(row.filter)}
                style={{ opacity: dimmed ? 0.4 : 1 }}
              >
                <span
                  className={`${g.meshMark} ${outClass} ${activeClass} ${s.meshLegendChip}`}
                  style={stripe}
                >
                  {row.short}
                </span>
                <span className="fs-9">{row.desc}</span>
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
}
