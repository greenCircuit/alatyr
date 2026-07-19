// Mesh posture rollup for the CURRENT SCOPE side of the Mesh section. Mirrors
// ClusterMeshSummary's shape (ns% + workloads% + mTLS bar + legend) so the
// operator can compare scope-vs-truth by scanning matching rows top-to-bottom.
// Chips remain clickable — they toggle the store's mesh filter and jump to
// the graph (unlike the read-only legend on the cluster-wide side).

import { useMemo } from 'react';
import type { MeshMembership, WorkloadNode } from '../../data/policies';
import { meshBadgeMeta } from '../../data/policies';
import type { MeshFilterValue } from '../../store/filters';
import r from '../../components/TablesView/Rollup.module.css';

const NEUTRAL = '#868e96';

type Verdict = 'strict' | 'permissive' | 'disable' | 'unset';

interface Bucket { value: MeshFilterValue; label: string; count: number; color: string }

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

  const strictMeta     = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'strict' } });
  const permissiveMeta = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'permissive' } });
  const disableMeta    = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'disable' } });
  const unsetMeta      = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'unset' } });
  const outMeta        = meshBadgeMeta(undefined);

  if (stats.total === 0) {
    return <div className="text-secondary fs-12">No workloads in the current scope.</div>;
  }

  const nsPct = stats.nsTotal === 0 ? 0 : Math.round((stats.nsEnrolled / stats.nsTotal) * 100);
  const wlPct = stats.total   === 0 ? 0 : Math.round((stats.inMesh / stats.total) * 100);

  const mtlsSegments = [
    { key: 'strict',     count: stats.mtls.strict,     color: strictMeta.color },
    { key: 'permissive', count: stats.mtls.permissive, color: permissiveMeta.color },
    { key: 'disable',    count: stats.mtls.disable,    color: disableMeta.color },
    { key: 'unset',      count: stats.mtls.unset,      color: unsetMeta.color },
  ];
  const mtlsTotalForBar = mtlsSegments.reduce((sum, segment) => sum + segment.count, 0);

  // Clickable buckets — same set as before but presented as compact chips
  // beneath the mTLS bar so the row-shape matches ClusterMeshSummary.
  const buckets: Bucket[] = [
    { value: 'in-mesh',         label: 'in mesh',    count: stats.inMesh,         color: '#2f9e44' },
    { value: 'out-of-mesh',     label: 'out',        count: stats.out,            color: outMeta.color },
    { value: 'mtls-strict',     label: 'STRICT',     count: stats.mtls.strict,    color: strictMeta.color },
    { value: 'mtls-permissive', label: 'permissive', count: stats.mtls.permissive, color: permissiveMeta.color },
    { value: 'mtls-disable',    label: 'DISABLE',    count: stats.mtls.disable,   color: disableMeta.color },
    { value: 'mtls-unset',      label: 'unset',      count: stats.mtls.unset,     color: unsetMeta.color },
  ];

  return (
    <div className="d-flex flex-column gap-3">
      <div className="d-flex gap-4 flex-wrap">
        <div>
          <div className="tnum fw-bold" style={{ fontSize: 20, lineHeight: 1 }}>{nsPct}%</div>
          <div className="text-secondary fs-12 mt-1">
            namespaces enrolled ({stats.nsEnrolled}/{stats.nsTotal})
          </div>
        </div>
        <div>
          <div className="tnum fw-bold" style={{ fontSize: 20, lineHeight: 1 }}>{wlPct}%</div>
          <div className="text-secondary fs-12 mt-1">
            workloads enrolled ({stats.inMesh}/{stats.total})
          </div>
        </div>
      </div>

      <div>
        <div className="text-secondary fs-12 mb-1">
          mTLS across {stats.inMesh} enrolled workloads
        </div>
        <div className="d-flex" style={{ height: 10, borderRadius: 4, overflow: 'hidden' }}>
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

      <div>
        <div className="text-secondary fs-12 mb-2">click a chip to filter</div>
        <div className={r.gridRollup}>
          {buckets.map((bucket) => {
            const active = selectedMeshFilters.has(bucket.value);
            const dimmed = selectedMeshFilters.size > 0 && !active;
            const disabled = bucket.count === 0;
            return (
              <div key={bucket.value} className={r.gridSlot}>
                <button
                  type="button"
                  className={`btn btn-sm d-inline-flex align-items-center gap-1 p-1 ${r.chip} ${active ? r.active : ''} ${dimmed ? r.dimmed : ''}`}
                  disabled={disabled}
                  onClick={() => onSelect(bucket.value)}
                  title={disabled ? 'No workloads in this bucket' : `${bucket.count} workload${bucket.count === 1 ? '' : 's'}`}
                  style={{ border: `1px solid ${bucket.color}` }}
                >
                  <span aria-hidden="true" style={{ color: bucket.color }}>●</span>
                  <span className="text-truncate">{bucket.label}</span>
                  <span className="chip-count ms-auto">{bucket.count}</span>
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
