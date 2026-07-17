// Culprit remediation block, shared by the Issues table + popover. Speaks the
// SAME language as the reachability panel's BlockerRow so panel↔table is a
// coherent hand-off: the arrowed direction pill + REASON_META cause badge match
// the panel verbatim, and the SRC/DST side pill (the one filled badge — it names
// the endpoint to edit) reuses the panel's endpoint colors. Cause reads first
// (colored reason badge), then the imperative fix the panel only implies.

import type { Issue, PolicyRef, DirectionReason } from '../../data/policies';
import { REASON_META, type ReasonTone } from '../DetailPanel/shared/presentation';
import { EngineBadge, EngineLogo } from '../../data/engineIcons';
import { ManifestButton } from '../DetailPanel/shared/ManifestModal';
import { useManifestStore } from '../../store/manifestStore';
import s from '../DetailPanel/DetailPanel.module.css';

type Direction = 'egress' | 'ingress';

// Which endpoint owns the fix, per direction. Egress blocks leave the source;
// ingress blocks land on the destination. Role tokens (SRC=blue / DST=amber)
// match the reachability panel's endpoint identity.
const DIRECTION_SIDE: Record<Direction, { role: 'SRC' | 'DST' }> = {
  egress:  { role: 'SRC' },
  ingress: { role: 'DST' },
};

const REASON_TONE_CLASS: Record<ReasonTone, string> = {
  allow: s.reasonAllow,
  deny:  s.reasonDeny,
  warn:  s.reasonWarn,
  inert: s.reasonInert,
};

// Imperative the panel only implies ("Edit to allow" / "delete the deny"). Kept
// as the secondary line under the shared reason badge — the badge is the cause,
// this is the action. Non-block reasons never render a group, so they're absent.
const REASON_VERB: Partial<Record<DirectionReason, string>> = {
  'default-deny':    'Add an allow rule',
  'locked-no-match': 'Widen the selector to include this peer',
  'explicit-deny':   'Remove the deny',
};

const endpointLabel = (issue: Issue, direction: Direction): string => {
  const node = direction === 'egress' ? (issue.src ?? issue.node) : (issue.dst ?? issue.node);
  return node?.label ?? node?.id ?? '';
};

// A direction is actionable when it names culprits OR its reason is a block —
// default-deny often has no policy ref to point at (the block is absence of a
// rule), and we still want to surface it as a finding, not hide it.
const isBlockReason = (reason?: DirectionReason): boolean =>
  reason === 'default-deny' || reason === 'locked-no-match' || reason === 'explicit-deny';

// Clickable policy chip — name badge opens the policy in the detail panel; the
// adjacent YAML button jumps straight to the raw manifest drawer (one-click,
// skips the edge→YAML hop). Reused for both block culprits and permitted-side
// allow refs. source doubles as the manifest kind ("k8s" | "istio" | "pa").
function PolicyChip({ policy, onOpen, showEngine }: {
  policy: PolicyRef;
  onOpen: (source: string, namespace: string, name: string) => boolean;
  // Allowed-side chips carry their own engine mark — the row-level EngineBadge
  // names the blocking engine, which can differ from the one granting access.
  showEngine?: boolean;
}) {
  const openManifest = useManifestStore((store) => store.open);
  // Default-deny / lockout policies emit no allow edges, so edge selection can
  // come back empty. Fall back to the manifest drawer — a chip that promises a
  // click and silently no-ops burns trust exactly on the issue classes that
  // matter most.
  const openPolicyOrManifest = () => {
    const selected = onOpen(policy.source, policy.namespace, policy.name);
    if (!selected) {
      openManifest({ kind: policy.source, namespace: policy.namespace, name: policy.name });
    }
  };
  return (
    <span className="d-inline-flex align-items-center gap-1" onClick={(event) => event.stopPropagation()}>
      <button
        type="button"
        className="chip-label text-truncate max-w-180"
        style={{ cursor: 'pointer' }}
        title={`${policy.source} · ${policy.namespace}/${policy.name}`}
        onClick={openPolicyOrManifest}
      >
        <span className="d-inline-flex align-items-center gap-1">
          {showEngine && <EngineLogo engine={policy.source} size={11} />}
          {policy.name}
        </span>
      </button>
      <ManifestButton kind={policy.source} namespace={policy.namespace} name={policy.name} />
    </span>
  );
}

