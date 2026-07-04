// Policy + rule rows: one card per PolicyEdge (graph edge), NodeRule
// (outbound/inbound), or PolicyRef (selecting policy). These are the
// mid-altitude pieces — they compose the badge primitives and carry the
// policy-specific layout shared by the workload, edge, and reachability views.

import { useState } from 'react';
import type { PolicyEdge, NodeRule, NeighborRef, PolicyRef, PolicySelector } from '../../../data/policies';
import { formatPort, realPorts } from '../../../data/policies';
import { CoverageBadge, DirectionBadge, EngineBadge, L7Block } from './badges';
import { ManifestButton } from './ManifestModal';
import { groupNeighborsByPolicy, splitByPeer, distinctPeers, type NeighborGroup as NeighborGroupData, type PeerSummary } from './groupNeighborsByPolicy';
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
              highlight={selectorLines(rule.srcSelector)}
              highlightPeer={selectorLines(rule.dstSelector)}
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

// One peer line inside a NeighborGroup: the resolved peer plus the peer-side
// selector. The shared policy/direction/ports fields live in the group header,
// so this stays lean — arrow + label + the selector that picked the peer.
function NeighborPeerRow({ neighbor, nodeIsSource }: { neighbor: NeighborRef; nodeIsSource: boolean }) {
  const rule = neighbor.Rule;
  const peerId = nodeIsSource ? rule.dstId : rule.srcId;
  const peerLabel = neighbor.Workload?.label || peerId;
  const peerNs = neighbor.Workload?.namespace;
  const peerArrow = nodeIsSource ? '→' : '←';
  // srcSelector describes the source side, dstSelector the destination —
  // independent of which end the clicked node is. The peer is the opposite end.
  const peerSelector = nodeIsSource ? rule.dstSelector : rule.srcSelector;
  const nodeSelector = nodeIsSource ? rule.srcSelector : rule.dstSelector;
  const contributor = rule.contributor?.name ? rule.contributor : undefined;
  return (
    <div className={`border-top border-secondary pt-2 mt-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2">
        <div className="fw-semibold text-light text-break">
          {peerArrow} {peerLabel}
          {peerNs && <span className="text-secondary"> / {peerNs}</span>}
        </div>
        {/* Per-peer YAML: highlights only this peer's selector (+ this node's),
            so a single-color highlight still correlates one rule to one peer. The
            card-header button highlights every peer at once for the overview. */}
        {contributor && (
          <ManifestButton
            kind={contributor.source}
            namespace={contributor.namespace}
            name={contributor.name}
            highlight={selectorLines(nodeSelector)}
            highlightPeer={selectorLines(peerSelector)}
          />
        )}
      </div>
      <SelectorBlock label="Selector" sel={peerSelector} policyNs={rule.contributor?.namespace} />
    </div>
  );
}

// Above this many peers a group collapses its peer list by default — the chip
// row already gives the overview, so card detail is opt-in for big fan-outs.
const PEER_COLLAPSE_THRESHOLD = 5;

// Sort key for peer rows: namespace then label (the operator's mental index),
// falling back to the raw endpoint id when the workload is unresolved.
function peerSortKey(neighbor: NeighborRef, nodeIsSource: boolean): string {
  const rule = neighbor.Rule;
  const peerId = nodeIsSource ? rule.dstId : rule.srcId;
  return `${neighbor.Workload?.namespace ?? ''}/${neighbor.Workload?.label || peerId}`;
}

// NeighborGroup renders one policy's fan-out: the shared rule fields (direction,
// ports, L7, policy, this-node selector) as a header, then every peer the policy
// touches as a lean sub-row. Collapses the backend's one-ref-per-peer emission
// so a policy admitting N sources reads as one card, not N repeated cards.
// nodeIsSource picks the peer end: Out (source) → peer is dstId, In → srcId.
// nodeNamespace is the clicked workload's ns — used to flag a policy that lives
// in a different namespace (cross-ns ingress), which is easy to miss otherwise.
export function NeighborGroup({ group, nodeIsSource, nodeNamespace }: { group: NeighborGroupData; nodeIsSource: boolean; nodeNamespace?: string }) {
  const rule = group.rule;
  const isDeny = rule.action === 1;
  const ports = realPorts(rule.ports);
  const peerVerb = nodeIsSource ? 'To' : 'From';
  const peerCount = group.peers.length;
  const peerSummary = `${peerVerb} ${peerCount} peer${peerCount > 1 ? 's' : ''}`;
  // Collapse big fan-outs by default; small lists stay open so a click isn't
  // needed for the common 1-3 peer case.
  const [peersOpen, setPeersOpen] = useState(peerCount <= PEER_COLLAPSE_THRESHOLD);
  const sortedPeers = [...group.peers].sort(
    (left, right) => peerSortKey(left, nodeIsSource).localeCompare(peerSortKey(right, nodeIsSource)),
  );
  // This-node side is the opposite of the peer end: Out → clicked node is source.
  const nodeSelector = nodeIsSource ? rule.srcSelector : rule.dstSelector;
  // Manifest highlight spans the node selector plus every peer selector so the
  // rendered YAML lights up all sources the grouped card represents.
  const peerSelectors = group.peers.map((peer) => (nodeIsSource ? peer.Rule.dstSelector : peer.Rule.srcSelector));
  // Deny cards carry a danger border so they read as blocks even mid-scroll,
  // past the "Denied by policy" section header.
  const borderClass = isDeny ? 'border-danger' : 'border-secondary';
  // Flag cross-ns policies: an ingress rule authored in another namespace is a
  // common blind spot when auditing what can reach a workload. Treat a zero-value
  // contributor (empty name — backend sends the struct, never null) as absent so
  // it renders like an unattributed rule instead of a blank-named policy header.
  const contributor = rule.contributor?.name ? rule.contributor : undefined;
  const crossNs = !!contributor && !!nodeNamespace && contributor.namespace !== nodeNamespace;
  return (
    <div className={`border ${borderClass} rounded p-2 mb-2 ${s.smallText}`}>
      {/* Policy name is the headline — it's what the operator edits to change
          the verdict. Peer count + manifest sit alongside as supporting detail. */}
      <div className="d-flex justify-content-between align-items-start gap-2 mb-2">
        <div className="d-flex flex-column">
          <div className="fw-bold text-light text-break d-flex align-items-center gap-2 flex-wrap">
            {contributor ? contributor.name : peerSummary}
            {crossNs && (
              <span
                className={`badge bg-warning text-dark ${s.badgeSm}`}
                title={`Policy lives in ${contributor!.namespace}, not this workload's namespace (${nodeNamespace})`}
              >
                ns: {contributor!.namespace}
              </span>
            )}
          </div>
          {contributor && <div className="text-secondary small">{peerSummary}</div>}
        </div>
        <div className="d-flex gap-1 flex-shrink-0 align-items-center">
          {/* Suppress the deny verb when the rule is unenforced — "No effect"
              is a state, and "deny No effect" contradicts itself. */}
          {isDeny && rule.coverage !== 'unenforced' && <span className="badge bg-danger">deny</span>}
          <CoverageBadge coverage={rule.coverage} />
          {contributor && (
            <ManifestButton
              kind={contributor.source}
              namespace={contributor.namespace}
              name={contributor.name}
              highlight={selectorLines(nodeSelector)}
              highlightPeer={selectorLines(...peerSelectors)}
            />
          )}
        </div>
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
      {contributor && (
        <div className="mt-2 pt-2 border-top border-secondary">
          <SelectorBlock
            label={nodeIsSource ? 'Source (this node)' : 'Destination (this node)'}
            sel={nodeSelector}
            policyNs={contributor.namespace}
          />
        </div>
      )}
      <div className="mt-2">
        <button
          type="button"
          className="btn btn-link p-0 text-decoration-none text-uppercase text-secondary small fw-semibold d-flex align-items-center gap-1"
          onClick={() => setPeersOpen((open) => !open)}
          aria-expanded={peersOpen}
        >
          <span>{peersOpen ? '▾' : '▸'}</span>
          {peerVerb} ({peerCount})
        </button>
        {peersOpen && sortedPeers.map((peer, i) => <NeighborPeerRow key={i} neighbor={peer} nodeIsSource={nodeIsSource} />)}
      </div>
    </div>
  );
}

