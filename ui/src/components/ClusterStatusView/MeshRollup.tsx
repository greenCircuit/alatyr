// Mesh posture rollup for the status page. Answers: how many workloads are
// actually in the mesh (labels alone lie when ztunnel isn't running), and
// what mtls mode dominates. Each chip is clickable — applies the matching
// mesh filter and jumps to the graph so the operator can drill from
// "23 STRICT" straight into which nodes those are.

import { useMemo } from 'react';
import type { MeshMembership, WorkloadNode } from '../../data/policies';
import { meshBadgeMeta } from '../../data/policies';
import type { MeshFilterValue } from '../../store/filters';

interface Bucket { value: MeshFilterValue; label: string; count: number; color: string }

export default function MeshRollup({
  nodes,
  meshStatus,
  selectedMeshFilters,
  onSelect,
}: {
  nodes: WorkloadNode[];
  meshStatus: Record<string, MeshMembership>;
  selectedMeshFilters: Set<MeshFilterValue>;
  onSelect: (value: MeshFilterValue) => void;
}) {
  const stats = useMemo(() => {
    let inMesh = 0;
    let out    = 0;
    const mtls = { strict: 0, permissive: 0, disable: 0, unset: 0 };
    // Track provider · mode combinations so the header can name the actual
    // dataplane (istio ambient today; future multi-provider needs the breakdown).
    const providerModes = new Map<string, number>();
    for (const node of nodes) {
      if (node.type === 'namespace' || node.type === 'external') continue;
      const membership = meshStatus[node.id];
      if (membership?.inMesh) {
        inMesh += 1;
        const verdict = membership.mtls?.verdict ?? 'unset';
        mtls[verdict] += 1;
        const key = [membership.provider, membership.mode, membership.waypoint ? 'waypoint' : '']
          .filter(Boolean).join(' · ');
        if (key) providerModes.set(key, (providerModes.get(key) ?? 0) + 1);
      } else {
        out += 1;
      }
    }
    return { inMesh, out, mtls, total: inMesh + out, providerModes };
  }, [nodes, meshStatus]);

  const strictMeta     = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'strict' } });
  const permissiveMeta = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'permissive' } });
  const disableMeta    = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'disable' } });
  const unsetMeta      = meshBadgeMeta({ inMesh: true, mtls: { verdict: 'unset' } });
  const outMeta        = meshBadgeMeta(undefined);

  const buckets: Bucket[] = [
    { value: 'in-mesh',         label: 'In mesh',     count: stats.inMesh,         color: '#2f9e44' },
    { value: 'out-of-mesh',     label: 'Out of mesh', count: stats.out,            color: outMeta.color },
    { value: 'mtls-strict',     label: 'STRICT',      count: stats.mtls.strict,    color: strictMeta.color },
    { value: 'mtls-permissive', label: 'PERMISSIVE',  count: stats.mtls.permissive, color: permissiveMeta.color },
    { value: 'mtls-disable',    label: 'DISABLE',     count: stats.mtls.disable,   color: disableMeta.color },
    { value: 'mtls-unset',      label: 'UNSET',       count: stats.mtls.unset,     color: unsetMeta.color },
  ];

  if (stats.total === 0) {
    return <div className="text-secondary fs-12">No workloads in the current scope.</div>;
  }

  const enrolledPct = stats.total === 0 ? 0 : Math.round((stats.inMesh / stats.total) * 100);
  const providerLines = Array.from(stats.providerModes.entries())
    .sort((a, b) => b[1] - a[1])
    .map(([label, count]) => `${label} (${count})`);

  return (
    <div className="d-flex flex-column gap-2">
      <div className="text-secondary fs-12">
        {stats.inMesh} of {stats.total} workloads enrolled ({enrolledPct}%). Click a chip to filter.
      </div>
      {providerLines.length > 0 && (
        <div className="text-secondary fs-11">
          Dataplane: {providerLines.join(' · ')}
        </div>
      )}
      <div className="d-flex flex-wrap gap-1">
        {buckets.map((bucket) => {
          const active = selectedMeshFilters.has(bucket.value);
          const disabled = bucket.count === 0;
          return (
            <button
              key={bucket.value}
              type="button"
              className={`btn btn-sm d-inline-flex align-items-center gap-2 fs-12 ${active ? 'btn-warning' : 'btn-dark border border-secondary'}`}
              disabled={disabled}
              onClick={() => onSelect(bucket.value)}
              title={disabled ? 'No workloads in this bucket' : `${bucket.count} workload${bucket.count === 1 ? '' : 's'}`}
            >
              <span
                aria-hidden="true"
                style={{
                  display: 'inline-block', width: 8, height: 8, borderRadius: 2,
                  background: bucket.color,
                }}
              />
              {bucket.label}
              <span className={`badge ${disabled ? 'bg-secondary' : 'bg-dark'} border border-secondary`}>
                {bucket.count}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
