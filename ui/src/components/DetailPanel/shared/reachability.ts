// Pure reachability derivations shared by ReachabilityView. Two jobs:
//   1. deriveBlockers — collapse the per-engine/per-direction verdict into the
//      Tier-1 answer: the (engine, direction) pairs that actually deny, ranked
//      by what the operator does about them (delete a deny > widen/add a rule).
//   2. classifyPolicy — join a selecting-policy chip in the SRC/DST column back
//      to what it did in this peer's verdict, so the column shows *which* policy
//      decided, not just the universe of policies touching the workload.
// Kept out of the .tsx so the view file exports components only (fast-refresh).

import type {
  ReachabilityResult,
  DirectionVerdict,
  DirectionReason,
  PolicyRef,
  PARef,
  NodeRule,
  Port,
} from '../../../data/policies';
import { realPorts, formatPort } from '../../../data/policies';

// A direction permits unless it explicitly denies or fails to match. permitted
// and no-opinion are the two non-blocking reasons — everything else stops
// src→dst on that side (the model ANDs egress+ingress across every engine).
function isBlocking(reason: DirectionReason): boolean {
  return reason !== 'permitted' && reason !== 'no-opinion';
}

// Rank blockers by remediation: an explicit deny rule is actively winning
// (delete it) and sorts first; a lock with no matching allow (add/widen) comes
// after. Mesh sits between — it's a real block but a separate axis.
const BLOCK_RANK: Record<string, number> = {
  'explicit-deny': 0,
  'carved-out': 2,
  'locked-no-match': 2,
  'default-deny': 3,
};
const MESH_RANK = 1;

// One blocking axis in the Tier-1 list. side points at the column to edit
// (egress = src, ingress = dst); paSource is set only for mesh rows.
export interface Blocker {
  engine:    string;
  direction: 'egress' | 'ingress' | 'mesh';
  side:      'src' | 'dst';
  reason:    DirectionReason | string;
  culprits:  PolicyRef[];
  denyRules: NodeRule[];
  paSource?: PARef;
  rank:      number;
}

// deriveBlockers walks every engine's egress+ingress plus mesh denies and
// returns only the axes that block, ranked. Empty result == nothing blocks on
// the enforced engines (caller should not reach here unless verdict is deny).
export function deriveBlockers(result: ReachabilityResult): Blocker[] {
  const blockers: Blocker[] = [];
  for (const [engine, verdict] of Object.entries(result.engines)) {
    const directions = [
      ['egress', verdict.egress, 'src'],
      ['ingress', verdict.ingress, 'dst'],
    ] as const;
    for (const [direction, dir, side] of directions) {
      if (!isBlocking(dir.reason)) continue;
      blockers.push({
        engine,
        direction,
        side,
        reason: dir.reason,
        culprits: dir.culprits ?? [],
        denyRules: dir.denyMatches ?? [],
        rank: BLOCK_RANK[dir.reason] ?? 9,
      });
    }
  }
  // Mesh mTLS is a separate axis; a deny there blocks the path too. Attribute
  // it to the PA that forced it rather than a NetworkPolicy/AuthorizationPolicy.
  for (const [name, meshVerdict] of Object.entries(result.mesh ?? {})) {
    if (meshVerdict.verdict !== 'deny') continue;
    blockers.push({
      engine:    `mesh/${name}`,
      direction: 'mesh',
      side:      'dst',
      reason:    meshVerdict.reason,
      culprits:  [],
      denyRules: [],
      paSource:  meshVerdict.effectiveSource?.name ? meshVerdict.effectiveSource : undefined,
      rank:      MESH_RANK,
    });
  }
  return blockers.sort((left, right) => left.rank - right.rank);
}

// Aggregated allow-port view of one direction (src egress or dst ingress).
//   allPorts  — some allow rule (or a non-enforcing engine) opens every port;
//               union with any subset is still "all", so the list is dropped.
//   ports     — deduped restricted ports when no rule opens everything.
//   blocked   — direction denies; port talk is moot on this side.
export interface SidePorts {
  allPorts: boolean;
  ports:    Port[];
  blocked:  boolean;
}

// aggregateAllowPorts unions the ports across every allow rule matching this
// peer. An engine with no opinion doesn't restrict ports, so it reads as all.
export function aggregateAllowPorts(dir: DirectionVerdict): SidePorts {
  if (dir.reason !== 'permitted' && dir.reason !== 'no-opinion') {
    return { allPorts: false, ports: [], blocked: true };
  }
  // no-opinion, or a permit with no attributed rule — nothing restricts ports.
  if (dir.reason === 'no-opinion' || !(dir.allowMatches ?? []).length) {
    return { allPorts: true, ports: [], blocked: false };
  }
  const seen = new Set<string>();
  const ports: Port[] = [];
  for (const rule of dir.allowMatches ?? []) {
    const restricted = realPorts(rule.ports);
    // allPorts flag or nothing but the port-0 sentinel → rule opens every port.
    if (rule.allPorts || !restricted) return { allPorts: true, ports: [], blocked: false };
    for (const port of restricted) {
      const key = `${formatPort(port)}/${port.protocol}`;
      if (seen.has(key)) continue;
      seen.add(key);
      ports.push(port);
    }
  }
  return { allPorts: false, ports, blocked: false };
}

// Chip state for a selecting policy, told from the peer's actual verdict:
//   permitter — authored an allow rule that opens this peer (green)
//   blocker   — authored a deny rule or deny-all lock hitting this peer (red)
//   near-miss — allows something, not this peer, or is the widen target (amber)
//   inert     — selects the workload, did nothing to this verdict (grey)
export type ChipState = 'permitter' | 'blocker' | 'near-miss' | 'inert';

function policyKey(ref: { source: string; name: string; namespace: string }): string {
  return `${ref.source}/${ref.namespace}/${ref.name}`;
}

function authored(rules: NodeRule[] | undefined, key: string): boolean {
  return (rules ?? []).some((rule) => rule.contributor && policyKey(rule.contributor) === key);
}

// classifyPolicy joins one column chip to the direction verdict for that side
// (egress for SRC, ingress for DST). Order matters: a real deny/lock outranks a
// culprit tag, and an active allow outranks a near-miss. A locked-no-match
// culprit lands as near-miss (the widen target), not a red blocker — it isn't
// denying, it just fails to reach this peer.
export function classifyPolicy(ref: PolicyRef, dir: DirectionVerdict): ChipState {
  const key = policyKey(ref);
  if (authored(dir.denyMatches, key) || authored(dir.denyAllMatches, key)) return 'blocker';
  if (authored(dir.allowMatches, key)) return 'permitter';
  const inCulprits = (dir.culprits ?? []).some((culprit) => policyKey(culprit) === key);
  if (authored(dir.allowOtherMatches, key) || inCulprits) return 'near-miss';
  return 'inert';
}
