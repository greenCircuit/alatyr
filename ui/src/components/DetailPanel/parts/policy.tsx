// Policy + rule rendering: rows for PolicyEdge (graph edges), NodeRule
// (outbound/inbound), PolicyRef (selecting policies), and the L7 matcher
// block they share. DirectionBadge lives here too because policy/rule rows
// are its primary consumers — reachability.tsx imports it for DirectionBlock.

import type { PolicyEdge, NodeRule, PolicyRef, L7Match, WorkloadNode, ReachabilityResult } from '../../../data/policies';
import { formatPort, realPorts, SEVERITY_COLOR } from '../../../data/policies';
import { engineMeta } from '../../../data/engines';
import { EngineLogo } from '../../../data/engineIcons';
import { verdictColor } from './status';
import s from '../DetailPanel.module.css';

// Engine provenance chip — brand logo + name. Shared by the edge panel and the
// workload panel's per-engine cards so the same brand-colored mark that rides
// the arrows also labels the panels. Border (not fill) carries the brand color
// so the logo keeps its own color on the dark chip.
export function EngineBadge({ engine }: { engine: string }) {
  const { label, color } = engineMeta(engine);
  return (
    <span
      className="badge d-inline-flex align-items-center gap-1"
      title={label}
      style={{ background: '#11151a', border: `1px solid ${color}`, color: '#e9ecef' }}
    >
      <EngineLogo engine={engine} size={12} /> {engine}
    </span>
  );
}

// Match graph arrow colors so panel → canvas is a visual hand-off.
// Same hex values as style/edgeStyles.ts.
export const DIR_COLOR: Record<string, { tint: string; arrow: string }> = {
  egress:  { tint: '#4dabf7', arrow: '↑' },
  ingress: { tint: '#f783ac', arrow: '↓' },
  both:    { tint: '#a9e34b', arrow: '↕' },
};

export function DirectionBadge({ direction }: { direction: string }) {
  const { tint, arrow } = DIR_COLOR[direction] ?? { tint: '#adb5bd', arrow: '' };
  return (
    <span className="badge" style={{ background: tint, color: '#1a1d20' }}>
      {arrow} {direction}
    </span>
  );
}

