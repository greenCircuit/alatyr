// Policy + rule rows: one card per PolicyEdge (graph edge), NodeRule
// (outbound/inbound), or PolicyRef (selecting policy). These are the
// mid-altitude pieces — they compose the badge primitives and carry the
// policy-specific layout shared by the workload, edge, and reachability views.

import type { PolicyEdge, NodeRule, PolicyRef, PolicySelector } from '../../../data/policies';
import { formatPort, realPorts } from '../../../data/policies';
import { DirectionBadge, EngineBadge, L7Block } from './badges';
import { ManifestButton } from './ManifestModal';
import s from '../DetailPanel.module.css';

export function PolicyRow({ p }: { p: PolicyEdge }) {
  const isDeny = p.action === 1;
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-2">
        <div className="fw-semibold text-light text-break">{p.policyName}</div>
        <div className="d-flex gap-1 flex-shrink-0 align-items-center">
          {isDeny && <span className="badge bg-danger">deny</span>}
          <span className={`badge ${p.level === 'namespace' ? 'bg-warning text-dark' : 'bg-secondary'}`}>
            {p.level}
          </span>
          <ManifestButton kind={p.policySource} namespace={p.namespace} name={p.policyName} />
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
        <div className='d-flex gap-1'>
          <div className="text-secondary">Ports:</div>
          <div className="d-flex flex-row align-items-start gap-1 mt-1">
            {realPorts(p.ports)?.map((pt, i) => (
              <span key={i} className="badge bg-info text-dark">{formatPort(pt)}/{pt.protocol}</span>
            )) ?? (
              <span className="badge bg-secondary" title="Policy does not restrict ports — all TCP/UDP allowed">
                any port
              </span>
            )}
          </div>

        </div>
      </div>
      {p.l7Matches && <L7Block blocks={p.l7Matches} />}
    </div>
  );
}

