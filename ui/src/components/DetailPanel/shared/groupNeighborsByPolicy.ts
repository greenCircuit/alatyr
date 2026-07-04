// Pure helper: collapse a direction's NeighborRefs into per-policy groups.
// The backend emits one NeighborRef per (peer, rule) — an ingress policy that
// admits four sources fans out to four refs sharing one policy/direction/ports.
// The panel repeats the invariant fields once per ref, bloating the side panel.
// Grouping folds the shared rule metadata into one header and lists the peers
// under it. Refs differing in policy/direction/action/ports/L7 stay in separate
// groups so nothing is misrepresented as identical. A CIDR/external peer still
// has a contributor policy (it groups normally) — what's blank there is the
// resolved Workload, handled by the raw-id fallback in the peer row. Only a rule
// with no attributed policy at all falls back to a standalone per-ref key.

import type { NeighborRef, Port, L7Match } from '../../../data/policies';
import { realPorts } from '../../../data/policies';

export interface NeighborGroup {
  key:   string;
  rule:  NeighborRef['Rule']; // representative rule — carries the shared fields
  peers: NeighborRef[];
}

// Stable signature of the fields that must match for two refs to share a header:
// policy identity, direction, action, effective ports, and L7. Backend sends
// Contributor as a value type (never null), so an unattributed rule arrives as a
// zero-value PolicyRef with an empty name — guard on the name, not the object,
// so those go standalone instead of merging into one empty-named card.
function groupKey(neighbor: NeighborRef, index: number): string {
  const rule = neighbor.Rule;
  const contributor = rule.contributor;
  if (!contributor?.name) return `_no_policy_${index}`;
  const policy = `${contributor.source}|${contributor.namespace}|${contributor.name}`;
  return `${policy}|${rule.direction}|${rule.action}|${portsKey(rule.ports)}|${l7Key(rule.l7Match)}`;
}

function portsKey(ports?: Port[]): string {
  const real = realPorts(ports);
  if (!real || real.length === 0) return 'any';
  return real
    .map((port) => `${port.protocol}/${port.port ?? ''}-${port.endPort ?? ''}`)
    .sort()
    .join(',');
}

function l7Key(l7Match?: L7Match): string {
  return l7Match ? JSON.stringify(l7Match) : '';
}

// A resolved peer, deduped across a section's groups. label/namespace fall back
// to the raw endpoint id when the workload is unresolved (external CIDR).
export interface PeerSummary {
  id:         string;
  label:      string;
  namespace?: string;
}

// Distinct peers across a set of groups — one entry per resolved workload no
// matter how many policies touch it. Feeds the section chip summary that answers
// "who can reach this" without the per-policy fan-out inflating the count.
export function distinctPeers(groups: NeighborGroup[], nodeIsSource: boolean): PeerSummary[] {
  const seen = new Map<string, PeerSummary>();
  for (const group of groups) {
    for (const peer of group.peers) {
      const rule = peer.Rule;
      const peerId = nodeIsSource ? rule.dstId : rule.srcId;
      const id = peer.Workload?.id || peerId;
      if (!id) continue; // blanket coverage rule (deny/allow-all) — no real peer
      if (seen.has(id)) continue;
      seen.set(id, { id, label: peer.Workload?.label || peerId, namespace: peer.Workload?.namespace });
    }
  }
  return [...seen.values()];
}

// Distinct-peer count straight from raw refs — the honest "how many can reach
// this" number for section headers, independent of the one-ref-per-rule fan-out.
export function distinctPeerCount(neighbors: NeighborRef[], nodeIsSource: boolean): number {
  return distinctPeers(groupNeighborsByPolicy(neighbors), nodeIsSource).length;
}

// splitByPeer separates blanket coverage rules (deny-all / allow-all /
// unenforced) from peer rules. The backend emits a blanket rule with the peer
// endpoint blank — grouping it as a neighbor would render an empty-peer card, so
// posture rules render as their own compact line instead. peerId blank ⟺ blanket
// (CIDR peers carry the CIDR string, ns peers the NSNode id — both non-empty).
export function splitByPeer(neighbors: NeighborRef[], nodeIsSource: boolean): {
  posture: NeighborRef[];
  peers:   NeighborRef[];
} {
  const posture: NeighborRef[] = [];
  const peers: NeighborRef[] = [];
  for (const neighbor of neighbors) {
    const rule = neighbor.Rule;
    const peerId = nodeIsSource ? rule.dstId : rule.srcId;
    if (peerId) peers.push(neighbor);
    else posture.push(neighbor);
  }
  return { posture, peers };
}

export function groupNeighborsByPolicy(neighbors: NeighborRef[]): NeighborGroup[] {
  const groups = new Map<string, NeighborGroup>();
  neighbors.forEach((neighbor, index) => {
    const key = groupKey(neighbor, index);
    let group = groups.get(key);
    if (!group) {
      group = { key, rule: neighbor.Rule, peers: [] };
      groups.set(key, group);
    }
    group.peers.push(neighbor);
  });
  return [...groups.values()];
}
