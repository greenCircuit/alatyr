// Reachability pane. Information hierarchy, top to bottom:
//   Tier 0 — verdict headline: can src reach dst, yes/no + one-line reason.
//   Tier 1 — blocker list (deny only): every (engine, direction) that blocks,
//            ranked by remediation, with the culprit policies to edit. AND
//            semantics — the path opens only when every blocker clears.
//   Tier 2 — per-engine/per-direction breakdown + selecting-policy columns,
//            collapsed by default. Proof and audit detail, not the 3am answer.
// Column policy chips are joined back to the verdict (classifyPolicy) so the
// SRC/DST lists show which policy decided, not just the universe selecting it.

import type { CSSProperties } from 'react';
import type {
  ReachabilityResult,
  EngineVerdict,
  DirectionVerdict,
  DirectionReason,
  PolicyRef,
  WorkloadNode,
  MeshMembership,
  MeshVerdict,
} from '../../../data/policies';
import { SEVERITY_COLOR } from '../../../data/policies';
import s from '../DetailPanel.module.css';
import { PolicyRefList, RuleGroupList } from '../shared/rows';
import { ManifestButton } from '../shared/ManifestModal';
import { DIR_COLOR, MTLS_VERDICT_COLOR, verdictColor, REASON_META, CHIP_STATE } from '../shared/presentation';
import { deriveBlockers, classifyPolicy, type Blocker, type ChipState } from '../shared/reachability';

// ── Tier 0 ──────────────────────────────────────────────────────────────────

// Per-subsystem chip. Green allow, red deny, grey unknown/not-enforced.
// Rendered in the headline so the operator sees every engine's + mesh's answer
// at a glance — not just the aggregate. If istio says allow but k8s says deny,
// both should be visible before the operator drills into the breakdown.
function subsystemColor(status: string): string {
  if (status === 'allow') return SEVERITY_COLOR.secure;
  if (status === 'deny')  return SEVERITY_COLOR.high;
  return '#6c757d';
}

function SubsystemChip({ label, status }: { label: string; status: string }) {
  return (
    <span
      className={`badge text-ink-dark ${s.smallText}`}
      style={{ background: subsystemColor(status) }}
    >
      {label}: {status}
    </span>
  );
}

// Bidirectional summary — pairs forward + reverse verdicts into one chip so the
// operator instantly knows whether the pair talks both ways, is one-way, or is
// blocked in both directions. Reverse fetch fires alongside the forward one; if
// it errored, an explicit "reverse unavailable" chip surfaces the failure rather
// than silently showing nothing (which would read as "no bidirectional info").
function BidirectionalChip({ forward, reverse, error }: {
  forward: string;
  reverse: string | undefined;
  error:   boolean;
}) {
  if (error) {
    return (
      <span
        className={`badge bg-slate text-light ${s.smallText}`}
        title="Reverse-direction fetch failed"
      >
        ⚠ reverse unavailable
      </span>
    );
  }
  if (!reverse) return null;
  let label = '';
  let color = '#6c757d';
  if (forward === 'allow' && reverse === 'allow') {
    label = '↔ bidirectional'; color = SEVERITY_COLOR.secure;
  } else if (forward === 'allow' && reverse === 'deny') {
    label = '→ one-way (reverse blocked)'; color = SEVERITY_COLOR.caution;
  } else if (forward === 'deny' && reverse === 'allow') {
    label = '← reverse only reachable'; color = SEVERITY_COLOR.caution;
  } else if (forward === 'deny' && reverse === 'deny') {
    label = '✗ blocked both ways'; color = SEVERITY_COLOR.high;
  } else {
    return null;
  }
  return (
    <span className={`badge text-ink-dark ${s.smallText}`} style={{ background: color }}>
      {label}
    </span>
  );
}

