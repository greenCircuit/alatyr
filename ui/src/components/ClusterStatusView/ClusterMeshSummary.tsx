// Cluster-wide mesh posture snapshot. Numbers come from /api/cluster-metrics
// (server-side, invariant to filter scope): totals at the top level, mesh
// posture nested. Compact 2-number + 1-bar layout —
// SRE feedback: 3 bars × 15 chips was scan fatigue.

import type { ClusterMetrics } from '../../data/policies';
import { meshBadgeMeta, SEVERITY_COLOR } from '../../data/policies';

const NEUTRAL = '#868e96';

function Skeleton() {
  return (
    <div className="d-flex flex-column gap-2 placeholder-glow">
      <div className="d-flex gap-4">
        <span className="placeholder" style={{ width: 80, height: 24 }} />
        <span className="placeholder" style={{ width: 80, height: 24 }} />
      </div>
      <span className="placeholder" style={{ height: 10, borderRadius: 3 }} />
      <div className="d-flex gap-3">
        {[0, 1, 2, 3].map((chip) => (
          <span key={chip} className="placeholder" style={{ width: 60, height: 10, borderRadius: 3 }} />
        ))}
      </div>
    </div>
  );
}

export default function ClusterMeshSummary({ metrics }: { metrics: ClusterMetrics | null }) {
  if (!metrics) return <Skeleton />;
  const mesh = metrics.meshMetrics;

  const strictMeta     = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'strict' } });
  const permissiveMeta = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'permissive' } });
  const disableMeta    = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'disable' } });
  const unsetMeta      = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'unset' } });

  const nsPct = metrics.nsTotal === 0 ? 0 : Math.round((mesh.nsEnrolled / metrics.nsTotal) * 100);
  const wlPct = metrics.workloadTotal === 0 ? 0 : Math.round((mesh.workloadsEnrolled / metrics.workloadTotal) * 100);

  // mTLS partition across ENROLLED workloads. Server-counted `mtlsUnknown`
  // covers degraded PA fetches; the local remainder guard keeps the bar a
  // true partition even if the counters ever drift.
  const mtlsBucketed = mesh.mtlsStrict + mesh.mtlsPermissive + mesh.mtlsDisabled + mesh.mtlsUnset;
  const mtlsUnknown  = mesh.mtlsUnknown + Math.max(mesh.workloadsEnrolled - mtlsBucketed - mesh.mtlsUnknown, 0);
  const totalForBar  = mtlsBucketed + mtlsUnknown;

  const segments = [
    { key: 'strict',     count: mesh.mtlsStrict,     color: strictMeta.color,     label: 'STRICT' },
    { key: 'permissive', count: mesh.mtlsPermissive, color: permissiveMeta.color, label: 'permissive' },
    { key: 'disable',    count: mesh.mtlsDisabled,   color: disableMeta.color,    label: 'DISABLE' },
    { key: 'unset',      count: mesh.mtlsUnset,      color: unsetMeta.color,      label: 'unset' },
    { key: 'unknown',    count: mtlsUnknown,         color: NEUTRAL,              label: 'unknown' },
  ];

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex gap-4 flex-wrap">
        <div>
          <div className="tnum fw-bold" style={{ fontSize: 20, lineHeight: 1 }}>{nsPct}%</div>
          <div className="text-secondary fs-12 mt-1">
            namespaces enrolled ({mesh.nsEnrolled}/{metrics.nsTotal})
            {mesh.nsPartial > 0 && (
              <span className="ms-1" style={{ color: SEVERITY_COLOR.caution }}>
                · {mesh.nsPartial} partial
              </span>
            )}
          </div>
        </div>
        <div>
          <div className="tnum fw-bold" style={{ fontSize: 20, lineHeight: 1 }}>{wlPct}%</div>
          <div className="text-secondary fs-12 mt-1">
            workloads enrolled ({mesh.workloadsEnrolled}/{metrics.workloadTotal})
          </div>
        </div>
      </div>

      <div>
        <div className="text-secondary fs-12 mb-1">
          mTLS across {mesh.workloadsEnrolled} enrolled workloads
        </div>
        <div className="d-flex" style={{ height: 10, borderRadius: 4, overflow: 'hidden' }}>
          {segments.map((segment) => {
            const width = totalForBar === 0 ? 0 : (segment.count / totalForBar) * 100;
            if (width === 0) return null;
            return (
              <div
                key={segment.key}
                style={{ width: `${width}%`, background: segment.color }}
                title={`${segment.label}: ${segment.count}`}
              />
            );
          })}
        </div>
        <div className="d-flex flex-wrap gap-3 fs-12 mt-2">
          {segments.map((segment) => {
            const zero = segment.count === 0;
            return (
              <span
                key={segment.key}
                className="d-inline-flex align-items-center gap-1"
                title={segment.label}
              >
                <span
                  className="swatch-dot"
                  style={{ background: segment.color, opacity: zero ? 0.4 : 1, width: 8, height: 8, borderRadius: 2 }}
                />
                <span className={zero ? 'text-secondary' : 'text-light'}>{segment.label}</span>
                <span className={`tnum ${zero ? 'text-secondary opacity-50' : 'text-secondary'}`}>{segment.count}</span>
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
}