// L7 matchers contribute six possible field sets (hosts/methods/paths +
// their notX exclusions). Render only the ones a policy actually populated
// so empty rules don't litter the panel.
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
      <div className="d-flex flex-column align-items-start gap-1 mt-1">
        <div className="text-secondary">{negate ? `not ${label}` : label}</div>
        <div className="d-flex flex-wrap gap-1">
          {items.map((v, i) => (
            <span
              key={i}
              className={`badge ${negate ? 'bg-danger' : 'bg-primary'} text-light`}
            >
              {v}
            </span>
          ))}
        </div>
      </div>
    );
  };

  return (
    <div className="mt-2">
      <div className="text-secondary">L7</div>
      {nonEmpty.map((block, blockIndex) => (
        <div
          key={blockIndex}
          className="border border-secondary rounded p-2 mt-1"
        >
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

// EndpointCard shows one side of the edge: name, ns, and the workload's k8s
// labels. Lives once at the connection-panel level — the endpoints are shared
// across every policy in the bundle, so rendering per-row would duplicate.
export function EndpointCard({
  role,
  node,
}: {
  role: 'src' | 'dst';
  node: WorkloadNode | undefined;
}) {
  const badge = role === 'src' ? 'SRC' : 'DST';
  const roleColor = role === 'src' ? 'bg-info' : 'bg-warning';
  const labelEntries = Object.entries(node?.labels ?? {});
  // Namespace-type endpoints have no `namespace` field of their own — show
  // their kind in that slot so the row reads "ns-foo / namespace" instead of
  // "ns-foo / —" (which looks like missing data).
  const isNs = node?.type === 'namespace';
  const subtitle = isNs ? 'namespace' : (node?.namespace || '—');
  return (
    <div className="border border-secondary rounded p-2 d-flex flex-column gap-1">
      <div className="d-flex align-items-center gap-2">
        <span className={`badge ${roleColor} text-dark flex-shrink-0`}>{badge}</span>
        <span className="fw-semibold text-light text-break">{node?.label ?? role}</span>
        <span className="text-secondary">/ {subtitle}</span>
      </div>
      <div>
        <div className="text-secondary">Labels</div>
        <div className="d-flex flex-wrap gap-1 mt-1">
          {labelEntries.length === 0
            ? <span className="text-secondary">—</span>
            : labelEntries.map(([k, v]) => (
                <span key={k} className={`badge bg-secondary ${s.badgeSm}`}>{k}={v}</span>
              ))}
        </div>
      </div>
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
      <div className={`border border-secondary rounded p-2 mb-2 text-secondary ${s.smallText}`}>
        Computing reachability…
      </div>
    );
  }
  if (!result) return null;
  const verdictBg = verdictColor(result.verdict);
  const engineEntries = Object.entries(result.engines ?? {});
  const meshEntries = Object.entries(result.mesh ?? {});
  return (
    <div
      className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}
      style={{ borderLeftColor: verdictBg, borderLeftWidth: 5 }}
    >
      <div className="d-flex align-items-center gap-2 mb-1">
        <span className="badge" style={{ background: verdictBg, color: '#1a1d20' }}>
          {result.verdict.toUpperCase()}
        </span>
        <span className="fw-semibold text-light">end-to-end</span>
      </div>
      <div className="text-secondary mb-2">{result.reason}</div>
      {engineEntries.length > 0 && (
        <div className="d-flex flex-column gap-1 mb-1">
          <div className="text-secondary">Engines</div>
          <div className="d-flex flex-wrap gap-1">
            {engineEntries.map(([name, ev]) => (
              <span
                key={name}
                className="badge"
                style={{ background: engineStatusColor(ev.status), color: '#1a1d20' }}
                title={ev.status}
              >
                {name}: {ev.status}
              </span>
            ))}
          </div>
        </div>
      )}
      {meshEntries.length > 0 && (
        <div className="d-flex flex-column gap-1">
          <div className="text-secondary">Mesh</div>
          <div className="d-flex flex-wrap gap-1">
            {meshEntries.map(([name, mv]) => (
              <span
                key={name}
                className="badge"
                style={{ background: engineStatusColor(mv.verdict), color: '#1a1d20' }}
                title={mv.reason}
              >
                {name}: {mv.verdict}
              </span>
            ))}
          </div>
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
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-2">
        <div className="fw-semibold text-light text-break">{first.policyName}</div>
        <div className="d-flex gap-1 flex-shrink-0">
          {isDeny
            ? <span className="badge bg-danger">deny</span>
            : <span className="badge bg-success">allow</span>}
        </div>
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Engine:</div>
        <EngineBadge engine={first.policySource} />
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Namespace:</div>
        <div>{first.namespace}</div>
      </div>
      <div className={`${s.fieldRow}`}>
        <div className={s.fieldLabel}>Directions:</div>
        <div className="d-flex flex-wrap gap-1">
          {[...directions].map((d) => <DirectionBadge key={d} direction={d} />)}
        </div>
      </div>
    </div>
  );
}

interface PairGroup {
  key: string;
  src: PolicyEdge['source'];
  dst: PolicyEdge['target'];
  edges: PolicyEdge[];
}

// Group selectedEdges by (source, target). Bundles produced by the Policies
// table span pairs; graph-click bundles always share a single pair.
export function groupEdgesByPair(edges: PolicyEdge[]): PairGroup[] {
  const map = new Map<string, PairGroup>();
  for (const e of edges) {
    const key = `${e.source}|${e.target}`;
    let g = map.get(key);
    if (!g) {
      g = { key, src: e.source, dst: e.target, edges: [] };
      map.set(key, g);
    }
    g.edges.push(e);
  }
  return [...map.values()];
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
      <div className={`text-secondary mb-1 ${s.smallText}`}>
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
              className={`text-start border border-secondary rounded p-2 bg-transparent text-light ${s.smallText}`}
              onClick={() => onSelect(pair.edges)}
            >
              <div className="d-flex align-items-center gap-2 flex-wrap">
                <span className="badge bg-info text-dark">SRC</span>
                <span className="fw-semibold text-break">{src?.label ?? pair.src}</span>
                <span className="text-secondary">/ {src?.namespace || '—'}</span>
                <span className="text-secondary">→</span>
                <span className="badge bg-warning text-dark">DST</span>
                <span className="fw-semibold text-break">{dst?.label ?? pair.dst}</span>
                <span className="text-secondary">/ {dst?.namespace || '—'}</span>
              </div>
              <div className="d-flex align-items-center gap-2 flex-wrap mt-1">
                {[...directions].map((d) => <DirectionBadge key={d} direction={d} />)}
                {portChips.length === 0
                  ? <span className="badge bg-secondary" title="Policy does not restrict ports — all TCP/UDP allowed">any port</span>
                  : portChips.map((p, i) => (
                      <span key={i} className="badge bg-info text-dark">{p}</span>
                    ))}
                {pair.edges.some((e) => (e.l7Matches?.length ?? 0) > 0) && (
                  <span className="badge bg-primary" title="L7 matchers present — click to drill in">L7</span>
                )}
              </div>
            </button>
          );
        })}
      </div>
    </div>
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

export function PolicyRow({ p }: { p: PolicyEdge }) {
  const isDeny = p.action === 1;
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-2">
        <div className="fw-semibold text-light text-break">{p.policyName}</div>
        <div className="d-flex gap-1 flex-shrink-0">
          {isDeny && <span className="badge bg-danger">deny</span>}
          <span className={`badge ${p.level === 'namespace' ? 'bg-warning text-dark' : 'bg-secondary'}`}>
            {p.level}
          </span>
        </div>
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Engine:</div>
        <EngineBadge engine={p.policySource} />
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Namespace:</div>
        <div>{p.namespace}</div>
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Direction:</div>
        <DirectionBadge direction={p.direction} />
      </div>
      <div>
        <div className="text-secondary">Ports</div>
        <div className="d-flex flex-column align-items-start gap-1 mt-1">
          {realPorts(p.ports)?.map((pt, i) => (
            <span key={i} className="badge bg-info text-dark">{formatPort(pt)}/{pt.protocol}</span>
          )) ?? (
            <span className="badge bg-secondary" title="Policy does not restrict ports — all TCP/UDP allowed">
              any port
            </span>
          )}
        </div>
      </div>
      {p.l7Matches && <L7Block blocks={p.l7Matches} />}
    </div>
  );
}

