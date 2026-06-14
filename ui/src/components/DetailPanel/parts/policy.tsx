// Policy + rule rendering: rows for PolicyEdge (graph edges), NodeRule
// (outbound/inbound), PolicyRef (selecting policies), and the L7 matcher
// block they share. DirectionBadge lives here too because policy/rule rows
// are its primary consumers — reachability.tsx imports it for DirectionBlock.

import type { PolicyEdge, NodeRule, PolicyRef, L7Match } from '../../../data/policies';
import { formatPort, realPorts } from '../../../data/policies';
import s from '../DetailPanel.module.css';

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
        <div>{p.policySource}</div>
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
          )) ?? <div>all ports</div>}
        </div>
      </div>
      {p.l7Matches && <L7Block blocks={p.l7Matches} />}
    </div>
  );
}

export function RuleRow({ rule }: { rule: NodeRule }) {
  const isDeny = rule.action === 1;
  const ports = realPorts([rule.port]);
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
          {ports?.map((pt, i) => (
            <span key={i} className="badge bg-info text-dark">{formatPort(pt)}/{pt.protocol}</span>
          )) ?? <div>all ports</div>}
        </div>
      </div>
      {rule.l7Match && <L7Block blocks={[rule.l7Match]} />}
      {rule.contributors && rule.contributors.length > 0 && (
        <div className="mt-2">
          <div className="text-secondary">From policy</div>
          {rule.contributors.map((policyRef, i) => (
            <div key={i} className="text-light">
              {policyRef.name}<span className="text-secondary"> / {policyRef.namespace}</span>
            </div>
          ))}
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
