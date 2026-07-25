// Leaf presentation primitives shared by every DetailPanel view: engine
// provenance chip, direction badge, and the L7 matcher block. No policy/edge
// domain logic here — just badges keyed off a string or a small struct.

import { useState } from 'react';
import type { L7Match } from '../../../data/policies';
import { DIR_COLOR } from './presentation';
import s from '../DetailPanel.module.css';

// Engine provenance chip lives with the logo it wraps. Re-exported here so the
// DetailPanel views that already import it from ./badges keep one import path.
export { EngineBadge } from '../../../data/engineIcons';

// SRC/DST role indicator. Bright solid — spatial role, never competes w/ semantic hue.
export function RolePill({ role }: { role: 'SRC' | 'DST' }) {
  return (
    <span className={`${s.rolePill} ${role === 'SRC' ? s.roleSrc : s.roleDst}`}>{role}</span>
  );
}

// Colored ✓/✗ square for policy action. Pre-attentive at scroll speed vs text badge.
export function ActionIcon({ action }: { action: 'allow' | 'deny' }) {
  return (
    <span
      className={`${s.actionIcon} ${action === 'allow' ? s.actionAllow : s.actionDeny}`}
      title={action}
      aria-label={action}
    >
      {action === 'allow' ? '✓' : '✗'}
    </span>
  );
}

// Provenance/tooling labels operators don't select on. Filtered out of the
// primary strip; surfaced behind a disclosure so they stay accessible.
const SYSTEM_LABEL_PREFIXES = [
  'app.kubernetes.io/',
  'helm.sh/',
  'meta.helm.sh/',
  'kubernetes.io/',
  'k8s.io/',
  'argocd.argoproj.io/',
];
export function isSystemLabel(key: string): boolean {
  return SYSTEM_LABEL_PREFIXES.some((prefix) => key.startsWith(prefix));
}

// Click chip = copy `key=value`; button = copy full `k1=v1,k2=v2` selector for
// `kubectl -l`. Zero-chrome affordance — chip itself is the click target.
function LabelChip({ k, v }: { k: string; v: string }) {
  const copy = () => navigator.clipboard?.writeText(`${k}=${v}`);
  return (
    <button
      type="button"
      className={s.labelChip}
      onClick={copy}
      title={`Click to copy \`${k}=${v}\``}
    >
      <span className={s.labelKey}>{k}</span>
      <span className={s.labelEq}>=</span>
      <span className={s.labelValue}>{v}</span>
    </button>
  );
}

// LabelStrip renders a set of k8s labels as click-to-copy chips + a
// "copy selector" button. System labels folded behind a "Show N system labels"
// disclosure so the primary strip stays tuned to selector-currency labels.
//
// `collapsible`: when true, selector chips themselves fold behind a `▸ N labels`
// summary once the count crosses `defaultOpenThreshold`. Used on Edge/Compare
// panels where two endpoints' full strips create a wall of labels that pushes
// verdict/policy evidence below the fold. Node panel leaves this off — labels
// are the workload's primary identity, earn inline space.
export function LabelStrip({
  labels,
  collapsible = false,
  defaultOpenThreshold = 3,
}: {
  labels: Record<string, string> | undefined;
  collapsible?: boolean;
  defaultOpenThreshold?: number;
}) {
  const entries = Object.entries(labels ?? {});
  const selector: Array<[string, string]> = [];
  const system:   Array<[string, string]> = [];
  for (const [key, value] of entries) {
    (isSystemLabel(key) ? system : selector).push([key, value]);
  }
  const startsOpen = !collapsible || selector.length < defaultOpenThreshold;
  const [open, setOpen] = useState(startsOpen);
  const [showSystem, setShowSystem] = useState(false);
  if (entries.length === 0) {
    return null;
  }
  const selectorStr = selector.map(([k, v]) => `${k}=${v}`).join(',');
  const copyAll = () => navigator.clipboard?.writeText(selectorStr);
  const summary = collapsible && !open && selector.length > 0 && (
    <div className={s.labelStrip}>
      <button
        type="button"
        className={s.systemLabelToggle}
        onClick={() => setOpen(true)}
        title="Expand label chips"
      >
        ▸ {selector.length} label{selector.length === 1 ? '' : 's'}
      </button>
      <button type="button" className={s.labelCopyAll} onClick={copyAll} title={`Copy selector: ${selectorStr}`}>
        copy selector
      </button>
    </div>
  );
  if (summary) return summary;
  return (
    <div className={s.labelStrip} title="Click any chip to copy `key=value`; click `copy selector` to copy the full `k1=v1,k2=v2` string for `kubectl -l`">
      {collapsible && (
        <button
          type="button"
          className={s.systemLabelToggle}
          onClick={() => setOpen(false)}
          title="Collapse label chips"
        >
          ▾
        </button>
      )}
      {selector.map(([key, value]) => <LabelChip key={key} k={key} v={value} />)}
      {selector.length > 0 && (
        <button type="button" className={s.labelCopyAll} onClick={copyAll} title={`Copy selector: ${selectorStr}`}>
          copy selector
        </button>
      )}
      {system.length > 0 && (
        <button
          type="button"
          className={s.systemLabelToggle}
          onClick={() => setShowSystem((prev) => !prev)}
        >
          {showSystem ? `Hide ${system.length} system label${system.length === 1 ? '' : 's'}` : `Show ${system.length} system label${system.length === 1 ? '' : 's'}`}
        </button>
      )}
      {showSystem && system.map(([key, value]) => <LabelChip key={key} k={key} v={value} />)}
    </div>
  );
}

