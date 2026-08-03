// Reachability pane. Information hierarchy, top to bottom:
//   Tier 0 — verdict headline: can src reach dst, yes/no + one-line reason.
//   Tier 1 — blocker list (deny only): every (engine, direction) that blocks,
//            ranked by remediation, with the culprit policies to edit. AND
//            semantics — the path opens only when every blocker clears.
//   Tier 2 — per-engine/per-direction breakdown + selecting-policy columns,
//            collapsed by default. Proof and audit detail, not the 3am answer.
// Column policy chips are joined back to the verdict (classifyPolicy) so the
// SRC/DST lists show which policy decided, not just the universe selecting it.

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
import { SEVERITY_COLOR, formatPort } from '../../../data/policies';
import s from '../DetailPanel.module.css';
import { PolicyRefList, RuleGroupList } from '../shared/rows';
import { RolePill, LabelStrip, EngineBadge, ActionIcon, DirectionBadge } from '../shared/badges';
import { EndpointCard } from '../shared/edge-composites';
import { ManifestButton } from '../shared/ManifestModal';
import { REASON_META, CHIP_STATE } from '../shared/presentation';
import { MtlsChip } from '../shared/MtlsChip';
import type { MtlsScope } from '../../../data/policies';
import { deriveBlockers, classifyPolicy, aggregateAllowPorts, type Blocker, type ChipState, type SidePorts } from '../shared/reachability';

// Mesh is an in-cluster transport concern (mTLS between workloads). A CIDR node
// is a synthetic ipBlock peer with no pod identity, so mesh status is
// meaningless for it — suppress mesh UI whenever either endpoint is CIDR.
const meshAppliesTo = (src: WorkloadNode, dst: WorkloadNode) =>
  src.type !== 'cidr' && dst.type !== 'cidr';

// ── Tier 0 ──────────────────────────────────────────────────────────────────

// Per-subsystem chip — engine chip + colored ✓/✗ action icon. Reuses the
// token vocab used everywhere else so subsystem verdicts read identically
// across the verdict headline, per-engine grid, and policy cards.
function SubsystemChip({ label, status }: { label: string; status: string }) {
  const action = status === 'allow' ? 'allow' : status === 'deny' ? 'deny' : undefined;
  return (
    <span className={s.subsystemChip}>
      <EngineBadge engine={label} />
      {action ? <ActionIcon action={action} /> : <span className={`${s.smallText} ${s.dim}`}>{status}</span>}
    </span>
  );
}