// Full-width yes/no. Loud on deny (this is what pages someone), quiet on allow.
// Names both endpoints so the verdict reads without cross-referencing columns.
// Subsystem strip below the arrow enumerates every engine + mesh so the operator
// can't miss a disagreeing subsystem.
function VerdictHeadline({ result, reverse, reverseError, src, dst }: {
  result:       ReachabilityResult;
  reverse:      ReachabilityResult | null;
  reverseError: boolean;
  src:          WorkloadNode;
  dst:          WorkloadNode;
}) {
  const deny  = result.verdict === 'deny';
  const color = verdictColor(result.verdict);
  const engines = Object.entries(result.engines);
  const meshes  = result.mesh ? Object.entries(result.mesh) : [];
  return (
    <div className={`rounded p-2 mb-3 ${s.calloutAccent}`} style={{ '--accent': color } as CSSProperties}>
      <div className="d-flex align-items-center gap-2 mb-1 flex-wrap">
        <span className="badge text-uppercase text-ink-dark" style={{ background: color }}>{result.verdict}</span>
        <span className="fw-bold d-flex align-items-center gap-1 flex-wrap">
          <span className="badge bg-info text-dark">{src.label}</span>
          <span style={{ color }}>{deny ? '✗ cannot reach' : '→ can reach'}</span>
          <span className="badge bg-warning text-dark">{dst.label}</span>
        </span>
        <BidirectionalChip forward={result.verdict} reverse={reverse?.verdict} error={reverseError} />
      </div>
      {(engines.length > 0 || meshes.length > 0) && (
        <div className="d-flex flex-wrap gap-1 mb-2">
          {engines.map(([name, ev]) => (
            <SubsystemChip key={`e-${name}`} label={name} status={ev.status} />
          ))}
          {meshes.map(([name, v]) => (
            <SubsystemChip key={`m-${name}`} label={`mesh ${name}`} status={v.verdict} />
          ))}
        </div>
      )}
      <div
        className={deny ? 'fw-semibold' : `text-secondary ${s.smallText}`}
        style={deny ? { color } : undefined}
      >
        {result.reason}
      </div>
    </div>
  );
}

// ── Tier 1 ──────────────────────────────────────────────────────────────────

// One blocking axis: engine, a colored direction + side pill naming the endpoint
// to edit, the reason chip, culprits, and (explicit-deny) the rules to delete.
// side maps egress→src, ingress→dst; mesh pins to the dst endpoint.
function BlockerRow({ blocker, src, dst }: { blocker: Blocker; src: WorkloadNode; dst: WorkloadNode }) {
  const meta     = REASON_META[blocker.reason as DirectionReason];
  const isMesh   = blocker.direction === 'mesh';
  const sideNode = blocker.side === 'src' ? src : dst;
  // Reuse the panel's direction tints (↑ egress / ↓ ingress); mesh gets a lock.
  const dir      = isMesh ? { tint: '#845ef7', arrow: '🔒' } : DIR_COLOR[blocker.direction];
  const dirLabel = isMesh ? 'mTLS' : blocker.direction;
  // Side pill echoes the SRC/DST column colors so the eye lands on the right column.
  const sideColor = blocker.side === 'src' ? 'bg-info' : 'bg-warning';
  return (
    <div className="border border-danger rounded p-2 mb-2">
      <div className="d-flex justify-content-between align-items-center gap-2 mb-2 flex-wrap">
        <div className="d-flex align-items-center gap-1 flex-wrap">
          <span className="fw-semibold me-1">{blocker.engine}</span>
          <span className="badge text-ink-dark" style={{ background: dir.tint }}>{dir.arrow} {dirLabel}</span>
          <span className={`badge ${sideColor} text-dark`}>
            {blocker.side.toUpperCase()} · {sideNode.label}
          </span>
        </div>
        <span className={`badge ${meta?.badge ?? 'bg-danger'}`}>{meta?.label ?? blocker.reason}</span>
      </div>
      {blocker.paSource && (
        <div className={`text-secondary d-flex align-items-center gap-2 ${s.smallText} mb-1`}>
          <span>Forced by: {blocker.paSource.namespace}/{blocker.paSource.name}</span>
          <ManifestButton kind="pa" namespace={blocker.paSource.namespace} name={blocker.paSource.name} />
        </div>
      )}
      {blocker.culprits.length > 0 && (
        <div className="mt-1">
          <div className={`text-secondary ${s.smallText} mb-1`}>Edit to allow</div>
          <PolicyRefList items={blocker.culprits} />
        </div>
      )}
      {/* explicit-deny only: a deny rule is actively winning — show it as the
          delete target alongside the culprit. */}
      {blocker.denyRules.length > 0 && (
        <div className="mt-2">
          <div className="text-danger small">Deny rules — delete to allow</div>
          <RuleGroupList rules={blocker.denyRules} />
        </div>
      )}
    </div>
  );
}

