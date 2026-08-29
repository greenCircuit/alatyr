// Policy + rule rows: one card per PolicyEdge (graph edge), NodeRule
// (outbound/inbound), or PolicyRef (selecting policy). These are the
// mid-altitude pieces — they compose the badge primitives and carry the
// policy-specific layout shared by the workload, edge, and reachability views.

import type { PolicyEdge, NodeRule, NeighborRef, PolicyRef, PolicySelector } from '../../../data/policies';
import { formatPort, realPorts } from '../../../data/policies';
import { CoverageBadge, DirectionArrow, EngineBadge, L7Block, ActionIcon } from './badges';
import { ManifestButton } from './ManifestModal';
import { groupNeighborsByPolicy, groupNodeRulesByPolicy, splitByPeer, distinctPeers, type NeighborGroup as NeighborGroupData, type NodeRuleGroup, type PeerSummary } from './groupNeighborsByPolicy';
import s from '../DetailPanel.module.css';

export function PolicyRow({ p }: { p: PolicyEdge }) {
  const isDeny = p.action === 1;
  const ports = realPorts(p.ports);
  // Compact header shape per mockup: ActionIcon + name (section) + EngineChip +
  // ns + level → YAML on the right. Direction + ports on the second line.
  // Drops the tabular 4-row Engine/Namespace/Direction/Ports grid — visually
  // dense but scan-slow, since each field carried its own label.
  return (
    <div className={`${s.card} ${isDeny ? s.cardDeny : ''} ${s.smallText}`}>
      <div className={s.policyMetaRow}>
        <ActionIcon action={isDeny ? 'deny' : 'allow'} />
        <span className={`${s.section} text-break ${s.flexFill}`}>{p.policyName}</span>
        <EngineBadge engine={p.policySource} />
        <span className={s.dim}>{p.namespace}</span>
        {p.level === 'namespace' && (
          <span className={s.crossNsChip} title="Namespace-level edge (one endpoint is a namespace, not a workload)">
            {p.level}
          </span>
        )}
        <ManifestButton kind={p.policySource} namespace={p.namespace} name={p.policyName} />
      </div>
      <div className={s.policyMetaRow}>
        <DirectionArrow direction={p.direction} />
        <span className={s.dim}>ports:</span>
        {ports?.length
          ? ports.map((pt, i) => <span key={i} className={s.portChip}>{formatPort(pt)}/{pt.protocol}</span>)
          : <span className={`${s.catchAll}`} title="Policy does not restrict ports — all TCP/UDP allowed">any port</span>}
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
    <div className="d-flex flex-column gap-1">
      <div className={s.eyebrow}>{label}</div>
      {isCatchAll ? (
        <div className={`${s.body} ${s.catchAll}`}>{catchAllCopy}</div>
      ) : (
        <div className="d-flex flex-column gap-1">
          {pod.length > 0 && (
            <div className="d-flex flex-wrap align-items-center gap-1">
              <span className={s.dim}>pod:</span>
              {pod.map(([k, v]) => (
                <span key={`p-${k}`} className={s.labelChip}>
                  <span className={s.labelKey}>{k}</span>
                  <span className={s.labelEq}>=</span>
                  <span className={s.labelValue}>{v}</span>
                </span>
              ))}
            </div>
          )}
          {ns.length > 0 && (
            <div className="d-flex flex-wrap align-items-center gap-1">
              <span className={s.dim}>ns:</span>
              {ns.map(([k, v]) => (
                <span key={`n-${k}`} className={s.labelChip}>
                  <span className={s.labelKey}>{k}</span>
                  <span className={s.labelEq}>=</span>
                  <span className={s.labelValue}>{v}</span>
                </span>
              ))}
            </div>
          )}
          {nsNames.length > 0 && (
            <div className="d-flex flex-wrap align-items-center gap-1">
              <span className={s.dim}>from ns:</span>
              {nsNames.map((name) => (
                <span key={`ns-${name}`} className={s.labelChip}>
                  <span className={s.labelValue}>{name}</span>
                </span>
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

// Peer end of a reachability NodeRule, picked by direction: ingress = traffic
// in from peer (peer is the source, identified by srcId), egress = traffic out
// to peer (dstId). Node-info payloads predate src resolution and always carry
// the peer in the dst fields — hence the dst fallbacks. Label falls back to the
// raw id for CIDR / unresolved endpoints.
function rulePeer(rule: NodeRule) {
  const isIngress = rule.direction === 'ingress';
  const peerId = (isIngress ? rule.srcId : rule.dstId) || rule.dstId;
  return {
    // Blank peer id = the rule doesn't name a peer (catch-all side) — say so
    // instead of rendering a bare arrow; the selector block scopes it below.
    label:    (isIngress ? rule.srcLabel : rule.dstLabel) || rule.dstLabel || peerId || 'any workload',
    ns:       (isIngress ? rule.srcNamespace : rule.dstNamespace) || rule.dstNamespace,
    kind:     (isIngress ? rule.srcKind : rule.dstKind) || rule.dstKind,
    selector: isIngress ? rule.srcSelector : rule.dstSelector,
    verb:     isIngress ? 'From' : 'To',
    arrow:    isIngress ? '←' : '→',
  };
}

// Peer headline fragment: label, ns suffix, kind badge. A namespace peer's own
// ns is its label — the suffix would just repeat it, so it's suppressed there.
function PeerHeadline({ peer }: { peer: ReturnType<typeof rulePeer> }) {
  const isNamespace = peer.kind === 'namespace';
  return (
    <>
      {peer.label}
      {peer.ns && !isNamespace && <span className={s.dim}> / {peer.ns}</span>}
      {peer.kind && (
        <span className={`${s.typeChip} ms-2`}>
          {peer.kind}
        </span>
      )}
    </>
  );
}

function PortBadges({ ports }: { ports?: NodeRule['ports'] }) {
  const real = realPorts(ports);
  if (!real?.length) {
    return (
      <span className={s.portChipAny} title="Rule does not restrict ports — all TCP/UDP allowed">
        any port
      </span>
    );
  }
  return (
    <>
      {real.map((port, index) => (
        <span key={index} className={s.portChip}>{formatPort(port)}/{port.protocol}</span>
      ))}
    </>
  );
}

export function RuleRow({ rule }: { rule: NodeRule }) {
  const isDeny = rule.action === 1;
  const ports = realPorts(rule.ports);
  const peer = rulePeer(rule);
  const { verb: peerVerb, arrow: peerArrow } = peer;
  return (
    <div className={`${s.card} ${isDeny ? s.cardDeny : ''} ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className={`${s.policyMetaRow} ${s.flexFill}`}>
          <ActionIcon action={isDeny ? 'deny' : 'allow'} />
          <span className={`${s.section} text-break`}>
            {peerVerb}: {peerArrow} <PeerHeadline peer={peer} />
          </span>
        </div>
      </div>
      <div className={s.policyMetaRow}>
        <DirectionArrow direction={rule.direction} />
        <span className={s.dim}>ports:</span>
        {ports?.length
          ? ports.map((pt, i) => <span key={i} className={s.portChip}>{formatPort(pt)}/{pt.protocol}</span>)
          : <span className={`${s.catchAll}`} title="Rule does not restrict ports — all TCP/UDP allowed">any port</span>}
      </div>
      {rule.l7Match && <L7Block blocks={[rule.l7Match]} />}
      {rule.contributor && (
        <div className={s.subRow}>
          <div className={s.policyMetaRow}>
            <span className={s.dim}>policy:</span>
            <span className={`${s.section} text-break ${s.flexFill}`}>{rule.contributor.name}</span>
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

// One policy's rules in the reachability breakdown: policy headline once, then
// each peer as a lean sub-row carrying its own ports/L7/selector (they vary per
// peer within one policy). Mirrors the node panel's NeighborGroup so both
// panels teach the same card shape.
function RuleGroupCard({ group }: { group: NodeRuleGroup }) {
  const contributor = group.contributor!;
  const isDeny = group.rules[0].action === 1;
  const peers = group.rules.map(rulePeer);
  // Manifest highlight spans every rule: node-side selectors in the primary
  // color, peer-side in the secondary — same split the node panel uses.
  const nodeSelectors = group.rules.map((rule) => (rule.direction === 'ingress' ? rule.dstSelector : rule.srcSelector));
  return (
    <div className={`${s.card} ${isDeny ? s.cardDeny : ''} ${s.smallText}`}>
      <div className={s.policyMetaRow}>
        <ActionIcon action={isDeny ? 'deny' : 'allow'} />
        <span className={`${s.section} text-break ${s.flexFill}`}>{contributor.name}</span>
        <EngineBadge engine={contributor.source} />
        <span className={s.dim}>{contributor.namespace}</span>
        <ManifestButton
          kind={contributor.source}
          namespace={contributor.namespace}
          name={contributor.name}
          highlight={selectorLines(...nodeSelectors)}
          highlightPeer={selectorLines(...peers.map((peer) => peer.selector))}
        />
      </div>
      <div className={s.policyMetaRow}>
        <DirectionArrow direction={group.rules[0].direction} />
      </div>
      {group.rules.map((rule, index) => {
        const peer = peers[index];
        return (
          <div key={index} className={s.subRow}>
            <div className={`${s.section} text-break`}>
              {peer.arrow} <PeerHeadline peer={peer} />
            </div>
            <div className="d-flex flex-wrap align-items-center gap-1 mt-1">
              <PortBadges ports={rule.ports} />
            </div>
            {rule.l7Match && <L7Block blocks={[rule.l7Match]} />}
            <SelectorBlock label="Selector" sel={peer.selector} policyNs={contributor.namespace} />
          </div>
        );
      })}
    </div>
  );
}

// Entry point for the reachability panel's rule lists: one card per policy with
// its peers underneath; rules with no attributed policy stay standalone RuleRows.
export function RuleGroupList({ rules }: { rules: NodeRule[] }) {
  return (
    <>
      {groupNodeRulesByPolicy(rules).map((group) =>
        group.contributor
          ? <RuleGroupCard key={group.key} group={group} />
          : group.rules.map((rule, index) => <RuleRow key={`${group.key}-${index}`} rule={rule} />),
      )}
    </>
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
    <div className={`${s.subRow} ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2">
        <div className={`${s.section} text-break`}>
          {peerArrow} {peerLabel}
          {peerNs && <span className={s.dim}> / {peerNs}</span>}
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
  const defaultOpen = peerCount <= PEER_COLLAPSE_THRESHOLD;
  const sortedPeers = [...group.peers].sort(
    (left, right) => peerSortKey(left, nodeIsSource).localeCompare(peerSortKey(right, nodeIsSource)),
  );
  // This-node side is the opposite of the peer end: Out → clicked node is source.
  const nodeSelector = nodeIsSource ? rule.srcSelector : rule.dstSelector;
  // Manifest highlight spans the node selector plus every peer selector so the
  // rendered YAML lights up all sources the grouped card represents.
  const peerSelectors = group.peers.map((peer) => (nodeIsSource ? peer.Rule.dstSelector : peer.Rule.srcSelector));
  // State-colored left stripe reads as block/allow at scroll speed. Full-bg
  // tint on every card was the wall-of-red anti-pattern (STYLEGUIDE §3).
  // Allow rows stay neutral — wall-of-green on lists = signal fatigue.
  // Only deny gets a stripe (STYLEGUIDE §3 central-neutrality).
  const stripeClass = isDeny ? s.cardDeny : '';
  // Flag cross-ns policies: an ingress rule authored in another namespace is a
  // common blind spot when auditing what can reach a workload. Treat a zero-value
  // contributor (empty name — backend sends the struct, never null) as absent so
  // it renders like an unattributed rule instead of a blank-named policy header.
  const contributor = rule.contributor?.name ? rule.contributor : undefined;
  const crossNs = !!contributor && !!nodeNamespace && contributor.namespace !== nodeNamespace;
  return (
    <div className={`${s.card} ${stripeClass} ${s.smallText}`}>
      {/* PolicyMetaRow header per mockup: ActionIcon + name + engine + ns +
          optional cross-ns badge + coverage + YAML. Compact single line. */}
      <div className={s.policyMetaRow}>
        <ActionIcon action={isDeny && rule.coverage !== 'unenforced' ? 'deny' : 'allow'} />
        <div className={`d-flex align-items-baseline flex-wrap gap-2 ${s.flexFill}`}>
          <span className={`${s.section} text-break`}>
            {contributor ? contributor.name : peerSummary}
          </span>
          {contributor && (
            <span className={`${s.body} ${s.dim} ${s.mono}`}>{contributor.namespace}</span>
          )}
        </div>
        {crossNs && (
          <span
            className={s.crossNsChip}
            title={`Policy lives in ${contributor!.namespace}, not this workload's namespace (${nodeNamespace})`}
          >
            cross-ns
          </span>
        )}
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
      {contributor && <div className={`${s.dim} ${s.smallText}`}>{peerSummary}</div>}
      <div className={s.policyMetaRow}>
        <DirectionArrow direction={rule.direction} />
        <span className={s.dim}>ports:</span>
        {ports?.length
          ? ports.map((pt, i) => <span key={i} className={s.portChip}>{formatPort(pt)}/{pt.protocol}</span>)
          : <span className={`${s.catchAll}`} title="Rule does not restrict ports — all TCP/UDP allowed">any port</span>}
      </div>
      {rule.l7Match && <L7Block blocks={[rule.l7Match]} />}
      {contributor && (
        <div className={s.subRow}>
          <SelectorBlock
            label={nodeIsSource ? 'Source (this node)' : 'Destination (this node)'}
            sel={nodeSelector}
            policyNs={contributor.namespace}
          />
        </div>
      )}
      <details open={defaultOpen}>
        <summary className={`${s.disclosureSummary} ${s.eyebrow}`}>
          {peerVerb} ({peerCount})
        </summary>
        {sortedPeers.map((peer, i) => <NeighborPeerRow key={i} neighbor={peer} nodeIsSource={nodeIsSource} />)}
      </details>
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
          className={s.peerChip}
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
function NeighborSection({ title, groups, nodeIsSource, nodeNamespace, tone }: { title: string; groups: NeighborGroupData[]; nodeIsSource: boolean; nodeNamespace?: string; tone?: 'deny' | 'allow' }) {
  const peers = distinctPeers(groups, nodeIsSource);
  const toneClass = tone === 'deny' ? s.eyebrowDeny : tone === 'allow' ? s.eyebrowAllow : '';
  return (
    <div className="mb-2">
      <div className={`${s.eyebrow} ${toneClass} mb-1`}>
        {title} ({peers.length} peer{peers.length > 1 ? 's' : ''})
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
  // Deny-all posture gets full border (not just stripe) — hard-block scroll-speed
  // read, STYLEGUIDE §3 exception. Other postures use standard postureRow.
  const postureClass = rule.coverage === 'deny all' ? `${s.postureRow} ${s.postureDenyAll}` : s.postureRow;
  return (
    <div className={`${postureClass} d-flex justify-content-between align-items-center gap-2 ${s.smallText}`}>
      <div className="d-flex align-items-center gap-2 flex-wrap">
        <DirectionArrow direction={rule.direction} />
        <CoverageBadge coverage={rule.coverage} />
        {contributor && <span className={`${s.section} text-break`}>{contributor.name}</span>}
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
      <NeighborSection title="⛔ Denied by policy" groups={denyGroups} nodeIsSource={nodeIsSource} nodeNamespace={nodeNamespace} tone="deny" />
      {allowGroups.length > 0 && (
        <NeighborSection title="✓ Allowed by policy" groups={allowGroups} nodeIsSource={nodeIsSource} nodeNamespace={nodeNamespace} tone="allow" />
      )}
    </>
  );
}

export function PolicyRefRow({ policyRef }: { policyRef: PolicyRef }) {
  const stripeClass = policyRef.action === 'deny' ? s.cardDeny
                    : '';
  return (
    <div className={`${s.card} ${stripeClass} ${s.smallText}`}>
      <div className={s.policyMetaRow}>
        {policyRef.action && <ActionIcon action={policyRef.action} />}
        <span className={`${s.section} text-break ${s.flexFill}`}>{policyRef.name}</span>
        <EngineBadge engine={policyRef.source} />
        <span className={s.dim}>{policyRef.namespace}</span>
        <CalicoPrecedenceChip tier={policyRef.tier} order={policyRef.order} />
        {policyRef.direction && <DirectionArrow direction={policyRef.direction} />}
        <ManifestButton kind={policyRef.source} namespace={policyRef.namespace} name={policyRef.name} />
      </div>
    </div>
  );
}

// Calico first-match precedence chip. Tier + order together determine which
// policy wins when two select the same workload. Hidden for engines that don't
// emit these fields (k8s, istio) so the row stays uncluttered.
function CalicoPrecedenceChip({ tier, order }: { tier?: string; order?: number | null }) {
  if (!tier && order == null) return null;
  const orderText = order == null ? 'no order' : `order ${order}`;
  const tierText = tier || 'default';
  return (
    <span
      className={s.countChip}
      title={`Calico precedence: tier=${tierText}, ${orderText}. Lower order wins within a tier; unset order sorts last.`}
    >
      {tierText} ({orderText})
    </span>
  );
}

export function PolicyRefList({ items }: { items?: PolicyRef[] }) {
  if (!items || items.length === 0) {
    return <div className={`${s.dim} ${s.smallText}`}>none</div>;
  }
  return <div>{items.map((p, i) => <PolicyRefRow key={i} policyRef={p} />)}</div>;
}
