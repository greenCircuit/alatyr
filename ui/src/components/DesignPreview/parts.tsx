// V2 primitives for the design preview: bars, tables, section header, zone
// divider. Throwaway — port pieces into real components once shape lands.

import { SEVERITY_COLOR } from '../../data/policies';
import type { RiskyRow, NamespaceRow, SeveritySegment } from './mockData';

// ────────────────────────────────────────────────────────────────────────────
// Section header: quieter eyebrow; optional trailing link is icon-only chevron
// instead of "Open in tables →" chrome. Uniform across sections *within* a zone.

export function SectionHeader({ title, hint, onLink }: {
  title: string; hint?: string; onLink?: () => void;
}) {
  return (
    <div className="d-flex align-items-baseline justify-content-between">
      <div className="d-flex align-items-baseline gap-2">
        <span className="text-light fs-13 fw-semibold">{title}</span>
        {hint && <span className="text-secondary fs-12">{hint}</span>}
      </div>
      {onLink && (
        <button
          type="button"
          className="btn btn-sm btn-link p-0 fs-12 text-secondary"
          onClick={onLink}
        >Open in tables →</button>
      )}
    </div>
  );
}

// Zone divider: a bigger visual break separating groups of related sections.
// Gives the page rhythm — Posture / Coverage / Drilldowns.

export function ZoneDivider({ label }: { label: string }) {
  return (
    <div className="d-flex align-items-center gap-3">
      <span className="text-secondary text-uppercase fs-11 tracking-wider fw-bold">{label}</span>
      <span className="flex-grow-1 border-top border-secondary opacity-25" />
    </div>
  );
}

// ────────────────────────────────────────────────────────────────────────────
// Bars — thicker, gapped segments so adjacent colors don't blend.

interface Segment { key: string; count: number; color: string; }

export function ProportionBarV2({ segments, total: totalOverride, height = 14 }: {
  segments: Segment[]; total?: number; height?: number;
}) {
  const total = totalOverride ?? segments.reduce((sum, seg) => sum + seg.count, 0);
  if (total === 0) return null;
  return (
    <div className="d-flex w-100 overflow-hidden" style={{ height, gap: 1, borderRadius: 3 }}>
      {segments.map((seg) => {
        if (seg.count === 0) return null;
        const width = (seg.count / total) * 100;
        return (
          <div
            key={seg.key}
            title={`${seg.key}: ${seg.count}`}
            style={{ width: `${width}%`, background: seg.color, minWidth: 2 }}
          />
        );
      })}
    </div>
  );
}

// Legend chips — zero-count entries stay visible with muted styling. "We
// checked, it's zero" is the posture signal; hiding suggests unchecked.
export function LegendChips({ segments, hints }: {
  segments: Segment[];
  hints?: Record<string, string>;
}) {
  return (
    <div className="d-flex align-items-center flex-wrap gap-3 fs-12">
      {segments.map((seg) => {
        const zero = seg.count === 0;
        return (
          <span
            key={seg.key}
            className="d-inline-flex align-items-center gap-1"
            title={hints?.[seg.key]}
          >
            <span
              className="swatch-dot"
              style={{ background: seg.color, opacity: zero ? 0.4 : 1 }}
            />
            <span className={zero ? 'text-secondary' : 'text-light'}>{seg.key}</span>
            <span className={`tnum ${zero ? 'text-secondary opacity-50' : 'text-secondary'}`}>{seg.count}</span>
          </span>
        );
      })}
    </div>
  );
}

// ────────────────────────────────────────────────────────────────────────────
// Risky workloads — leading severity swatch column, severity word carries
// color + weight, hover-only action, tabular numerals, zero-issue muted.

import styles from './parts.module.css';

