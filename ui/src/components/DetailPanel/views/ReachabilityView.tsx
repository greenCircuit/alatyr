// Reachability pane: three columns (SRC workload, DST workload, verdict
// summary). Imports policy/mesh rendering pieces from sibling part files so
// per-engine and per-source breakdowns share visual language with the
// workload-detail view.

import type { CSSProperties } from 'react';
import type {
  ReachabilityResult,
  EngineVerdict,
  DirectionVerdict,
  WorkloadNode,
  MeshMembership,
  MeshVerdict,
} from '../../../data/policies';
import { SEVERITY_COLOR } from '../../../data/policies';
import s from '../DetailPanel.module.css';
import { PolicyRefList, RuleRow } from '../shared/rows';
import { ManifestButton } from '../shared/ManifestModal';
import { DIR_COLOR, MTLS_VERDICT_COLOR, verdictColor } from '../shared/presentation';

function DirectionBlock({ label, dir, direction }: {
  label:     string;
  dir:       DirectionVerdict;
  direction: 'egress' | 'ingress';
}) {
  const { tint, arrow } = DIR_COLOR[direction];
  const allowCount = dir.allowMatches?.length ?? 0;
  const denyCount  = dir.denyMatches?.length  ?? 0;
  return (
    <div
      className={`rounded p-2 mb-2 border border-secondary ${s.calloutAccentThin}`}
      style={{ '--accent': tint } as CSSProperties}
    >
      <div className="d-flex justify-content-between align-items-center mb-1 flex-wrap gap-1">
        <span className={`fw-semibold ${s.calloutAccentText}`}>{arrow} {label}</span>
        <div className="d-flex gap-1 flex-wrap">
          {allowCount > 0 && (
            <span className="badge" style={{ background: SEVERITY_COLOR.secure }}>
              {allowCount} allow
            </span>
          )}
          {denyCount > 0 && (
            <span className="badge bg-danger">{denyCount} deny</span>
          )}
          {/* Surface "locked + no allow" as default-deny so it screams from the chip row
              instead of hiding inside the reason text. */}
          {dir.locked && allowCount === 0 && denyCount === 0
            ? <span className="badge bg-danger">default-deny</span>
            : dir.locked
              ? <span className="badge bg-warning text-dark">locked</span>
              : <span className="badge bg-secondary">open</span>}
        </div>
      </div>
      <div className={`text-secondary ${s.smallText} mb-1`}>{dir.reason}</div>
      {allowCount > 0 && (
        <div className="mt-2">
          <div className="text-success small">Allow rules</div>
          {dir.allowMatches!.map((rule, i) => <RuleRow key={`a-${i}`} rule={rule} />)}
        </div>
      )}
      {denyCount > 0 && (
        <div className="mt-2">
          <div className="text-danger small">Deny rules</div>
          {dir.denyMatches!.map((rule, i) => <RuleRow key={`d-${i}`} rule={rule} />)}
        </div>
      )}
    </div>
  );
}

function EngineCard({ name, ev }: { name: string; ev: EngineVerdict }) {
  const color = ev.status === 'allow' ? SEVERITY_COLOR.secure
              : ev.status === 'deny'  ? SEVERITY_COLOR.high
              : '#6c757d';
  return (
    <div
      className={`border border-secondary rounded p-2 mb-2 ${s.calloutAccent}`}
      style={{ '--accent': color } as CSSProperties}
    >
      <div className="d-flex justify-content-between align-items-center mb-2">
        <span className="fw-semibold">{name}</span>
        <span className="badge text-light" style={{ background: color }}>{ev.status}</span>
      </div>
      <DirectionBlock label="Egress (src side)"  dir={ev.egress}  direction="egress" />
      <DirectionBlock label="Ingress (dst side)" dir={ev.ingress} direction="ingress" />
    </div>
  );
}

function MeshSideCard({ source, membership }: { source: string; membership: MeshMembership }) {
  const inMesh = membership.inMesh;
  const verdict = membership.mtls?.verdict;
  return (
    <div className="border border-secondary rounded p-2 mb-2">
      <div className="d-flex justify-content-between align-items-center mb-1">
        <span className="fw-semibold">mesh · {source}</span>
        <div className="d-flex gap-1">
          <span className={`badge ${inMesh ? 'bg-success' : 'bg-secondary'}`}>
            {inMesh ? 'in mesh' : 'not in mesh'}
          </span>
          {verdict && (
            <span
              className="badge"
              style={{ background: MTLS_VERDICT_COLOR[verdict] ?? '#6c757d', color: '#1a1d20' }}
            >
              mTLS: {verdict}
            </span>
          )}
        </div>
      </div>
      {membership.mtls?.effectiveSource?.name && (
        <div className={`text-secondary d-flex align-items-center gap-2 ${s.smallText}`}>
          <span>From PA: {membership.mtls.effectiveSource.namespace}/{membership.mtls.effectiveSource.name}</span>
          <ManifestButton
            kind="pa"
            namespace={membership.mtls.effectiveSource.namespace}
            name={membership.mtls.effectiveSource.name}
          />
        </div>
      )}
    </div>
  );
}

