// Pure presentation tokens shared across DetailPanel views. Kept out of the
// component files so each .tsx exports components only (fast-refresh) — these
// are the color/verdict mappings the badges, mesh cards, and reachability
// columns all read from.

import { SEVERITY_COLOR } from '../../../data/policies';
import type { DirectionReason } from '../../../data/policies';
import type { ChipState } from './reachability';

// Per-direction reason → chip label + semantic tone. Callers map tone to
// their local module class (e.g. `.reasonAllow` / `.reasonDeny`). Blocked
// reasons go red/amber; permitted / no-opinion stay quiet. Shared by the
// Tier-1 blocker list and the Tier-2 per-direction breakdown.
export type ReasonTone = 'allow' | 'deny' | 'warn' | 'inert';
export const REASON_META: Record<DirectionReason, { label: string; tone: ReasonTone }> = {
  'permitted':       { label: 'permitted',                   tone: 'allow' },
  'no-opinion':      { label: 'no policy',                   tone: 'inert' },
  'explicit-deny':   { label: 'explicit deny',               tone: 'deny'  },
  'default-deny':    { label: 'default-deny',                tone: 'deny'  },
  'locked-no-match': { label: 'blocked · no matching rule',  tone: 'warn'  },
};

// Selecting-policy chip accent per join state. Green opens the path, red blocks
// it, amber is the widen target, grey did nothing to this verdict.
export const CHIP_STATE: Record<ChipState, { accent: string; label: string; muted: boolean }> = {
  permitter: { accent: SEVERITY_COLOR.secure,  label: 'permits',   muted: false },
  blocker:   { accent: SEVERITY_COLOR.high,     label: 'blocks',    muted: false },
  'near-miss': { accent: SEVERITY_COLOR.caution, label: 'near-miss', muted: false },
  inert:     { accent: '#6c757d',               label: '',          muted: true  },
};

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