function BlockerList({ result, src, dst }: {
  result: ReachabilityResult;
  src:    WorkloadNode;
  dst:    WorkloadNode;
}) {
  const blockers = deriveBlockers(result);
  if (blockers.length === 0) return null;
  return (
    <div className="mb-3">
      <div className="text-uppercase text-secondary small fw-semibold mb-2">
        Blocked on {blockers.length} {blockers.length === 1 ? 'axis' : 'axes'} · all must clear
      </div>
      {blockers.map((blocker, index) => <BlockerRow key={index} blocker={blocker} src={src} dst={dst} />)}
    </div>
  );
}

// ── Tier 2: per-engine/per-direction breakdown ───────────────────────────────

function DirectionBlock({ label, dir, direction }: {
  label:     string;
  dir:       DirectionVerdict;
  direction: 'egress' | 'ingress';
}) {
  const { tint, arrow } = DIR_COLOR[direction];
  const meta     = REASON_META[dir.reason];
  const allow    = dir.allowMatches ?? [];
  const deny     = dir.denyMatches ?? [];
  const denyAll  = dir.denyAllMatches ?? [];
  const other    = dir.allowOtherMatches ?? [];
  const culprits = dir.culprits ?? [];
  // Near-miss "allowed elsewhere" is only actionable on locked-no-match (an
  // allow exists, this peer misses the selector → widen it). On default/explicit
  // deny or a permit it's noise, so it's suppressed there.
  const showNearMiss = other.length > 0 && dir.reason === 'locked-no-match';
  return (
    <div
      className={`rounded p-2 mb-2 border border-secondary ${s.calloutAccentThin}`}
      style={{ '--accent': tint } as CSSProperties}
    >
      <div className="d-flex justify-content-between align-items-center mb-1 flex-wrap gap-1">
        <span className={`fw-semibold ${s.calloutAccentText}`}>{arrow} {label}</span>
        <span className={`badge ${meta.badge}`}>{meta.label}</span>
      </div>
      {/* Culprits: the policies to edit for this block. */}
      {culprits.length > 0 && (
        <div className="mt-1 mb-2">
          <div className={`text-secondary ${s.smallText} mb-1`}>Policies to edit</div>
          <PolicyRefList items={culprits} />
        </div>
      )}
      {/* deny-all lock: one line, not a full rule dump — it explains the
          default-deny reason, the per-rule detail adds nothing. */}
      {denyAll.length > 0 && (
        <div className="mt-1 text-danger small">
          deny-all lock: {denyAll.map((rule) => rule.contributor?.name).filter(Boolean).join(', ') || 'default-deny'}
        </div>
      )}
      {allow.length > 0 && (
        <div className="mt-2">
          <div className="text-success small">Allow rules</div>
          <RuleGroupList rules={allow} />
        </div>
      )}
      {deny.length > 0 && (
        <div className="mt-2">
          <div className="text-danger small">Deny rules</div>
          <RuleGroupList rules={deny} />
        </div>
      )}
      {showNearMiss && (
        <div className="mt-2">
          <div className="small" style={{ color: SEVERITY_COLOR.caution }}>
            Allowed elsewhere (not this peer) — widen to reach
          </div>
          <RuleGroupList rules={other} />
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
              className="badge text-ink-dark"
              style={{ background: MTLS_VERDICT_COLOR[verdict] ?? '#6c757d' }}
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

// One selecting-policy chip, accented by what it did to this peer's verdict.
// Inert chips dim so the deciding policies (green/red/amber) carry the eye.
function SelectingPolicyChip({ policyRef, state }: { policyRef: PolicyRef; state: ChipState }) {
  const meta = CHIP_STATE[state];
  return (
    <div
      className={`border border-secondary rounded p-2 mb-1 ${s.calloutAccentThin} ${s.smallText} ${meta.muted ? 'opacity-75' : ''}`}
      style={{ '--accent': meta.accent } as CSSProperties}
    >
      <div className="d-flex justify-content-between align-items-center gap-2">
        <span className="fw-semibold text-light text-break">{policyRef.name}</span>
        <div className="d-flex gap-1 align-items-center flex-shrink-0">
          {meta.label && (
            <span className="badge text-ink-dark" style={{ background: meta.accent }}>{meta.label}</span>
          )}
          <ManifestButton kind={policyRef.source} namespace={policyRef.namespace} name={policyRef.name} />
        </div>
      </div>
      <div className="text-secondary">{policyRef.namespace}</div>
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
        {Object.entries(node.labels ?? {}).map(([key, value]) => (
          <span key={key} className={`badge bg-secondary ${s.badgeSm}`}>{key}={value}</span>
        ))}
        {Object.keys(node.labels ?? {}).length === 0 && (
          <span className={`text-secondary ${s.smallText}`}>no labels</span>
        )}
      </div>
      {engines.map(([name, ev]) => {
        // SRC column decides via egress, DST via ingress — join each chip to it.
        const dir = role === 'SRC' ? ev.egress : ev.ingress;
        const policies = ev[policiesKey] ?? [];
        return (
          <div key={name} className="border border-secondary rounded p-2 mb-2">
            <div className="fw-semibold mb-1">{name}</div>
            <div className={`text-secondary ${s.smallText} mb-1`}>Selecting policies</div>
            {policies.length === 0 ? (
              <div className={`text-secondary ${s.smallText}`}>none</div>
            ) : (
              policies.map((policyRef, index) => (
                <SelectingPolicyChip key={index} policyRef={policyRef} state={classifyPolicy(policyRef, dir)} />
              ))
            )}
          </div>
        );
      })}
      {meshEntries.map(([source, membership]) => (
        <MeshSideCard key={source} source={source} membership={membership} />
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
      <div className={s.reachColHeader}>verdict detail</div>
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

// Full reachability region: the pinned-source banner, a loading hint, then the
// tiered result — headline + blocker list + collapsed per-engine breakdown.
export function ReachabilityView({ source, target, loading, result, reverse, reverseError, onClear, onSwap }: {
  source:       WorkloadNode;
  target:       WorkloadNode | null;
  loading:      boolean;
  result:       ReachabilityResult | null;
  reverse:      ReachabilityResult | null;
  reverseError: boolean;
  onClear:      () => void;
  onSwap:       () => void;
}) {
  return (
    <>
      <div className="border border-info rounded p-2 mb-2">
        <div className="d-flex align-items-center gap-2">
          <div className="flex-grow-1 min-w-0">
            <div>
              <span className="badge bg-info text-dark me-2">SRC</span>
              <span className="fw-semibold text-break">{source.label}</span>
            </div>
            <div className={`text-secondary ${s.smallText}`}>
              {source.namespace || '—'} · {source.type}
            </div>
          </div>
          <div className="d-flex align-items-center gap-1 flex-shrink-0">
            <span className="text-secondary">→</span>
            {target && (
              <button
                className="btn btn-sm btn-outline-light py-0 px-1"
                title="Swap SRC and DST"
                onClick={onSwap}
              >
                ⇄
              </button>
            )}
          </div>
          <div className="flex-grow-1 min-w-0">
            {target ? (
              <>
                <div>
                  <span className="badge bg-warning text-dark me-2">DST</span>
                  <span className="fw-semibold text-break">{target.label}</span>
                </div>
                <div className={`text-secondary ${s.smallText}`}>
                  {target.namespace || '—'} · {target.type}
                </div>
              </>
            ) : (
              <div className={`text-secondary ${s.smallText}`}>
                Click another node to check reachability
              </div>
            )}
          </div>
          <button className="btn btn-sm btn-outline-light flex-shrink-0" onClick={onClear}>
            Cancel
          </button>
        </div>
      </div>

      {loading && (
        <div className="text-secondary small mb-2">Computing reachability…</div>
      )}

      {result && target && (
        <>
          <VerdictHeadline result={result} reverse={reverse} reverseError={reverseError} src={source} dst={target} />
          {result.verdict === 'deny' && <BlockerList result={result} src={source} dst={target} />}
          <details>
            <summary className={`text-secondary small fw-semibold mb-2 ${s.detailsSummary}`}>
              Per-engine breakdown
            </summary>
            <div className="mt-2">
              <ReachabilityGrid src={source} dst={target} result={result} />
            </div>
          </details>
        </>
      )}
    </>
  );
}