function WorkloadColumn({ node, role, engines, policiesKey, mesh }: {
  node:        WorkloadNode;
  role:        'SRC' | 'DST';
  engines:     [string, EngineVerdict][];
  policiesKey: 'srcPolicies' | 'dstPolicies';
  mesh?:       Record<string, MeshMembership>;
}) {
  const roleColor = role === 'SRC' ? 'bg-info' : 'bg-warning';
  const meshEntries = mesh ? Object.entries(mesh) : [];
  return (
    <div className={s.reachCol}>
      <div className={s.reachColHeader}>
        <span className={`badge ${roleColor} text-dark me-2`}>{role}</span>
        <span className="text-break">{node.label}</span>
        <div className={`text-secondary ${s.smallText}`}>
          {node.namespace || '—'} · {node.type}
        </div>
      </div>
      <div className="d-flex flex-wrap gap-1 mb-3">
        {Object.entries(node.labels).map(([k, v]) => (
          <span key={k} className={`badge bg-secondary ${s.badgeSm}`}>{k}={v}</span>
        ))}
        {Object.keys(node.labels).length === 0 && (
          <span className={`text-secondary ${s.smallText}`}>no labels</span>
        )}
      </div>
      {engines.map(([name, ev]) => (
        <div key={name} className="border border-secondary rounded p-2 mb-2">
          <div className="fw-semibold mb-1">{name}</div>
          <div className={`text-secondary ${s.smallText} mb-1`}>Selecting policies</div>
          <PolicyRefList items={ev[policiesKey]} />
        </div>
      ))}
      {meshEntries.map(([source, m]) => (
        <MeshSideCard key={source} source={source} membership={m} />
      ))}
    </div>
  );
}

function MeshCardReach({ name, v }: { name: string; v: MeshVerdict }) {
  const color = v.verdict === 'allow' ? SEVERITY_COLOR.secure
              : v.verdict === 'deny'  ? SEVERITY_COLOR.high
              : '#6c757d';
  return (
    <div
      className={`border border-secondary rounded p-2 mb-2 ${s.calloutAccent}`}
      style={{ '--accent': color } as CSSProperties}
    >
      <div className="d-flex justify-content-between align-items-center mb-1">
        <span className="fw-semibold">mesh · {name}</span>
        <span className="badge text-light" style={{ background: color }}>{v.verdict}</span>
      </div>
      <div className={`text-secondary ${s.smallText}`}>{v.reason}</div>
      {v.effectiveSource?.name && (
        <div className={`text-secondary d-flex align-items-center gap-2 ${s.smallText} mt-1`}>
          <span>Forced by: {v.effectiveSource.namespace}/{v.effectiveSource.name}</span>
          <ManifestButton kind="pa" namespace={v.effectiveSource.namespace} name={v.effectiveSource.name} />
        </div>
      )}
    </div>
  );
}

function ResultColumn({ result, engines }: {
  result:  ReachabilityResult;
  engines: [string, EngineVerdict][];
}) {
  const meshEntries = result.mesh ? Object.entries(result.mesh) : [];
  return (
    <div className={s.reachCol}>
      <div className={s.reachColHeader}>
        <span
          className="badge text-bold text-uppercase me-2"
          style={{ background: verdictColor(result.verdict) }}
        >
          {result.verdict}
        </span>
        verdict
      </div>
      <div
        className={`mb-3 ${result.verdict === 'allow' ? `text-secondary ${s.smallText}` : 'fw-bold fs-5'}`}
        style={result.verdict !== 'allow' ? { color: verdictColor(result.verdict) } : undefined}
      >
        {result.reason}
      </div>
      {engines.map(([name, ev]) => (
        <EngineCard key={name} name={name} ev={ev} />
      ))}
      {meshEntries.map(([name, v]) => (
        <MeshCardReach key={name} name={name} v={v} />
      ))}
    </div>
  );
}

function ReachabilityGrid({ src, dst, result }: {
  src:    WorkloadNode;
  dst:    WorkloadNode;
  result: ReachabilityResult;
}) {
  const engines = Object.entries(result.engines);
  return (
    <div className={s.reachGrid}>
      <WorkloadColumn node={src} role="SRC" engines={engines} policiesKey="srcPolicies" mesh={result.srcMesh} />
      <WorkloadColumn node={dst} role="DST" engines={engines} policiesKey="dstPolicies" mesh={result.dstMesh} />
      <ResultColumn result={result} engines={engines} />
    </div>
  );
}

// Full reachability region: the pinned-source banner (always shown once a
// source is pinned), a loading hint while the verdict computes, and the
// three-column grid once a target is clicked and the result lands.
export function ReachabilityView({ source, target, loading, result, onClear }: {
  source:  WorkloadNode;
  target:  WorkloadNode | null;
  loading: boolean;
  result:  ReachabilityResult | null;
  onClear: () => void;
}) {
  return (
    <>
      <div className="border border-info rounded p-2 mb-2">
        <div className="d-flex justify-content-between align-items-center">
          <div>
            <span className="badge bg-info text-dark me-2">SRC</span>
            <span className="fw-semibold">{source.label}</span>
            <span className="text-secondary"> / {source.namespace || '—'}</span>
          </div>
          <button className="btn btn-sm btn-outline-light" onClick={onClear}>
            Cancel
          </button>
        </div>
        <div className={`text-secondary mt-1 ${s.smallText}`}>
          {target
            ? <>Target: {target.label}</>
            : <>Click another node to check reachability</>}
        </div>
      </div>

      {loading && (
        <div className="text-secondary small mb-2">Computing reachability…</div>
      )}

      {result && target && (
        <ReachabilityGrid src={source} dst={target} result={result} />
      )}
    </>
  );
}