function CulpritGroup({ direction, culprits, allowed, reason, endpoint, engines, layering, onOpen }: {
  direction: Direction;
  culprits:  PolicyRef[];
  allowed:   PolicyRef[];
  reason?:   DirectionReason;
  endpoint:  string;
  engines:   string[];
  layering:  boolean;
  onOpen:    (source: string, namespace: string, name: string) => boolean;
}) {
  const side = DIRECTION_SIDE[direction];
  const meta = reason ? REASON_META[reason] : undefined;
  const verb = reason ? REASON_VERB[reason] : undefined;
  const dirOutlineClass = direction === 'egress' ? s.dirOutlineEgress : s.dirOutlineIngress;
  const dirArrow = direction === 'egress' ? '↑' : '↓';
  const rolePillClass = side.role === 'SRC' ? s.roleSrc : s.roleDst;
  // A permitted direction is the SATISFIED side of the conflict — show it muted
  // as confirmation (✓ allowed by <policy>) so the operator sees both sides here
  // instead of digging through the panel to learn the other side is already fine.
  const permitted = reason === 'permitted';
  return (
    <div className="d-flex align-items-center flex-wrap gap-1 my-1">
      {/* Engine mark leads the subrow — names which policy engine owns the fix
          for THIS direction (k8s egress vs istio ingress on a merged pair), so
          the badge sits next to the culprit it points at, not in a detached
          column. Same logo+name badge as the Policies table's Engine cell. */}
      {engines.map((engine) => (
        <EngineBadge key={engine} engine={engine} />
      ))}
      {/* Direction: thin role-tinted outline + arrow — an accent, not a filled
          badge, so the only saturated pill is the SRC/DST side that names the
          fix target. Role tokens keep hue vocabulary consistent w/ DirectionArrow. */}
      <span className={`${s.dirOutline} ${dirOutlineClass}`}>
        {dirArrow} {direction}
      </span>
      {/* The filled SRC/DST pill names the endpoint to EDIT — layering rows have
          nothing to fix, so the pill would misdirect. */}
      {!layering && (
        <span className={`${s.rolePill} ${rolePillClass}`}>
          {side.role}{endpoint && ` · ${endpoint}`}
        </span>
      )}
      {permitted ? (
        // Satisfied side — green check + the policy that already admits this peer.
        <>
          <span className={`${s.body} text-allow`} style={{ fontWeight: 600 }}>
            ✓ allowed{allowed.length === 0 && ' (no policy governs)'}
          </span>
          {allowed.length > 0 && <span className={`${s.smallText} ${s.dim}`}>by</span>}
          {allowed.map((policy) => (
            <PolicyChip key={`${policy.source}|${policy.namespace}|${policy.name}`} policy={policy} onOpen={onOpen} showEngine />
          ))}
        </>
      ) : layering ? (
        // Layering (partial access): expected two-tier composition, not a fault.
        // No reason badge, no fix imperative — those frame design as breakage
        // ("widen the selector" would tell the operator to open their baseline).
        // Reads coarse-first: the ns-level allow works, narrowed by the fine tier.
        <>
          <span
            className={`${s.reasonChip} ${s.reasonInert}`}
            title="Coarse ns-level allow intentionally narrowed by a pod-level policy — nothing to fix"
          >
            Layered
          </span>
          {/* Lead-in renders unconditionally — a bare "narrowed by <chip>" with
              no framing reads like a finding. Chips only when refs exist. */}
          <span className={`${s.body} text-allow`} style={{ fontWeight: 600 }}>
            ✓ ns-level allow verified at pod level
          </span>
          {allowed.map((policy) => (
            <PolicyChip key={`${policy.source}|${policy.namespace}|${policy.name}`} policy={policy} onOpen={onOpen} showEngine />
          ))}
          <span className={`${s.smallText} ${s.dim}`}>narrowed to specific pods by</span>
          {culprits.map((culprit) => (
            <PolicyChip key={`${culprit.source}|${culprit.namespace}|${culprit.name}`} policy={culprit} onOpen={onOpen} />
          ))}
        </>
      ) : (
        <>
          {/* Cause — the shared reason chip, token-tinted per tone. */}
          {meta && <span className={`${s.reasonChip} ${REASON_TONE_CLASS[meta.tone]}`}>{meta.label}</span>}
          {/* Action — the imperative, subordinate to the cause chip. */}
          {verb && <span className={`${s.smallText} ${s.dim}`}>→ {verb}</span>}
          {culprits.map((culprit) => (
            <PolicyChip key={`${culprit.source}|${culprit.namespace}|${culprit.name}`} policy={culprit} onOpen={onOpen} />
          ))}
        </>
      )}
    </div>
  );
}

export function CulpritActions({ issue, onOpen }: {
  issue:  Issue;
  onOpen: (source: string, namespace: string, name: string) => boolean;
}) {
  const egress    = issue.egressCulprits ?? [];
  const ingress   = issue.ingressCulprits ?? [];
  const egressAllowed  = issue.egressAllowed ?? [];
  const ingressAllowed = issue.ingressAllowed ?? [];
  const layering = issue.type === 'partial access';
  // Show a direction when it blocks (culprit/block reason) OR when it's the
  // permitted side of the conflict — the covered side is now confirmation, not
  // noise, so the operator sees both halves of the path in the row.
  const showEgress  = egress.length > 0 || isBlockReason(issue.egressReason) || issue.egressReason === 'permitted';
  const showIngress = ingress.length > 0 || isBlockReason(issue.ingressReason) || issue.ingressReason === 'permitted';
  if (!showEgress && !showIngress) {
    return <span className={s.dim}>—</span>;
  }
  // Engines behind a merged same-pair row (k8s egress + istio ingress). Prefer
  // the culprits' own source; when a block direction has no policy ref to point
  // at (default-deny), fall back to the issue's contributing engines.
  const issueEngines = issue.engines?.length ? issue.engines : (issue.engine ? [issue.engine] : []);
  const groupEngines = (culprits: PolicyRef[]) => {
    const sources = [...new Set(culprits.map((culprit) => culprit.source))];
    return sources.length > 0 ? sources : issueEngines;
  };
  return (
    <div className="d-flex flex-column gap-1 mb-2">
      {showEgress && (
        <CulpritGroup
          direction="egress"
          culprits={egress}
          allowed={egressAllowed}
          reason={issue.egressReason}
          endpoint={endpointLabel(issue, 'egress')}
          engines={groupEngines(egress.length > 0 ? egress : egressAllowed)}
          layering={layering}
          onOpen={onOpen}
        />
      )}
      {showIngress && (
        <CulpritGroup
          direction="ingress"
          culprits={ingress}
          allowed={ingressAllowed}
          reason={issue.ingressReason}
          endpoint={endpointLabel(issue, 'ingress')}
          engines={groupEngines(ingress.length > 0 ? ingress : ingressAllowed)}
          layering={layering}
          onOpen={onOpen}
        />
      )}
    </div>
  );
}