export function RuleRow({ rule }: { rule: NodeRule }) {
  const isDeny = rule.action === 1;
  const ports = realPorts(rule.ports);
  const dstLabel = rule.dstLabel || rule.dstId; // fall back to raw id for CIDR / unresolved
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className="fw-semibold text-light text-break">
          Workload: → {dstLabel}
          {rule.dstNamespace && <span className="text-secondary"> / {rule.dstNamespace}</span>}
        </div>
        {isDeny && <span className="badge bg-danger flex-shrink-0">deny</span>}
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Direction:</div>
        <DirectionBadge direction={rule.direction} />
      </div>
      <div>
        <div className="text-secondary">Ports</div>
        <div className="d-flex flex-column align-items-start gap-1 mt-1">
          {rule.allPorts ? (
            <span className="badge bg-secondary" title="Rule does not restrict ports — all TCP/UDP allowed">
              any port
            </span>
          ) : ports?.length ? (
            ports.map((pt, i) => (
              <span key={i} className="badge bg-info text-dark">{formatPort(pt)}/{pt.protocol}</span>
            ))
          ) : (
            <div className="text-secondary">none</div>
          )}
        </div>
      </div>
      {rule.l7Match && <L7Block blocks={[rule.l7Match]} />}
      {rule.contributor && (
        <div className="mt-2">
          <div className="text-secondary">From policy</div>
          <div className="text-light">
            {rule.contributor.name}
            <span className="text-secondary"> / {rule.contributor.namespace}</span>
          </div>
        </div>
      )}
    </div>
  );
}

export function PolicyRefRow({ policyRef }: { policyRef: PolicyRef }) {
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className="fw-semibold text-light text-break">{policyRef.name}</div>
        <div className="d-flex gap-1 flex-shrink-0">
          {policyRef.action === 'deny' && <span className="badge bg-danger">deny</span>}
          {policyRef.action === 'allow' && <span className="badge bg-success">allow</span>}
          {policyRef.direction && <DirectionBadge direction={policyRef.direction} />}
        </div>
      </div>
      <div className={s.fieldRow}>
        <div className={s.fieldLabel}>Namespace:</div>
        <div>{policyRef.namespace}</div>
      </div>
    </div>
  );
}

export function PolicyRefList({ items }: { items?: PolicyRef[] }) {
  if (!items || items.length === 0) {
    return <div className={`text-secondary ${s.smallText}`}>none</div>;
  }
  return <div>{items.map((p, i) => <PolicyRefRow key={i} policyRef={p} />)}</div>;
}