// Render the labels that selected one side of a rule. Wire convention:
// empty/absent selector = catch-all on that side (matches k8s — empty
// PodSelector selects every pod in the policy's namespace). The policyNs
// prop scopes the catch-all copy: a pure-empty selector is namespace-scoped
// in k8s NetworkPolicy land; nsSelector being populated (any value) means
// the policy reached cross-ns. Caveat: matchExpressions are not surfaced
// today, so an expression-only policy renders as catch-all — known gap.
function SelectorBlock({ label, sel, policyNs }: { label: string; sel?: PolicySelector; policyNs?: string }) {
  const pod = Object.entries(sel?.labelSelector ?? {});
  const ns  = Object.entries(sel?.nsSelector ?? {});
  const nsNames = sel?.namespaces ?? [];
  const isCatchAll = pod.length === 0 && ns.length === 0 && nsNames.length === 0;
  const catchAllCopy = policyNs
    ? `any workload in ${policyNs}`
    : 'any workload';
  return (
    <div className="mt-2">
      <div className="text-secondary">{label}</div>
      {isCatchAll ? (
        <div className="text-secondary mt-1">{catchAllCopy}</div>
      ) : (
        <div className="d-flex flex-column gap-1 mt-1">
          {pod.length > 0 && (
            <div className="d-flex flex-wrap align-items-center gap-1">
              <span className="text-secondary">pod:</span>
              {pod.map(([k, v]) => (
                <span key={`p-${k}`} className={`badge bg-secondary font-monospace ${s.badgeSm}`}>{k}={v}</span>
              ))}
            </div>
          )}
          {ns.length > 0 && (
            <div className="d-flex flex-wrap align-items-center gap-1">
              <span className="text-secondary">ns:</span>
              {ns.map(([k, v]) => (
                <span key={`n-${k}`} className={`badge bg-secondary font-monospace ${s.badgeSm}`}>{k}={v}</span>
              ))}
            </div>
          )}
          {nsNames.length > 0 && (
            <div className="d-flex flex-wrap align-items-center gap-1">
              <span className="text-secondary">from ns:</span>
              {nsNames.map((name) => (
                <span key={`ns-${name}`} className={`badge bg-secondary font-monospace ${s.badgeSm}`}>{name}</span>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// Flatten selector maps into "key: value" lines for YAML highlighting — both
// pod and ns labels, from whichever selectors the caller passes.
function selectorLines(...selectors: (PolicySelector | undefined)[]): string[] {
  const lines: string[] = [];
  for (const selector of selectors) {
    for (const [key, value] of Object.entries(selector?.labelSelector ?? {})) lines.push(`${key}: ${value}`);
    for (const [key, value] of Object.entries(selector?.nsSelector ?? {})) lines.push(`${key}: ${value}`);
    // YAML renders from.source.namespaces as list items; match the trimmed `- name` form.
    for (const name of selector?.namespaces ?? []) lines.push(`- ${name}`);
  }
  return lines;
}

export function RuleRow({ rule }: { rule: NodeRule }) {
  const isDeny = rule.action === 1;
  const ports = realPorts(rule.ports);
  const dstLabel = rule.dstLabel || rule.dstId; // fall back to raw id for CIDR / unresolved
  // Header phrasing tracks direction: ingress = traffic in from peer (peer is
  // the source), egress = traffic out to peer (peer is the destination).
  // The clicked node is always the *other* end — the field `dstId` carries
  // the peer regardless of direction, so the arrow has to flip with rule.direction.
  const peerVerb = rule.direction === 'ingress' ? 'From' : 'To';
  const peerArrow = rule.direction === 'ingress' ? '←' : '→';
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className="fw-semibold text-light text-break">
          {peerVerb}: {peerArrow} {dstLabel}
          {rule.dstNamespace && <span className="text-secondary"> / {rule.dstNamespace}</span>}
        </div>
        {isDeny && <span className="badge bg-danger flex-shrink-0">deny</span>}
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Direction:</div>
        <DirectionBadge direction={rule.direction} />
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Ports:</div>
        <div className="d-flex flex-wrap align-items-center gap-1">
          {ports?.length ? (
            ports.map((pt, i) => (
              <span key={i} className="badge bg-info text-dark">{formatPort(pt)}/{pt.protocol}</span>
            ))
          ) : (
            <span className="badge bg-secondary" title="Rule does not restrict ports — all TCP/UDP allowed">
              any port
            </span>
          )}
        </div>
      </div>
      {rule.l7Match && <L7Block blocks={[rule.l7Match]} />}
      {rule.contributor && (
        <div className="mt-3 pt-2 border-top border-secondary">
          <div className="d-flex justify-content-between align-items-start gap-2">
            <div className="fw-semibold">Policy: {rule.contributor.name}</div>
            <ManifestButton
              kind={rule.contributor.source}
              namespace={rule.contributor.namespace}
              name={rule.contributor.name}
              highlight={selectorLines(rule.srcSelector, rule.dstSelector)}
            />
          </div>
          {/* GetNodeData only returns rules where the clicked node is the
              SrcID (node-info.go), so every rule here is a flow OUT of this
              node: srcSelector always describes the clicked node, dstSelector
              the peer — independent of rule.direction (which is just the policy
              mechanism that allowed it). Fixed labels, no direction flip. */}
          <SelectorBlock
            label="Source (this node)"
            sel={rule.srcSelector}
            policyNs={rule.contributor.namespace}
          />
          <SelectorBlock
            label="Destination"
            sel={rule.dstSelector}
            policyNs={rule.contributor.namespace}
          />
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
        <div className="d-flex gap-1 flex-shrink-0 align-items-center">
          {policyRef.action === 'deny' && <span className="badge bg-danger">deny</span>}
          {policyRef.action === 'allow' && <span className="badge bg-success">allow</span>}
          {policyRef.direction && <DirectionBadge direction={policyRef.direction} />}
          <ManifestButton kind={policyRef.source} namespace={policyRef.namespace} name={policyRef.name} />
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