// Bidirectional summary — pairs forward + reverse verdicts into one reason
// chip so the operator instantly knows whether the pair talks both ways, is
// one-way, or is blocked in both directions. Reverse fetch fires alongside
// the forward one; if it errored, an explicit "reverse unavailable" chip
// surfaces the failure rather than silently showing nothing.
function BidirectionalChip({ forward, reverse, error }: {
  forward: string;
  reverse: string | undefined;
  error:   boolean;
}) {
  if (error) {
    return (
      <span className={`${s.reasonChip} ${s.reasonWarn}`} title="Reverse-direction fetch failed">
        ⚠ reverse unavailable
      </span>
    );
  }
  if (!reverse) return null;
  let label = '';
  let tone: 'allow' | 'warn' | 'deny' | null = null;
  if (forward === 'allow' && reverse === 'allow') {
    label = '↔ bidirectional'; tone = 'allow';
  } else if (forward === 'allow' && reverse === 'deny') {
    label = '→ one-way (reverse blocked)'; tone = 'warn';
  } else if (forward === 'deny' && reverse === 'allow') {
    label = '← reverse only reachable'; tone = 'warn';
  } else if (forward === 'deny' && reverse === 'deny') {
    label = '✗ blocked both ways'; tone = 'deny';
  }
  if (!tone) return null;
  const toneClass = tone === 'allow' ? s.reasonAllow : tone === 'deny' ? s.reasonDeny : s.reasonWarn;
  return <span className={`${s.reasonChip} ${toneClass}`}>{label}</span>;
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
  const allow = result.verdict === 'allow';
  const calloutTone = deny ? s.verdictDeny : allow ? s.verdictAllow : s.verdictWarn;
  const textTone    = deny ? s.verdictTextDeny : allow ? s.verdictTextAllow : s.verdictTextWarn;
  const engines = Object.entries(result.engines);
  const meshes  = result.mesh && meshAppliesTo(src, dst) ? Object.entries(result.mesh) : [];
  return (
    <div className={`${s.verdictCallout} ${calloutTone} mb-2`}>
      <div className="d-flex align-items-baseline gap-2 flex-wrap">
        <span className={`${s.verdictText} ${textTone}`}>
          {deny ? '✗ cannot reach' : '✓ can reach'}
        </span>
        <span className={`${s.section} d-flex align-items-center gap-1 flex-wrap`}>
          <RolePill role="SRC" />
          <span className="text-break">{src.label}</span>
          <span className={s.dim}>→</span>
          <RolePill role="DST" />
          <span className="text-break">{dst.label}</span>
        </span>
        <BidirectionalChip forward={result.verdict} reverse={reverse?.verdict} error={reverseError} />
      </div>
      {(engines.length > 0 || meshes.length > 0) && (
        <div className="d-flex flex-wrap gap-2 align-items-center">
          {engines.map(([name, ev]) => (
            <SubsystemChip key={`e-${name}`} label={name} status={ev.status} />
          ))}
          {meshes.map(([name, v]) => (
            <SubsystemChip key={`m-${name}`} label={`mesh ${name}`} status={v.verdict} />
          ))}
        </div>
      )}
      <div className={deny ? `${s.body} ${textTone} ${s.reasonEmph}` : `${s.dim} ${s.smallText}`}>
        {result.reason}
      </div>
    </div>
  );
}

// ── Ports summary ────────────────────────────────────────────────────────────

// One side's aggregated ports: grey "all ports" badge, per-port badges, or a
// blocked/none marker. Same badge conventions as the Tier-2 rule rows.
function SidePortBadges({ side }: { side: SidePorts }) {
  if (side.blocked) return <span className={s.portChipBlocked}>blocked</span>;
  if (side.allPorts) {
    return (
      <span className={s.portChipAny} title="No port restriction — all TCP/UDP allowed">
        all ports
      </span>
    );
  }
  if (side.ports.length === 0) return <span className={`${s.dim} ${s.smallText}`}>none</span>;
  return (
    <>
      {side.ports.map((port, index) => (
        <span key={index} className={s.portChip}>{formatPort(port)}/{port.protocol}</span>
      ))}
    </>
  );
}

