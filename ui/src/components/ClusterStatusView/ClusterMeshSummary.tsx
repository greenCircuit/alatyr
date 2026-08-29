// Cluster-wide mesh posture snapshot. Numbers come from /api/cluster-metrics
// (server-side, invariant to filter scope): totals at the top level, mesh
// posture nested. Compact 2-number + 1-bar layout —
// SRE feedback: 3 bars × 15 chips was scan fatigue.

import type { ClusterMetrics, MtlsScope } from '../../data/policies';
import { SEVERITY_COLOR, MTLS_COLOR } from '../../data/policies';
import { MtlsChip } from '../DetailPanel/shared/MtlsChip';
import p from './Panel.module.css';

const NEUTRAL = '#868e96';

// Skeleton mirrors the real DOM so nothing shifts on load: two stacked
// (metric %, sub-label) pairs, the mTLS bar, and a five-item legend.
function Skeleton() {
  return (
    <div className="d-flex flex-column gap-3 placeholder-glow">
      <div className="d-flex gap-4">
        {[0, 1].map((pair) => (
          <div key={pair} className="d-flex flex-column gap-1">
            <span className="placeholder" style={{ width: 56, height: 22 }} />
            <span className="placeholder" style={{ width: 120, height: 12 }} />
          </div>
        ))}
      </div>
      <span className={`placeholder ${p.mtlsBar}`} style={{ width: '100%' }} />
      <div className="d-flex flex-wrap gap-3">
        {[0, 1, 2, 3, 4].map((chip) => (
          <span key={chip} className="placeholder" style={{ width: 72, height: 12, borderRadius: 3 }} />
        ))}
      </div>
    </div>
  );
}

// mTLS legend segments — `scope` drives MtlsChip color; 'unknown' is the
// neutral degraded-PA bucket. Labels render uppercase mono via MtlsChip.
type Segment = { key: string; scope: MtlsScope | 'unknown'; count: number };

export default function ClusterMeshSummary({ metrics }: { metrics: ClusterMetrics | null }) {
  if (!metrics) return <Skeleton />;
  const mesh = metrics.meshMetrics;

  const nsPct = metrics.nsTotal === 0 ? 0 : Math.round((mesh.nsEnrolled / metrics.nsTotal) * 100);
  const wlPct = metrics.workloadTotal === 0 ? 0 : Math.round((mesh.workloadsEnrolled / metrics.workloadTotal) * 100);

  // mTLS partition across ENROLLED workloads. Server-counted `mtlsUnknown`
  // covers degraded PA fetches; the local remainder guard keeps the bar a
  // true partition even if the counters ever drift.
  const mtlsBucketed = mesh.mtlsStrict + mesh.mtlsPermissive + mesh.mtlsDisabled + mesh.mtlsUnset;
  const mtlsUnknown  = mesh.mtlsUnknown + Math.max(mesh.workloadsEnrolled - mtlsBucketed - mesh.mtlsUnknown, 0);
  const totalForBar  = mtlsBucketed + mtlsUnknown;

  const segments: Segment[] = [
    { key: 'strict',     scope: 'strict',     count: mesh.mtlsStrict },
    { key: 'permissive', scope: 'permissive', count: mesh.mtlsPermissive },
    { key: 'disable',    scope: 'disable',    count: mesh.mtlsDisabled },
    { key: 'unset',      scope: 'unset',      count: mesh.mtlsUnset },
    { key: 'unknown',    scope: 'unknown',    count: mtlsUnknown },
  ];

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex gap-4 flex-wrap">
        <div>
          <div className={`${p.metric} tnum`}>{nsPct}%</div>
          <div className="text-secondary fs-12 mt-1">
            namespaces enrolled ({mesh.nsEnrolled}/{metrics.nsTotal})
            {mesh.nsPartial > 0 && (
              <span className="ms-1" style={{ color: SEVERITY_COLOR.caution }}>
                {mesh.nsPartial} partial
              </span>
            )}
          </div>
        </div>
        <div>
          <div className={`${p.metric} tnum`}>{wlPct}%</div>
          <div className="text-secondary fs-12 mt-1">
            workloads enrolled ({mesh.workloadsEnrolled}/{metrics.workloadTotal})
          </div>
        </div>
      </div>

      <div>
        <div className="text-secondary fs-12 mb-1">
          mTLS across {mesh.workloadsEnrolled} enrolled workloads
        </div>
        <MtlsBar segments={segments} total={totalForBar} />
        <div className="d-flex flex-wrap gap-3 fs-12 mt-2">
          {segments.map((segment) => {
            const zero = segment.count === 0;
            return (
              <span key={segment.key} className="d-inline-flex align-items-center gap-1">
                <MtlsChip scope={segment.scope} variant="dot" size="xs" muted={zero} />
                <span className={`${p.verdictLabel} ${zero ? 'text-secondary' : 'text-light'}`}>{segment.scope}</span>
                <span className={`tnum text-secondary ${zero ? 'opacity-50' : ''}`}>{segment.count}</span>
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
}

// mTLS partition bar. Segment widths + colors ARE the data, so they stay
// inline; the bar frame is the shared `.mtlsBar` class.
function MtlsBar({ segments, total }: { segments: Segment[]; total: number }) {
  return (
    <div className={p.mtlsBar}>
      {segments.map((segment) => {
        const width = total === 0 ? 0 : (segment.count / total) * 100;
        if (width === 0) return null;
        const color = segment.scope === 'unknown' ? NEUTRAL : MTLS_COLOR[segment.scope];
        return (
          <div
            key={segment.key}
            style={{ width: `${width}%`, background: color }}
            title={`${segment.scope}: ${segment.count}`}
          />
        );
      })}
    </div>
  );
}