// Coverage posture chip — the rule's blanket state. 'restricted' (specific
// peers, the default) renders nothing; only the notable postures get a chip so
// a deny-all / allow-all jumps out. Token-tinted mono; unenforced reads as
// neutral inert (present but does nothing).
const COVERAGE_META: Record<string, { cls: string; text: string; title: string }> = {
  'deny all':     { cls: s.coverageDeny,  text: 'deny all',     title: 'Policy denies all traffic in this direction' },
  'allow all':    { cls: s.coverageWarn,  text: 'allow all',    title: 'Policy allows all traffic in this direction — no restriction' },
  'allow all ns': { cls: s.coverageWarn,  text: 'allow all ns', title: 'Allows every workload in the peer namespace' },
  'unenforced':   { cls: s.coverageInert, text: '⊘ no effect',  title: 'Policy exists but selects no traffic — nothing is blocked or allowed. Likely a dead or misconfigured control.' },
};

export function CoverageBadge({ coverage }: { coverage?: string }) {
  if (!coverage) return null;
  const meta = COVERAGE_META[coverage];
  if (!meta) return null; // 'restricted' / unknown → default, no chip
  return <span className={`${s.coverageChip} ${meta.cls}`} title={meta.title}>{meta.text}</span>;
}

// Colored arrow+word for ingress/egress. Token classes only, no inline color.
// Callers in the DetailPanel use this in place of the Bootstrap DirectionBadge.
export function DirectionArrow({ direction }: { direction: string }) {
  const isEgress = direction === 'egress';
  return (
    <span className={`${s.dirArrow} ${isEgress ? s.dirEgress : s.dirIngress}`}>
      {isEgress ? '↑' : '↓'} {direction}
    </span>
  );
}

// Direction outline chip — role/mesh-tinted border + arrow. Compact enough
// for table cells (PoliciesTable) and header meta strips (PolicyHeader,
// AffectedPairsList). DetailPanel body rows use `DirectionArrow` for the
// bolder colored-text render.
export function DirectionBadge({ direction }: { direction: string }) {
  const variant = direction === 'egress'  ? s.dirOutlineEgress
                : direction === 'ingress' ? s.dirOutlineIngress
                : direction === 'mesh'    ? s.dirOutlineMesh
                : direction === 'both'    ? s.dirOutlineBoth
                : '';
  const arrow = DIR_COLOR[direction]?.arrow ?? '';
  return (
    <span className={`${s.dirOutline} ${variant}`}>
      {arrow} {direction}
    </span>
  );
}

// L7 matchers contribute six possible field sets (hosts/methods/paths +
// their notX exclusions). Render only the ones a policy actually populated
// so empty rules don't litter the panel. Token chips: positive fields
// info-tinted mono, negated fields deny-tinted + line-through so silent
// Istio exclusions read as removals not additions.
export function L7Block({ blocks }: { blocks: L7Match[] }) {
  const nonEmpty = blocks.filter(
    (b) =>
      (b.hosts?.length ?? 0) +
        (b.methods?.length ?? 0) +
        (b.paths?.length ?? 0) +
        (b.notHosts?.length ?? 0) +
        (b.notMethods?.length ?? 0) +
        (b.notPaths?.length ?? 0) >
      0,
  );
  if (nonEmpty.length === 0) return null;

  const fieldRow = (label: string, items?: string[], negate = false) => {
    if (!items || items.length === 0) return null;
    return (
      <div className="d-flex flex-wrap align-items-center gap-1">
        <span className={s.l7Key}>{negate ? `not ${label}` : label}</span>
        {items.map((v, i) => (
          <span key={i} className={`${s.l7Chip} ${negate ? s.l7ChipNeg : ''}`}>{v}</span>
        ))}
      </div>
    );
  };

  return (
    <div className={s.l7Block} style={{ marginTop: 6 }}>
      <div className={s.eyebrow}>L7 match</div>
      {nonEmpty.map((block, blockIndex) => (
        <div key={blockIndex} className="d-flex flex-column gap-1">
          {fieldRow('hosts', block.hosts)}
          {fieldRow('methods', block.methods)}
          {fieldRow('paths', block.paths)}
          {fieldRow('hosts', block.notHosts, true)}
          {fieldRow('methods', block.notMethods, true)}
          {fieldRow('paths', block.notPaths, true)}
        </div>
      ))}
    </div>
  );
}
