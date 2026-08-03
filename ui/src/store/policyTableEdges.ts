// Edge selection for the Policies audit table. Diverges from the graph's
// filteredEdges in two ways, both because the table lists policy OBJECTS while
// the graph draws arrows:
//
//   1. Cluster-scoped policies (Calico GlobalNetworkPolicy) carry no namespace
//      and govern every namespace, so a namespace selection must not hide them.
//   2. Blanket rules (a default-deny, a Calico allow-all) state a direction-wide
//      fact and carry no peer endpoint, so the graph has no arrow to draw and
//      filteredEdges' src+dst visibility check can never pass. A policy whose
//      only rule is blanket would vanish from the audit entirely.
//
// Everything else — engine, action, direction, search, mesh — still applies.

import type { PolicyEdge } from '../data/policies';
import {
  filteredNodes as deriveFilteredNodes,
  filteredEdges as deriveFilteredEdges,
  edgeMatchesPolicyFilters,
  type FilterState,
} from './filters';

const edgeIdentity = (edge: PolicyEdge) =>
  `${edge.policySource}|${edge.namespace}|${edge.policyName}|${edge.action ?? 0}|${edge.source}|${edge.target}|${edge.direction}`;

// A rule with one populated endpoint states posture, not a peer relationship.
const isBlanket = (edge: PolicyEdge) => !edge.source || !edge.target;

export function policyTableEdges(state: FilterState, allNamespaces: Set<string>): PolicyEdge[] {
  const scoped = deriveFilteredEdges(state);
  const wideOpen = { ...state, selectedNamespaces: allNamespaces };
  const clusterScoped = deriveFilteredEdges(wideOpen).filter((edge) => edge.namespace === '');

  // Namespace nodes are absent from filteredNodes on purpose (the graph treats
  // them as compound parents, not togglable types), but collapseUniformRules
  // rewrites a cluster-wide policy's endpoint to the ns node — so a blanket rule
  // from an all() selector lands on one. Admit them the way filteredEdges does.
  const visible = new Set(deriveFilteredNodes(wideOpen).map((node) => node.id));
  for (const namespace of allNamespaces) visible.add(`ns-${namespace}`);
  const blanket = state.allEdges.filter((edge) => {
    if (!isBlanket(edge)) return false;
    const endpoint = edge.source || edge.target;
    if (!endpoint) return false;
    // Namespace-scoped policies still answer to the namespace selection; only
    // the endpoint check is relaxed, not the scoping.
    if (edge.namespace !== '' && !state.selectedNamespaces.has(edge.namespace)) return false;
    return visible.has(endpoint) && edgeMatchesPolicyFilters(state, edge);
  });

  const seen = new Set(scoped.map(edgeIdentity));
  const out = [...scoped];
  for (const edge of [...clusterScoped, ...blanket]) {
    const identity = edgeIdentity(edge);
    if (seen.has(identity)) continue;
    seen.add(identity);
    out.push(edge);
  }
  return out;
}
