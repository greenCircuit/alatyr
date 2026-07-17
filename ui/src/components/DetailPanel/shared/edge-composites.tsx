// Edge/connection composites: the higher-altitude pieces the edge views build
// from. Endpoint cards, the shared policy header + affected-pairs list for
// Policies-table bundles, the cross-engine reachability banner, and the
// pair-grouping helper they all key off.

import type { Coverage, PolicyEdge, WorkloadNode, ReachabilityResult } from '../../../data/policies';
import { formatPort, realPorts, SEVERITY_COLOR } from '../../../data/policies';
import { DirectionBadge, EngineBadge, RolePill, LabelStrip, ActionIcon } from './badges';
import { ManifestButton } from './ManifestModal';
import type { PairGroup } from './groupEdgesByPair';
import s from '../DetailPanel.module.css';

// Coverage values that mean "no peer restriction" — the edge intentionally has
// no target workload. Distinguishes wildcard rules from a target that failed
// to resolve, so the endpoint card can label it honestly.
const WILDCARD_COVERAGES: Coverage[] = ['allow all', 'allow all ns', 'deny all', 'unenforced'];

function wildcardLabel(coverage: Coverage): { title: string; subtitle: string } {
  switch (coverage) {
    case 'allow all':    return { title: 'any peer',            subtitle: 'no peer restriction' };
    case 'allow all ns': return { title: 'any workload in ns',  subtitle: 'namespace-wide selector' };
    case 'deny all':     return { title: 'no peer',             subtitle: 'default-deny' };
    case 'unenforced':   return { title: 'no peer',             subtitle: 'rule unenforced' };
    default:             return { title: 'any peer',            subtitle: '—' };
  }
}

// EndpointCard shows one side of the edge: name, ns, and the workload's k8s
// labels. Lives once at the connection-panel level — the endpoints are shared
// across every policy in the bundle, so rendering per-row would duplicate.
// coverage is threaded through so a wildcard-target edge (port-only egress,
// deny-all lock, etc.) reads as "any peer" instead of the confused "dst / —"
// placeholder we'd get from a missing workload lookup.
export function EndpointCard({
  role,
  node,
  coverage,
}: {
  role: 'src' | 'dst';
  node: WorkloadNode | undefined;
  coverage?: Coverage;
}) {
  const roleUpper = role.toUpperCase() as 'SRC' | 'DST';
  // Namespace-type endpoints have no `namespace` field of their own — show
  // their kind in that slot so the row reads "ns-foo · namespace" instead of
  // "ns-foo / —" (which looks like missing data).
  const isNs = node?.type === 'namespace';
  const isWildcard = !node && !!coverage && WILDCARD_COVERAGES.includes(coverage);
  if (isWildcard) {
    const { title, subtitle } = wildcardLabel(coverage!);
    return (
      <div className={`${s.card} ${s.cardFlush} ${s.cardDashed} d-flex flex-column gap-1`}>
        <div className={s.policyMetaRow}>
          <RolePill role={roleUpper} />
          <span className={`${s.section} ${s.catchAll}`}>{title}</span>
        </div>
        <div className={`${s.dim} ${s.smallText}`}>{subtitle}</div>
      </div>
    );
  }
  const subtitle = isNs ? 'namespace' : (node?.namespace || '—');
  // Ordering: RolePill + name (section) → LabelStrip (selector currency) →
  // ns · type (dim). Labels sit at line-2 because operator's "why does this
  // policy select this workload?" workflow starts on labels. Mockup shape.
  return (
    <div className={`${s.card} ${s.cardFlush} d-flex flex-column gap-1`}>
      <div className={s.policyMetaRow}>
        <RolePill role={roleUpper} />
        <span className={`${s.section} text-break`}>{node?.label ?? role}</span>
      </div>
      <LabelStrip labels={node?.labels} collapsible />
      <div className={`${s.dim} ${s.smallText}`}>{subtitle}{node?.type ? ` · ${node.type}` : ''}</div>
    </div>
  );
}

// Engine status → severity color. "not enforced" reads as caution because it
// means this engine has zero policies on the workload — not the same as allow.
function engineStatusColor(status: string): string {
  if (status === 'allow') return SEVERITY_COLOR.secure;
  if (status === 'deny')  return SEVERITY_COLOR.high;
  return SEVERITY_COLOR.caution;
}

