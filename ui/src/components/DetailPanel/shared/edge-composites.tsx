// Edge/connection composites: the higher-altitude pieces the edge views build
// from. Endpoint cards, the shared policy header + affected-pairs list for
// Policies-table bundles, the cross-engine reachability banner, and the
// pair-grouping helper they all key off.

import type { PolicyEdge, WorkloadNode, ReachabilityResult } from '../../../data/policies';
import { formatPort, realPorts, SEVERITY_COLOR } from '../../../data/policies';
import { DirectionBadge, EngineBadge } from './badges';
import { ManifestButton } from './ManifestModal';
import { verdictColor } from './presentation';
import type { PairGroup } from './groupEdgesByPair';
import s from '../DetailPanel.module.css';

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
        <div className="d-flex flex-row gap-1 mb-1">
            <div className="text-secondary">Engines: </div>
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
        <div className="d-flex flex-row mt-2 gap-1">
          <div className="text-secondary">Mesh: </div>
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
        <div className="d-flex gap-1 flex-shrink-0 align-items-center">
          {isDeny
            ? <span className="badge bg-danger">deny</span>
            : <span className="badge bg-success">allow</span>}
          <ManifestButton kind={first.policySource} namespace={first.namespace} name={first.policyName} />
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
              <div className="d-flex gap-2 flex-wrap mt-1">
                {[...directions].map((d) => <DirectionBadge key={d} direction={d} />)}
                {pair.edges.some((e) => (e.l7Matches?.length ?? 0) > 0) && (
                  <span className="badge bg-primary" title="L7 matchers present — click to drill in">L7</span>
                )}
              </div>
              <div className="d-flex gap-2 flex-wrap mt-1">
                {portChips.length === 0
                  ? <span className="badge bg-secondary" title="Policy does not restrict ports — all TCP/UDP allowed">any port</span>
                  : portChips.map((p, i) => (
                      <span key={i} className="badge bg-info text-dark">{p}</span>
                    ))}
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}
