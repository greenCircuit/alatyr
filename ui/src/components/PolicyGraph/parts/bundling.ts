// Edge bundling feature: collapse multiple policies between the same
// (src, dst, action) tuple onto a single visual arrow with a count + L7 hint.
// Bundles are keyed by action so allow and deny stay on separate arrows —
// never a mixed-semantics single line.

import type { PolicyEdge } from '../../../data/policies';
import { formatPort, realPorts, formatL7Summary } from '../../../data/policies';

export interface Bundle {
  id: string;
  source: string;
  target: string;
  policies: PolicyEdge[];
  hasNS: boolean;
  direction: 'egress' | 'ingress' | 'both';
  action: number;
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
    const key = `${e.source}→${e.target}@${action}`;
    if (!map.has(key)) map.set(key, []);
    map.get(key)!.push(e);
  }
  return Array.from(map.values()).map((policies) => {
    const action = policies[0].action ?? 0;
    return {
      id:        `bnd-${policies[0].source}-${policies[0].target}-${action}`,
      source:    policies[0].source,
      target:    policies[0].target,
      policies,
      hasNS:     policies.some((p) => p.level === 'namespace'),
      direction: aggregateDirection(policies),
      action,
    };
  });
}

// Compose edge label combining port list and L7 summary. Either side may be
// empty; when both are absent falls back to 'all ports'.
export function edgeLabel(edge: PolicyEdge): string {
  const portStr = realPorts(edge.ports)?.map(formatPort).join(', ');
  const l7Str = formatL7Summary(edge.l7Matches);
  if (portStr && l7Str) return `${portStr} · ${l7Str}`;
  if (l7Str) return l7Str;
  return portStr ?? 'all ports';
}

// Bundle label when multiple policies collapse onto one arrow. Surface a hint
// that some policies carry L7 so the user knows clicking will reveal more.
export function bundleLabel(edges: PolicyEdge[]): string {
  const hasL7 = edges.some((edge) => (edge.l7Matches?.length ?? 0) > 0);
  return hasL7 ? `${edges.length} policies · L7` : `${edges.length} policies`;
}