// Banner above edge PolicyRows: the combined src→dst verdict from
// /api/reachable. The arrow on canvas is one engine's opinion; this row is
// the cross-engine + mesh truth. When they disagree, this is the "lying graph"
// signal — Istio shows allow but k8s NetPol or mesh mTLS actually blocks.
export function EdgeReachabilityBanner({
  result,
  loading,
}: {
  result: ReachabilityResult | null;
  loading: boolean;
}) {
  if (loading) {
    return (
      <div className={`${s.card} ${s.dim} ${s.smallText}`}>
        Computing reachability…
      </div>
    );
  }
  if (!result) return null;
  const engineEntries = Object.entries(result.engines ?? {});
  const meshEntries = Object.entries(result.mesh ?? {});
  const deny = result.verdict === 'deny';
  const allow = result.verdict === 'allow';
  const calloutTone = deny ? s.verdictDeny : allow ? s.verdictAllow : s.verdictWarn;
  const textTone    = deny ? s.verdictTextDeny : allow ? s.verdictTextAllow : s.verdictTextWarn;
  // Hero verdict callout — full-tint bg + 4px stripe, verdictText at hero size.
  // STYLEGUIDE §3 exception: hero is one-per-panel, full tint legal.
  return (
    <div className={`${s.verdictCallout} ${calloutTone}`}>
      <div className={`${s.verdictText} ${textTone}`}>
        {deny ? '✗ cannot reach' : '→ can reach'}
      </div>
      <div className={`${s.body} ${deny ? textTone : s.dim}`} style={deny ? { fontWeight: 600 } : undefined}>
        {result.reason}
      </div>
      {engineEntries.length > 0 && (
        <div className={`${s.policyMetaRow} mb-1`}>
          <span className={s.eyebrow}>Engines</span>
          {engineEntries.map(([name, ev]) => (
            <span
              key={name}
              className={s.verdictChip}
              style={{ background: engineStatusColor(ev.status) }}
              title={ev.status}
            >
              {name}: {ev.status}
            </span>
          ))}
        </div>
      )}
      {meshEntries.length > 0 && (
        <div className={s.policyMetaRow}>
          <span className={s.eyebrow}>Mesh</span>
          {meshEntries.map(([name, mv]) => (
            <span
              key={name}
              className={s.verdictChip}
              style={{ background: engineStatusColor(mv.verdict) }}
              title={mv.reason}
            >
              {name}: {mv.verdict}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

// Header shown above AffectedPairsList when the selected bundle came from a
// Policies-table click (one policy, many pairs). All rows share policy name +
// engine + ns + action, so render that once instead of repeating per pair.
export function PolicyHeader({ edges }: { edges: PolicyEdge[] }) {
  if (edges.length === 0) return null;
  const first = edges[0];
  const isDeny = first.action === 1;
  const directions = new Set(edges.map((e) => e.direction));
  return (
    <div className={`${s.card} ${isDeny ? s.cardDeny : ''} ${s.smallText}`}>
      <div className={s.policyMetaRow}>
        <ActionIcon action={isDeny ? 'deny' : 'allow'} />
        <span className={`${s.section} text-break ${s.flexFill}`}>{first.policyName}</span>
        <EngineBadge engine={first.policySource} />
        <span className={s.dim}>{first.namespace}</span>
        <ManifestButton kind={first.policySource} namespace={first.namespace} name={first.policyName} />
      </div>
      <div className={s.policyMetaRow} style={{ marginTop: 4 }}>
        {[...directions].map((d) => <DirectionBadge key={d} direction={d} />)}
      </div>
    </div>
  );
}

// Inline endpoint label for a pair-list row. Same wildcard handling as
// EndpointCard so a port-only rule surfaced from the Policies table renders
// consistently in the affected-workloads list.
function PairEndpoint({
  node,
  rawId,
  coverage,
}: {
  node:     WorkloadNode | undefined;
  rawId:    string;
  coverage: Coverage | undefined;
}) {
  if (!node && coverage && WILDCARD_COVERAGES.includes(coverage)) {
    const { title } = wildcardLabel(coverage);
    return <span className={`${s.section} ${s.catchAll}`}>{title}</span>;
  }
  return (
    <>
      <span className={`${s.section} text-break`}>{node?.label ?? rawId}</span>
      <span className={s.dim}>/ {node?.namespace || '—'}</span>
    </>
  );
}

function uniquePortStrings(edges: PolicyEdge[]): string[] {
  const seen = new Set<string>();
  for (const e of edges) {
    for (const p of realPorts(e.ports) ?? []) {
      seen.add(`${formatPort(p)}/${p.protocol}`);
    }
  }
  return [...seen];
}

// Affected-workloads list shown when the selected bundle spans multiple
// (src, dst) pairs — typical when the user clicked a Policies-table row and
// pulled in every edge that policy produces. Each pair row narrows selection
// back to a single pair so the standard endpoint cards + reachability render.
export function AffectedPairsList({
  pairs,
  nodes,
  onSelect,
}: {
  pairs: PairGroup[];
  nodes: WorkloadNode[];
  onSelect: (edges: PolicyEdge[]) => void;
}) {
  const nodeById = new Map(nodes.map((n) => [n.id, n]));
  return (
    <div className="mb-3">
      <div className={`${s.eyebrow} mb-1`}>
        Affected workloads ({pairs.length})
      </div>
      <div className="d-flex flex-column gap-1">
        {pairs.map((pair) => {
          const src = nodeById.get(pair.src);
          const dst = nodeById.get(pair.dst);
          const directions = new Set(pair.edges.map((e) => e.direction));
          const portChips = uniquePortStrings(pair.edges);
          return (
            <button
              key={pair.key}
              type="button"
              className={`${s.card} ${s.cardButton} ${s.smallText}`}
              onClick={() => onSelect(pair.edges)}
            >
              <div className="d-flex align-items-center gap-2 flex-wrap">
                <RolePill role="SRC" />
                <PairEndpoint node={src} rawId={pair.src} coverage={pair.edges[0]?.coverage} />
                <span className={s.dim}>→</span>
                <RolePill role="DST" />
                <PairEndpoint node={dst} rawId={pair.dst} coverage={pair.edges[0]?.coverage} />
              </div>
              <div className="d-flex gap-2 flex-wrap mt-1">
                {[...directions].map((d) => <DirectionBadge key={d} direction={d} />)}
                {pair.edges.some((e) => (e.l7Matches?.length ?? 0) > 0) && (
                  <span className={s.miniChip} title="L7 matchers present — click to drill in">L7</span>
                )}
              </div>
              <div className="d-flex gap-2 flex-wrap mt-1">
                {portChips.length === 0
                  ? <span className={s.portChipAny} title="Policy does not restrict ports — all TCP/UDP allowed">any port</span>
                  : portChips.map((p, i) => (
                      <span key={i} className={s.portChip}>{p}</span>
                    ))}
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}
