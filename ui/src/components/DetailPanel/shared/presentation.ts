// Pure presentation tokens shared across DetailPanel views. Kept out of the
// component files so each .tsx exports components only (fast-refresh) — these
// are the color/verdict mappings the badges, mesh cards, and reachability
// columns all read from.

import { SEVERITY_COLOR } from '../../../data/policies';

// Direction tint + arrow. Match graph arrow colors so panel → canvas is a
// visual hand-off. Same hex values as style/edgeStyles.ts.
export const DIR_COLOR: Record<string, { tint: string; arrow: string }> = {
  egress:  { tint: '#4dabf7', arrow: '↑' },
  ingress: { tint: '#f783ac', arrow: '↓' },
  both:    { tint: '#a9e34b', arrow: '↕' },
};

// Color-codes mTLS verdict so the badge reads at a glance. Shared by the
// detail-panel mesh card and the reachability MeshSideCard.
export const MTLS_VERDICT_COLOR: Record<string, string> = {
  strict:     SEVERITY_COLOR.secure,
  permissive: SEVERITY_COLOR.caution,
  disable:    SEVERITY_COLOR.high,
  unset:      '#6c757d',
};

// verdictColor maps reachability verdicts to the project palette. Read by the
// status badges, the edge reachability banner, and the reachability columns.
export function verdictColor(verdict: string): string {
  if (verdict === 'allow') return SEVERITY_COLOR.secure;
  if (verdict === 'deny')  return SEVERITY_COLOR.high;
  return SEVERITY_COLOR.caution;
}
