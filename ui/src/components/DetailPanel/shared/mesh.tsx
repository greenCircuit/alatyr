// Mesh state rendering for the workload-detail view: membership card with
// nested mTLS verdict, source-PA list, and per-port overrides. The
// reachability pane has its own slimmer mesh card (in reachability.tsx) so
// these components stay tuned to the detail-panel layout.

import type { MeshMembership, MtlsScope, MtlsState, MtlsSource } from '../../../data/policies';
import s from '../DetailPanel.module.css';
import { ManifestButton } from './ManifestModal';
import { EngineBadge } from './badges';
import { MtlsChip } from './MtlsChip';

function MtlsSourceRow({ src, isEffective }: { src: MtlsSource; isEffective: boolean }) {
  const scope = src.meshScope;
  return (
    <div className={`${s.card} ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className={`${s.section} text-break`}>{src.namespace}/{src.name}</div>
        <div className="d-flex gap-1 flex-shrink-0 align-items-center">
          {isEffective && <span className={`${s.miniChip} ${s.miniChipAllow}`}>effective</span>}
          <MtlsChip scope={scope as MtlsScope} />
          <span className={s.typeChip}>{src.meshSource}</span>
          <ManifestButton kind="pa" namespace={src.namespace} name={src.name} />
        </div>
      </div>
      {src.portModes && Object.keys(src.portModes).length > 0 && (
        <div className="d-flex flex-wrap gap-1 mt-1">
          {Object.entries(src.portModes).map(([port, mode]) => (
            <span key={port} className={s.portChip}>
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
    <div className="d-flex flex-column gap-2">
      <div className="d-flex align-items-center gap-2">
        <span className={s.eyebrow}>Verdict</span>
        <MtlsChip scope={mtls.verdict as MtlsScope} />
      </div>
      {hasEffective && mtls.effectiveSource && (
        <div className={`${s.dim} ${s.smallText} ${s.mono}`}>
          effective: {mtls.effectiveSource.namespace}/{mtls.effectiveSource.name}
        </div>
      )}
      {mtls.portOverrides && Object.keys(mtls.portOverrides).length > 0 && (
        <div className="d-flex flex-column gap-1">
          <div className={s.eyebrow}>Port overrides</div>
          <div className="d-flex flex-wrap gap-1">
            {Object.entries(mtls.portOverrides).map(([port, mode]) => (
              <MtlsChip key={port} scope={mode as MtlsScope} label={`${port}: ${mode}`} />
            ))}
          </div>
        </div>
      )}
      {mtls.sources && mtls.sources.length > 0 && (
        <div className="d-flex flex-column gap-1">
          <div className={s.eyebrow}>PeerAuthentications</div>
          <div>
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
    <div className={s.tier1}>
      <div className="d-flex justify-content-between align-items-start gap-2">
        <div className="d-flex align-items-center gap-2">
          <EngineBadge engine={source} />
          <span className={s.section}>{source}</span>
        </div>
        <span className={`${s.miniChip} ${inMesh ? s.miniChipAllow : s.miniChipDim}`}>
          {inMesh ? 'in mesh' : 'not in mesh'}
        </span>
      </div>
      {inMesh && (membership.provider || membership.mode) && (
        <div className={`${s.body} ${s.dim}`}>
          {membership.provider}{membership.mode ? `/${membership.mode}` : ''}
        </div>
      )}
      {membership.waypoint && (
        <div className="d-flex align-items-center gap-1">
          <span className={s.eyebrow}>Waypoint</span>
          <span className={`${s.body} ${s.mono}`}>{membership.waypoint.namespace}/{membership.waypoint.name}</span>
        </div>
      )}
      {membership.mtls && <MtlsBlock mtls={membership.mtls} />}
    </div>
  );
}
