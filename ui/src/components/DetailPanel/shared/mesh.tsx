// Mesh state rendering for the workload-detail view: membership card with
// nested mTLS verdict, source-PA list, and per-port overrides. The
// reachability pane has its own slimmer mesh card (in reachability.tsx) so
// these components stay tuned to the detail-panel layout.

import type { MeshMembership, MtlsState, MtlsSource } from '../../../data/policies';
import { SEVERITY_COLOR } from '../../../data/policies';
import s from '../DetailPanel.module.css';
import { MTLS_VERDICT_COLOR } from './presentation';
import { ManifestButton } from './ManifestModal';

function MtlsSourceRow({ src, isEffective }: { src: MtlsSource; isEffective: boolean }) {
  const scope = src.meshScope;
  return (
    <div
      className={`border border-secondary rounded p-2 mb-1 ${s.smallText}`}
      style={isEffective ? { borderColor: SEVERITY_COLOR.secure, borderWidth: 2 } : undefined}
    >
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className="fw-semibold text-break">{src.namespace}/{src.name}</div>
        <div className="d-flex gap-1 flex-shrink-0 align-items-center">
          {isEffective && <span className="badge bg-success">effective</span>}
          <span className="badge" style={{ background: MTLS_VERDICT_COLOR[scope] ?? '#6c757d', color: '#1a1d20' }}>
            {scope}
          </span>
          <span className="badge bg-secondary">{src.meshSource}</span>
          <ManifestButton kind="pa" namespace={src.namespace} name={src.name} />
        </div>
      </div>
      {src.portModes && Object.keys(src.portModes).length > 0 && (
        <div className="d-flex flex-wrap gap-1 mt-1">
          {Object.entries(src.portModes).map(([port, mode]) => (
            <span key={port} className="badge bg-info text-dark">
              {port}: {mode}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

function MtlsBlock({ mtls }: { mtls: MtlsState }) {
  const effectiveKey = `${mtls.effectiveSource?.namespace ?? ''}/${mtls.effectiveSource?.name ?? ''}`;
  const hasEffective = mtls.effectiveSource?.name !== undefined && mtls.effectiveSource?.name !== '';
  return (
    <div className="mt-2">
      <div className="d-flex align-items-center gap-2">
        <span className="text-secondary">Verdict</span>
        <span
          className="badge text-uppercase"
          style={{ background: MTLS_VERDICT_COLOR[mtls.verdict] ?? '#6c757d', color: '#1a1d20' }}
        >
          {mtls.verdict}
        </span>
      </div>
      {hasEffective && mtls.effectiveSource && (
        <div className={`text-secondary ${s.smallText} mt-1`}>
          Effective: {mtls.effectiveSource.namespace}/{mtls.effectiveSource.name}
        </div>
      )}
      {mtls.portOverrides && Object.keys(mtls.portOverrides).length > 0 && (
        <div className="mt-2">
          <div className="text-secondary">Port overrides</div>
          <div className="d-flex flex-wrap gap-1 mt-1">
            {Object.entries(mtls.portOverrides).map(([port, mode]) => (
              <span
                key={port}
                className="badge"
                style={{ background: MTLS_VERDICT_COLOR[mode] ?? '#6c757d', color: '#1a1d20' }}
              >
                {port}: {mode}
              </span>
            ))}
          </div>
        </div>
      )}
      {mtls.issues && mtls.issues.length > 0 && (
        <div className="mt-2">
          <div className="text-warning">Issues</div>
          <ul className={`ps-3 mb-0 ${s.smallText}`}>
            {mtls.issues.map((issue, i) => <li key={i}>{issue}</li>)}
          </ul>
        </div>
      )}
      {mtls.sources && mtls.sources.length > 0 && (
        <div className="mt-2">
          <div className="text-secondary">PeerAuthentications</div>
          <div className="mt-1">
            {mtls.sources.map((src, i) => (
              <MtlsSourceRow key={i} src={src} isEffective={`${src.namespace}/${src.name}` === effectiveKey} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export function MeshCard({ source, membership }: { source: string; membership: MeshMembership }) {
  const inMesh = membership.inMesh;
  return (
    <div className="border border-secondary rounded p-2">
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className="fw-semibold">{source}</div>
        <span className={`badge ${inMesh ? 'bg-success' : 'bg-secondary'}`}>
          {inMesh ? 'in mesh' : 'not in mesh'}
        </span>
      </div>
      {inMesh && (membership.provider || membership.mode) && (
        <div className={`text-secondary ${s.smallText}`}>
          {membership.provider}{membership.mode ? ` · ${membership.mode}` : ''}
        </div>
      )}
      {membership.waypoint && (
        <div className={`text-secondary ${s.smallText} mt-1`}>
          Waypoint: {membership.waypoint.namespace}/{membership.waypoint.name}
        </div>
      )}
      {membership.mtls && <MtlsBlock mtls={membership.mtls} />}
    </div>
  );
}