// Distinct-peer summary chips — the "who" answer above the per-policy "how"
// cards. One chip per resolved workload, so a peer admitted by several policies
// counts once here even though it recurs in the policy cards below.
function PeerChips({ peers }: { peers: PeerSummary[] }) {
  if (peers.length === 0) return null;
  return (
    <div className="d-flex flex-wrap gap-1 mb-2">
      {peers.map((peer) => (
        <span
          key={peer.id}
          className={`badge bg-secondary ${s.badgeSm}`}
          title={peer.namespace ? `${peer.label} / ${peer.namespace}` : peer.label}
        >
          {peer.label}
        </span>
      ))}
    </div>
  );
}

// A titled run of neighbor cards — the deny/allow split within one direction.
// Title carries the distinct-peer count (not ref count), and the chip row lists
// those peers once before the per-policy cards.
function NeighborSection({ title, groups, nodeIsSource, nodeNamespace }: { title: string; groups: NeighborGroupData[]; nodeIsSource: boolean; nodeNamespace?: string }) {
  const peers = distinctPeers(groups, nodeIsSource);
  return (
    <div className="mb-2">
      <div className="text-uppercase text-secondary small fw-semibold mb-1">
        {title} · {peers.length} peer{peers.length > 1 ? 's' : ''}
      </div>
      <PeerChips peers={peers} />
      {groups.map((group) => <NeighborGroup key={group.key} group={group} nodeIsSource={nodeIsSource} nodeNamespace={nodeNamespace} />)}
    </div>
  );
}

