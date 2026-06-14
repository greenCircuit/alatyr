// Coverage feature: classify each namespace + workload as covered by
// workload-level policies, namespace-selector policies only, or no policies
// at all. Used to drive node border colors (the COV palette) — encodes
// policy coverage, not namespace identity.

import type { WorkloadNode, PolicyEdge } from '../../../data/policies';

export const COV = {
  workload:  '#20c997',  // green  – workload-level policies present
  namespace: '#ffc107',  // amber  – namespace-selector policies only
  none:      '#dc3545',  // red    – no policies (gap)
} as const;

export type CovType = keyof typeof COV;

export function computeCoverage(policyEdges: PolicyEdge[], allNodes: WorkloadNode[]): {
  ns: Record<string, CovType>;
  workload: Set<string>;
} {
  const nodeNS      = new Map(allNodes.map((n) => [n.id, n.namespace]));
  const nsWorkload  = new Set<string>();
  const nsNamespace = new Set<string>();
  const wlCovered   = new Set<string>();

  for (const e of policyEdges) {
    if (e.level === 'workload') {
      wlCovered.add(e.source);
      wlCovered.add(e.target);
      const sNS = nodeNS.get(e.source); if (sNS) nsWorkload.add(sNS);
      const tNS = nodeNS.get(e.target); if (tNS) nsWorkload.add(tNS);
    } else {
      nsNamespace.add(e.source.replace(/^ns-/, ''));
      nsNamespace.add(e.target.replace(/^ns-/, ''));
    }
  }

  // internet-* badges fire when traffic isn't locked — implicit, not proof of a policy.
  // Only treat statuses that require an actual policy as coverage signal.
  for (const n of allNodes) {
    const protective = n.statuses?.filter((s) => s !== 'internet-egress' && s !== 'internet-ingress' && s !== 'internet-full');
    if (protective && protective.length > 0) {
      wlCovered.add(n.id);
      if (n.namespace) nsWorkload.add(n.namespace);
    }
  }

  const ns: Record<string, CovType> = {};
  const derivedNS = [...new Set(allNodes.map((n) => n.namespace).filter(Boolean))];
  for (const n of derivedNS) {
    ns[n] = nsWorkload.has(n) ? 'workload' : nsNamespace.has(n) ? 'namespace' : 'none';
  }
  return { ns, workload: wlCovered };
}
