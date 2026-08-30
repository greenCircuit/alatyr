// Mesh posture rollup for the CURRENT SCOPE side of the Mesh section. Mirrors
// ClusterMeshSummary's shape (ns% + workloads% + mTLS bar + legend) so the
// operator can compare scope-vs-truth by scanning matching rows top-to-bottom.
// Chips remain clickable — they toggle the store's mesh filter and jump to
// the graph (unlike the read-only legend on the cluster-wide side).

import { useMemo } from 'react';
import type { MeshMembership, WorkloadNode, MtlsScope } from '../../data/policies';
import { meshBadgeMeta, MTLS_COLOR } from '../../data/policies';
import type { MeshFilterValue } from '../../store/filters';
import r from '../../components/TablesView/Rollup.module.css';
import p from './Panel.module.css';

const NEUTRAL = '#868e96';

type Verdict = 'strict' | 'permissive' | 'disable' | 'unset';

// `scope` marks mTLS-verdict buckets (vs membership buckets) — it selects the
// verdict label styling and keys the stacked-bar segments.
interface Bucket {
  value: MeshFilterValue;
  label: string;
  count: number;
  color: string;
  scope?: MtlsScope;
}

export default function MeshRollup({
  nodes,
  meshStatus,
  selectedMeshFilters,
  onSelect,
}: {
  nodes:               WorkloadNode[];
  meshStatus:          Record<string, MeshMembership>;
  selectedMeshFilters: Set<MeshFilterValue>;
  onSelect:            (value: MeshFilterValue) => void;
}) {
  const stats = useMemo(() => {
    let inMesh = 0;
    let out    = 0;
    const mtls: Record<Verdict, number> = { strict: 0, permissive: 0, disable: 0, unset: 0 };
    const nsTotal    = new Set<string>();
    const nsEnrolled = new Set<string>();
    for (const node of nodes) {
      if (node.type === 'namespace' || node.type === 'external') continue;
      nsTotal.add(node.namespace);
      const membership = meshStatus[node.id];
      if (membership?.inMesh) {
        inMesh += 1;
        nsEnrolled.add(node.namespace);
        const verdict = (membership.mtls?.verdict ?? 'unset') as Verdict;
        mtls[verdict] += 1;
      } else {
        out += 1;
      }
    }
    return {
      inMesh, out, mtls, total: inMesh + out,
      nsTotal: nsTotal.size, nsEnrolled: nsEnrolled.size,
    };
  }, [nodes, meshStatus]);

  const outMeta = meshBadgeMeta(undefined);

  if (stats.total === 0) {
    return <div className="text-secondary fs-12">No workloads in the current scope.</div>;
  }

  const nsPct = stats.nsTotal === 0 ? 0 : Math.round((stats.nsEnrolled / stats.nsTotal) * 100);
  const wlPct = stats.total   === 0 ? 0 : Math.round((stats.inMesh / stats.total) * 100);

  const mtlsSegments = [
    { key: 'strict',     count: stats.mtls.strict,     color: MTLS_COLOR.strict },
    { key: 'permissive', count: stats.mtls.permissive, color: MTLS_COLOR.permissive },
    { key: 'disable',    count: stats.mtls.disable,    color: MTLS_COLOR.disable },
    { key: 'unset',      count: stats.mtls.unset,      color: MTLS_COLOR.unset },
  ];
  const mtlsTotalForBar = mtlsSegments.reduce((sum, segment) => sum + segment.count, 0);

  // Two semantic groups — membership (enrolled or not) and mTLS verdict — kept
  // in separate rows so the operator scans one question at a time.
  const membershipBuckets: Bucket[] = [
    { value: 'in-mesh',     label: 'in mesh', count: stats.inMesh, color: '#2f9e44' },
    { value: 'out-of-mesh', label: 'out',     count: stats.out,    color: outMeta.color },
  ];
  const mtlsBuckets: Bucket[] = [
    { value: 'mtls-strict',     label: 'strict',     count: stats.mtls.strict,     color: MTLS_COLOR.strict,     scope: 'strict' },
    { value: 'mtls-permissive', label: 'permissive', count: stats.mtls.permissive, color: MTLS_COLOR.permissive, scope: 'permissive' },
    { value: 'mtls-disable',    label: 'disable',    count: stats.mtls.disable,    color: MTLS_COLOR.disable,    scope: 'disable' },
    { value: 'mtls-unset',      label: 'unset',      count: stats.mtls.unset,      color: MTLS_COLOR.unset,      scope: 'unset' },
  ];

  const renderChip = (bucket: Bucket) => {
    const active = selectedMeshFilters.has(bucket.value);
    const dimmed = selectedMeshFilters.size > 0 && !active;
    const disabled = bucket.count === 0;
    return (
      <button
        key={bucket.value}
        type="button"
        className={`btn btn-sm d-inline-flex align-items-center gap-1 p-1 ${r.chip} ${active ? r.active : ''} ${dimmed ? r.dimmed : ''}`}
        disabled={disabled}
        onClick={() => onSelect(bucket.value)}
        title={disabled ? 'No workloads in this bucket' : `${bucket.count} workload${bucket.count === 1 ? '' : 's'}`}
        style={{ border: `1px solid ${bucket.color}` }}
      >
        {/* No marker glyph — the chip border already carries the bucket hue. */}
        <span className={`text-truncate ${bucket.scope ? p.verdictLabel : ''}`}>{bucket.label}</span>
        <span className="chip-count ms-1">{bucket.count}</span>
      </button>
    );
  };

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex gap-4 flex-wrap">
        <div>
          <div className={`${p.metric} tnum`}>{nsPct}%</div>
          <div className="text-secondary fs-12 mt-1">
            namespaces enrolled ({stats.nsEnrolled}/{stats.nsTotal})
          </div>
        </div>
        <div>
          <div className={`${p.metric} tnum`}>{wlPct}%</div>
          <div className="text-secondary fs-12 mt-1">
            workloads enrolled ({stats.inMesh}/{stats.total})
          </div>
        </div>
      </div>

      <div>
        <div className="text-secondary fs-12 mb-1">
          mTLS across {stats.inMesh} enrolled workloads
        </div>
        <div className={p.mtlsBar}>
          {mtlsSegments.map((segment) => {
            const width = mtlsTotalForBar === 0 ? 0 : (segment.count / mtlsTotalForBar) * 100;
            if (width === 0) return null;
            return (
              <div
                key={segment.key}
                style={{ width: `${width}%`, background: segment.color }}
                title={`${segment.key}: ${segment.count}`}
              />
            );
          })}
          {mtlsTotalForBar === 0 && (
            <div style={{ width: '100%', background: NEUTRAL, opacity: 0.4 }} />
          )}
        </div>
      </div>

      <div className="d-flex flex-column gap-2">
        <span className={p.eyebrow}>Membership</span>
        <div className="d-flex flex-wrap gap-2">{membershipBuckets.map(renderChip)}</div>
      </div>

      <div className="d-flex flex-column gap-2">
        <span className={p.eyebrow}>mTLS</span>
        <div className="d-flex flex-wrap gap-2">{mtlsBuckets.map(renderChip)}</div>
      </div>
    </div>
  );
}