// One blanket-posture line: a deny-all / allow-all / unenforced rule that has no
// real peer. Renders the direction + coverage state + contributing policy on a
// single row — the whole-direction verdict an operator scans first, before the
// per-peer cards. Danger border for deny-all so a hard block reads at a glance.
function PostureRow({ neighbor }: { neighbor: NeighborRef }) {
  const rule = neighbor.Rule;
  const contributor = rule.contributor?.name ? rule.contributor : undefined;
  const borderClass = rule.coverage === 'deny all' ? 'border-danger' : 'border-secondary';
  return (
    <div className={`border ${borderClass} rounded p-2 mb-2 d-flex justify-content-between align-items-center gap-2 ${s.smallText}`}>
      <div className="d-flex align-items-center gap-2 flex-wrap">
        <DirectionBadge direction={rule.direction} />
        <CoverageBadge coverage={rule.coverage} />
        {contributor && <span className="fw-semibold text-light text-break">{contributor.name}</span>}
      </div>
      {contributor && (
        <ManifestButton kind={contributor.source} namespace={contributor.namespace} name={contributor.name} />
      )}
    </div>
  );
}

// NeighborList groups a direction's refs by policy, then renders deny cards
// above allow cards — the panel-level entry point WorkloadView calls. Blanket
// coverage rules (deny-all / allow-all / unenforced) split out first as posture
// lines, since they carry no real peer. The section split mirrors eval order
// (deny wins), so an operator scanning a peer reads the blocks before the grants.
// Section labels appear only when denies exist; the common allow-only case (k8s
// NetworkPolicy) renders flat.
export function NeighborList({ neighbors, nodeIsSource, nodeNamespace }: { neighbors: NeighborRef[]; nodeIsSource: boolean; nodeNamespace?: string }) {
  const { posture, peers } = splitByPeer(neighbors, nodeIsSource);
  const groups = groupNeighborsByPolicy(peers);
  const denyGroups  = groups.filter((group) => group.rule.action === 1);
  const allowGroups = groups.filter((group) => group.rule.action !== 1);
  const postureBlock = posture.length > 0 && (
    <div className="mb-2">{posture.map((ref, i) => <PostureRow key={i} neighbor={ref} />)}</div>
  );
  if (denyGroups.length === 0) {
    // Allow-only (k8s NetworkPolicy): no section labels, but still summarize the
    // distinct peers on top before the per-policy cards.
    return (
      <>
        {postureBlock}
        <PeerChips peers={distinctPeers(allowGroups, nodeIsSource)} />
        {allowGroups.map((group) => <NeighborGroup key={group.key} group={group} nodeIsSource={nodeIsSource} nodeNamespace={nodeNamespace} />)}
      </>
    );
  }
  return (
    <>
      {postureBlock}
      <NeighborSection title="⛔ Denied by policy" groups={denyGroups} nodeIsSource={nodeIsSource} nodeNamespace={nodeNamespace} />
      {allowGroups.length > 0 && (
        <NeighborSection title="✓ Allowed by policy" groups={allowGroups} nodeIsSource={nodeIsSource} nodeNamespace={nodeNamespace} />
      )}
    </>
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
