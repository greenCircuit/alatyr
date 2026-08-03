// Edge bundling feature: collapse multiple policies between the same
// (src, dst, action) tuple onto a single visual arrow with a count + L7 hint.
// Bundles are keyed by action so allow and deny stay on separate arrows —
// never a mixed-semantics single line.

import type { PolicyEdge, Coverage } from '../../../data/policies';
import { formatPort, realPorts, formatL7Summary } from '../../../data/policies';

export interface Bundle {
  id: string;
  source: string;
  target: string;
  policies: PolicyEdge[];
  hasNS: boolean;
  direction: 'egress' | 'ingress' | 'both';
  action: number;
  coverage?: Coverage;   // 'except' when every policy in the bundle is a carve-out; blank otherwise
}

export function aggregateDirection(policies: PolicyEdge[]): 'egress' | 'ingress' | 'both' {
  const dirs = new Set(policies.map((p) => p.direction));
  if (dirs.has('both') || (dirs.has('egress') && dirs.has('ingress'))) return 'both';
  return dirs.has('egress') ? 'egress' : 'ingress';
}

export function bundleEdges(policyEdges: PolicyEdge[]): Bundle[] {
  const map = new Map<string, PolicyEdge[]>();
  for (const e of policyEdges) {
    const action = e.action ?? 0;
    // Except carve-outs share (src, dst, action=1) with Istio DENY on rare
    // overlaps; splitting on coverage keeps the amber "except" arrow from
    // collapsing into the red "deny" arrow.
    const covKey = e.coverage === 'except' ? 'except' : '';
    const key = `${e.source}→${e.target}@${action}@${covKey}`;
    if (!map.has(key)) map.set(key, []);
    map.get(key)!.push(e);
  }
  return Array.from(map.values()).map((policies) => {
    const action = policies[0].action ?? 0;
    const coverage = policies.every((p) => p.coverage === 'except') ? 'except' : undefined;
    return {
      id:        `bnd-${policies[0].source}-${policies[0].target}-${action}${coverage ? '-except' : ''}`,
      source:    policies[0].source,
      target:    policies[0].target,
      policies,
      hasNS:     policies.some((p) => p.level === 'namespace'),
      direction: aggregateDirection(policies),
      action,
      coverage,
    };
  });
}

// Distinct engine names for the edges on one arrow, in first-seen order. A
// bundle can mix engines (k8s + istio both allowing the same pair), so the
// engine-icon overlay may render more than one logo per arrow.
export function edgeEngines(edges: PolicyEdge[]): string[] {
  const seen = new Set<string>();
  const engines: string[] = [];
  for (const edge of edges) {
    if (seen.has(edge.policySource)) continue;
    seen.add(edge.policySource);
    engines.push(edge.policySource);
  }
  return engines;
}

// Compose edge label combining port list and L7 summary. Either side may be
// empty; when both are absent falls back to 'all ports'.
export function edgeLabel(edge: PolicyEdge): string {
  const portStr = realPorts(edge.ports)?.map(formatPort).join(', ');
  const l7Str = formatL7Summary(edge.l7Matches);
  // Aggregated arrows must say so — one line standing for N workloads reads as a
  // namespace-scoped policy otherwise.
  const scope = edge.aggregatedFrom ? `all ${edge.aggregatedFrom} workloads` : null;
  const parts = [portStr, l7Str, scope].filter(Boolean);
  if (parts.length === 0) return 'all ports';
  return parts.join(' · ');
}

// Bundle label when multiple policies collapse onto one arrow. Surface a hint
// that some policies carry L7 so the user knows clicking will reveal more.
export function bundleLabel(edges: PolicyEdge[]): string {
  const hasL7 = edges.some((edge) => (edge.l7Matches?.length ?? 0) > 0);
  return hasL7 ? `${edges.length} policies · L7` : `${edges.length} policies`;
}