// Per-engine port comparison: what src's egress opens vs what dst's ingress
// accepts. Answers "full port access or a subset?" without expanding Tier 2 —
// the operator reads both sides; the effective set is their intersection.
function PortsSummary({ result }: { result: ReachabilityResult }) {
  const engines = Object.entries(result.engines).filter(([, ev]) => ev.status !== 'not enforced');
  if (engines.length === 0) return null;
  return (
    <div className="d-flex flex-column gap-1 mb-3">
      <div className={s.eyebrow}>Ports</div>
      {engines.map(([name, ev]) => {
        const egress  = aggregateAllowPorts(ev.egress);
        const ingress = aggregateAllowPorts(ev.ingress);
        return (
          <div key={name} className="d-flex align-items-center gap-2 flex-wrap">
            <EngineBadge engine={name} />
            <RolePill role="SRC" />
            <DirectionBadge direction="egress" />
            <span className="d-flex gap-1 flex-wrap"><SidePortBadges side={egress} /></span>
            <span className={s.dim}>→</span>
            <RolePill role="DST" />
            <DirectionBadge direction="ingress" />
            <span className="d-flex gap-1 flex-wrap"><SidePortBadges side={ingress} /></span>
          </div>
        );
      })}
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
  const dirOutlineClass = isMesh
    ? s.dirOutlineMesh
    : blocker.direction === 'egress' ? s.dirOutlineEgress : s.dirOutlineIngress;
  const dirArrow = isMesh ? '🔒' : blocker.direction === 'egress' ? '↑' : '↓';
  const dirLabel = isMesh ? 'mTLS' : blocker.direction;
  return (
    <div className={`${s.card} ${s.cardDeny}`}>
      <div className="d-flex justify-content-between align-items-center gap-2 mb-2 flex-wrap">
        <div className={s.policyMetaRow}>
          <EngineBadge engine={blocker.engine} />
          <span className={`${s.dirOutline} ${dirOutlineClass}`}>{dirArrow} {dirLabel}</span>
          <RolePill role={blocker.side.toUpperCase() as 'SRC' | 'DST'} />
          <span className={`${s.section} text-break`}>{sideNode.label}</span>
        </div>
        <span className={`${s.reasonChip} ${s.reasonDeny}`}>{meta?.label ?? blocker.reason}</span>
      </div>
      {blocker.paSource && (
        <div className={`${s.dim} d-flex align-items-center gap-2 ${s.smallText} mb-1`}>
          <span>Forced by: {blocker.paSource.namespace}/{blocker.paSource.name}</span>
          <ManifestButton kind="pa" namespace={blocker.paSource.namespace} name={blocker.paSource.name} />
        </div>
      )}
      {blocker.culprits.length > 0 && (
        <div className="mt-1">
          <div className={`${s.dim} ${s.smallText} mb-1`}>Edit to allow</div>
          <PolicyRefList items={blocker.culprits} />
        </div>
      )}
      {/* explicit-deny only: a deny rule is actively winning — show it as the
          delete target alongside the culprit. */}
      {blocker.denyRules.length > 0 && (
        <div className="mt-2">
          <div className={`${s.smallText} ${s.eyebrowDeny}`}>Deny rules — delete to allow</div>
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
      <div className={`${s.eyebrow} ${s.eyebrowDeny} mb-2`}>
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
  const sectionTintClass = direction === 'egress' ? s.egressSection : s.ingressSection;
  const arrow   = direction === 'egress' ? '↑' : '↓';
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
  // Reason drives the semantic stripe: block reasons render deny stripe,
  // allow-side (permitted) renders allow stripe, near-miss / unknown fall
  // back to warn tint so all three directions share vocabulary.
  const isDenyReason = dir.reason === 'default-deny' || dir.reason === 'explicit-deny';
  const shellClass = isDenyReason ? s.semanticDeny
                   : dir.reason === 'permitted' ? s.semanticAllow
                   : s.semanticWarn;
  return (
    <div className={`${shellClass} d-flex flex-column gap-1`}>
      <div className="d-flex justify-content-between align-items-center flex-wrap gap-1">
        <span className={`${s.section} ${sectionTintClass}`}>{arrow} {label}</span>
        <span className={`${s.reasonChip} ${isDenyReason ? s.reasonDeny : dir.reason === 'permitted' ? s.reasonAllow : s.reasonWarn}`}>
          {meta.label}
        </span>
      </div>
      {culprits.length > 0 && (
        <div className="d-flex flex-column gap-1">
          <div className={`${s.dim} ${s.smallText}`}>Policies to edit</div>
          <PolicyRefList items={culprits} />
        </div>
      )}
      {denyAll.length > 0 && (
        <div className={`${s.smallText} ${s.eyebrowDeny}`}>
          deny-all lock: {denyAll.map((rule) => rule.contributor?.name).filter(Boolean).join(', ') || 'default-deny'}
        </div>
      )}
      {allow.length > 0 && (
        <div>
          <div className={`${s.smallText} ${s.eyebrowAllow}`}>Allow rules</div>
          <RuleGroupList rules={allow} />
        </div>
      )}
      {deny.length > 0 && (
        <div>
          <div className={`${s.smallText} ${s.eyebrowDeny}`}>Deny rules</div>
          <RuleGroupList rules={deny} />
        </div>
      )}
      {showNearMiss && (
        <div>
          <div className={`${s.smallText} ${s.eyebrowWarn}`}>
            Allowed elsewhere (not this peer) — widen to reach
          </div>
          <RuleGroupList rules={other} />
        </div>
      )}
    </div>
  );
}

function EngineCard({ name, ev }: { name: string; ev: EngineVerdict }) {
  const shellClass = ev.status === 'allow' ? s.semanticAllow
                   : ev.status === 'deny'  ? s.semanticDeny
                   : s.semanticWarn;
  const chipClass  = ev.status === 'allow' ? s.reasonAllow
                   : ev.status === 'deny'  ? s.reasonDeny
                   : s.reasonInert;
  return (
    <div className={`${shellClass} d-flex flex-column gap-2`}>
      <div className="d-flex justify-content-between align-items-center">
        <span className={s.section}>{name}</span>
        <span className={`${s.reasonChip} ${chipClass}`}>{ev.status}</span>
      </div>
      <DirectionBlock label="Egress (src side)"  dir={ev.egress}  direction="egress" />
      <DirectionBlock label="Ingress (dst side)" dir={ev.ingress} direction="ingress" />
    </div>
  );
}

function MeshSideCard({ source, membership }: { source: string; membership: MeshMembership | undefined }) {
  if (!membership) {
    return (
      <div className={`${s.card} ${s.cardWarn}`}>
        <div className="d-flex justify-content-between align-items-center">
          <span className={s.section}>mesh · {source}</span>
          <span className={`${s.miniChip} ${s.miniChipWarn}`} title="Backend did not report mesh membership for this workload">
            mesh status unknown
          </span>
        </div>
      </div>
    );
  }
  const inMesh = membership.inMesh;
  const verdict = membership.mtls?.verdict;
  return (
    <div className={s.card}>
      <div className="d-flex justify-content-between align-items-center mb-1">
        <span className={s.section}>mesh · {source}</span>
        <div className="d-flex gap-1">
          <span className={`${s.miniChip} ${inMesh ? s.miniChipAllow : s.miniChipDim}`}>
            {inMesh ? 'in mesh' : 'not in mesh'}
          </span>
          {verdict && (
            <MtlsChip scope={verdict as MtlsScope} label={`mTLS: ${verdict}`} />
          )}
        </div>
      </div>
      {membership.mtls?.effectiveSource?.name && (
        <div className={`${s.dim} d-flex align-items-center gap-2 ${s.smallText}`}>
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
  const shellClass = state === 'permitter' ? s.semanticAllow
                   : state === 'blocker'   ? s.semanticDeny
                   : state === 'near-miss' ? s.semanticWarn
                   : s.card;
  const chipClass  = state === 'permitter' ? s.reasonAllow
                   : state === 'blocker'   ? s.reasonDeny
                   : state === 'near-miss' ? s.reasonWarn
                   : s.reasonInert;
  return (
    <div className={`${shellClass} ${s.smallText} ${meta.muted ? s.cardMuted : ''}`}>
      <div className="d-flex justify-content-between align-items-center gap-2">
        <span className={`${s.section} text-break`}>{policyRef.name}</span>
        <div className="d-flex gap-1 align-items-center flex-shrink-0">
          {meta.label && <span className={`${s.reasonChip} ${chipClass}`}>{meta.label}</span>}
          <ManifestButton kind={policyRef.source} namespace={policyRef.namespace} name={policyRef.name} />
        </div>
      </div>
      <div className={s.dim}>{policyRef.namespace}</div>
    </div>
  );
}

function WorkloadColumn({ node, role, engines, policiesKey, meshSources, mesh }: {
  node:        WorkloadNode;
  role:        'SRC' | 'DST';
  engines:     [string, EngineVerdict][];
  policiesKey: 'srcPolicies' | 'dstPolicies';
  meshSources: string[];
  mesh?:       Record<string, MeshMembership>;
}) {
  return (
    <div className={s.reachCol}>
      <div className={s.reachColHeader}>
        <div className={s.policyMetaRow}>
          <RolePill role={role} />
          <span className={`${s.section} text-break`}>{node.label}</span>
        </div>
        <div className={`${s.dim} ${s.smallText}`}>
          {node.namespace ? `${node.namespace} · ${node.type}` : node.type}
        </div>
      </div>
      <div className="mb-3">
        <LabelStrip labels={node.labels} collapsible />
      </div>
      {engines.map(([name, ev]) => {
        // SRC column decides via egress, DST via ingress — join each chip to it.
        const dir = role === 'SRC' ? ev.egress : ev.ingress;
        const policies = ev[policiesKey] ?? [];
        return (
          <div key={name} className={s.card}>
            <div className={`${s.section} mb-1`}>{name}</div>
            <div className={`${s.eyebrow} mb-1`}>Selecting policies</div>
            {policies.length === 0 ? (
              <div className={`${s.dim} ${s.smallText}`}>none</div>
            ) : (
              policies.map((policyRef, index) => (
                <SelectingPolicyChip key={index} policyRef={policyRef} state={classifyPolicy(policyRef, dir)} />
              ))
            )}
          </div>
        );
      })}
      {meshSources.map((source) => (
        <MeshSideCard key={source} source={source} membership={mesh?.[source]} />
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
      className={s.card}
      style={{ borderLeftColor: color }}
    >
      <div className="d-flex justify-content-between align-items-center mb-1">
        <span className={s.section}>mesh · {name}</span>
        <span className={s.verdictChip} style={{ background: color }}>{v.verdict}</span>
      </div>
      <div className={`${s.dim} ${s.smallText}`}>{v.reason}</div>
      {v.effectiveSource?.name && (
        <div className={`${s.dim} d-flex align-items-center gap-2 ${s.smallText} mt-1`}>
          <span>Forced by: {v.effectiveSource.namespace}/{v.effectiveSource.name}</span>
          <ManifestButton kind="pa" namespace={v.effectiveSource.namespace} name={v.effectiveSource.name} />
        </div>
      )}
    </div>
  );
}

function ResultColumn({ result, engines, showMesh }: {
  result:  ReachabilityResult;
  engines: [string, EngineVerdict][];
  showMesh: boolean;
}) {
  const meshEntries = result.mesh && showMesh ? Object.entries(result.mesh) : [];
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
  const showMesh = meshAppliesTo(src, dst);
  const meshSources = result.mesh && showMesh ? Object.keys(result.mesh) : [];
  return (
    <div className={s.reachGrid}>
      <WorkloadColumn node={src} role="SRC" engines={engines} policiesKey="srcPolicies" meshSources={meshSources} mesh={result.srcMesh} />
      <WorkloadColumn node={dst} role="DST" engines={engines} policiesKey="dstPolicies" meshSources={meshSources} mesh={result.dstMesh} />
      <ResultColumn result={result} engines={engines} showMesh={showMesh} />
    </div>
  );
}

// Full reachability region: the pinned-source banner, a loading hint, then the
// tiered result — headline + blocker list + collapsed per-engine breakdown.
export function ReachabilityView({ source, target, loading, result, reverse, reverseError, onSwap }: {
  source:       WorkloadNode;
  target:       WorkloadNode | null;
  loading:      boolean;
  result:       ReachabilityResult | null;
  reverse:      ReachabilityResult | null;
  reverseError: boolean;
  onSwap:       () => void;
}) {
  return (
    <>
      {/* SRC → DST endpoint pair — full EndpointCards side-by-side w/ swap
          arrow between. Panel's own close X unpins the source. */}
      <div className={`${s.endpointRow} mb-2`}>
        <EndpointCard role="src" node={source} />
        {target ? (
          <button
            type="button"
            className={s.iconButton}
            title="Swap SRC and DST"
            onClick={onSwap}
          >
            ⇄
          </button>
        ) : (
          <span className={s.endpointArrow}>→</span>
        )}
        {target ? (
          <EndpointCard role="dst" node={target} />
        ) : (
          <div className={`${s.card} ${s.cardFlush} ${s.cardDashed} d-flex align-items-center`}>
            <span className={`${s.dim} ${s.body}`}>
              Click a workload to check reachability
            </span>
          </div>
        )}
      </div>

      {loading && (
        <div className={`${s.dim} ${s.smallText} mb-2`}>Computing reachability…</div>
      )}

      {result && target && (
        <>
          <VerdictHeadline result={result} reverse={reverse} reverseError={reverseError} src={source} dst={target} />
          {result.verdict === 'deny' && <BlockerList result={result} src={source} dst={target} />}
          <PortsSummary result={result} />
          <details>
            <summary className={s.disclosureSummary}>
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