export function RiskyTableV2({ rows }: { rows: RiskyRow[] }) {
  if (rows.length === 0) return <EmptyRow msg="No risky workloads in the current scope." />;
  return (
    <table className={`table table-dark table-sm mb-0 fs-12 tnum ${styles.hoverTable}`}>
      <thead>
        <tr className="text-secondary text-uppercase fs-11">
          <th style={{ width: 8 }} />
          <th>Workload</th>
          <th>Namespace</th>
          <th>Worst status</th>
          <th className="text-end">Issues</th>
          <th style={{ width: 60 }} />
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => {
          const color = SEVERITY_COLOR[row.worstTier];
          return (
            <tr key={row.id} className={styles.row}>
              <td style={{ background: color, padding: 0 }} title={row.worstTier} />
              <td className="fw-semibold text-light">{row.workload}</td>
              <td className="text-secondary">{row.namespace}</td>
              <td>
                <span className="fw-semibold" style={{ color }}>{row.worstTier}</span>
                <span className="text-secondary ms-2">{row.worstLabel}</span>
              </td>
              <td className="text-end">
                {row.issueCount > 0
                  ? <span className="text-danger fw-semibold">{row.issueCount}</span>
                  : <span className="text-secondary opacity-50">0</span>}
              </td>
              <td className="text-end">
                <button
                  type="button"
                  className="btn btn-sm btn-link p-0 fs-12 text-secondary"
                  onClick={(event) => event.stopPropagation()}
                  title={`Open ${row.workload} in the graph`}
                >graph →</button>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

// ────────────────────────────────────────────────────────────────────────────
// Namespace table — leading coverage stripe cell, single dot-badge for
// gap/stale (never two solid Bootstrap badges), namespace name is identity.

export function NamespaceTableV2({ rows }: { rows: NamespaceRow[] }) {
  if (rows.length === 0) return <EmptyRow msg="No workloads in the current scope." />;
  return (
    <table className={`table table-dark table-sm mb-0 fs-12 tnum ${styles.hoverTable}`}>
      <thead>
        <tr className="text-secondary text-uppercase fs-11">
          <th style={{ width: 8 }} />
          <th>Namespace</th>
          <th className="text-end">Workloads</th>
          <th className="text-end">Policies</th>
          <th className="text-end">Coverage</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => {
          const stripe = row.worst === 'none' ? '#495057' : SEVERITY_COLOR[row.worst];
          return (
            <tr key={row.namespace} className={styles.row}>
              <td style={{ background: stripe, padding: 0 }} title={row.worst} />
              <td>
                <span className="fw-semibold text-light">{row.namespace}</span>
                {row.hasGap && (
                  <span
                    className="d-inline-flex align-items-center gap-1 ms-2 fs-11"
                    title="Internet-exposed workloads here but no policy covers anything"
                  >
                    <span className="swatch-dot" style={{ background: SEVERITY_COLOR.high }} />
                    <span style={{ color: SEVERITY_COLOR.high }}>gap</span>
                  </span>
                )}
                {row.hasStale && (
                  <span
                    className="d-inline-flex align-items-center gap-1 ms-2 fs-11"
                    title="Policies exist here but no workloads — likely stale config"
                  >
                    <span className="swatch-dot" style={{ background: SEVERITY_COLOR.warning }} />
                    <span style={{ color: SEVERITY_COLOR.warning }}>stale</span>
                  </span>
                )}
              </td>
              <td className="text-end">{row.workloads}</td>
              <td className="text-end">{row.policies}</td>
              <td className="text-end fw-semibold" style={{ color: coverageColor(row.coverage) }}>
                {row.coverage}%
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

// Coverage % → color band. Green ≥75, yellow 40-74, red <40. Threshold picked
// to match how an SRE reads posture: below 40% = "unprotected", above 75% =
// "mostly locked", middle = "attention needed".
function coverageColor(pct: number): string {
  if (pct >= 75) return SEVERITY_COLOR.secure;
  if (pct >= 40) return SEVERITY_COLOR.caution;
  return SEVERITY_COLOR.high;
}

// ────────────────────────────────────────────────────────────────────────────
// Standardized empty row — same treatment across every section.

export function EmptyRow({ msg }: { msg: string }) {
  return <div className="text-secondary fs-12 fst-italic py-1">{msg}</div>;
}

// ────────────────────────────────────────────────────────────────────────────
// Small chip rows for placeholder rollups (engines / issues / statuses).
// Real page pulls these from EngineRollup/IssueRollup/StatusRollup.

export function ChipRow({ items }: { items: Array<{ key: string; count: number; color: string; hint?: string }> }) {
  return (
    <div className="d-flex flex-wrap gap-2">
      {items.map((item) => (
        <span
          key={item.key}
          className="d-inline-flex align-items-center gap-2 px-2 py-1 rounded border border-secondary"
          title={item.hint}
          style={{ background: '#1a1d20' }}
        >
          <span className="swatch-dot" style={{ background: item.color }} />
          <span className="text-light fs-12">{item.key}</span>
          <span className="text-secondary fs-12 tnum">{item.count}</span>
        </span>
      ))}
    </div>
  );
}

// Convenience re-export shape for severity segments used by preview.
export type { SeveritySegment };
