// Single mtls-verdict rendering primitive. Consumers pass a verdict + variant;
// this owns color, shape, radius, tone. Replaces the four dialects that used
// to render the same verdict as a raw verdictChip (panel), a swatch dot
// (cluster summary), a legend pill (MeshRollup), and the graph meshMark.
//
// Variants:
//   pill — labeled solid pill, uppercase verdict text (panel + rollup use).
//   dot  — 8x8 rounded swatch, no label (legend rows, summary chips).
//   text — raw color-only string (rare — callers styling their own container).

import type { MtlsScope } from '../../../data/policies';
import { MTLS_COLOR } from '../../../data/policies';

const UNKNOWN_COLOR = '#6c757d';

function colorFor(scope: MtlsScope | 'unknown' | undefined): string {
  if (!scope || scope === 'unknown') return UNKNOWN_COLOR;
  return MTLS_COLOR[scope] ?? UNKNOWN_COLOR;
}

const pillStyle: React.CSSProperties = {
  display: 'inline-flex',
  padding: '2px 8px',
  borderRadius: 3,
  fontSize: 11,
  fontWeight: 700,
  letterSpacing: '0.04em',
  color: '#0b0d0f',
  textTransform: 'uppercase',
  fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
};

const dotStyle: React.CSSProperties = {
  display: 'inline-block',
  width: 8,
  height: 8,
  borderRadius: 2,
  flexShrink: 0,
};

export function MtlsChip({
  scope, label, variant = 'pill', muted = false, title,
}: {
  scope:    MtlsScope | 'unknown' | undefined;
  label?:   string;
  variant?: 'pill' | 'dot';
  muted?:   boolean;
  title?:   string;
}) {
  const color = colorFor(scope);
  const opacity = muted ? 0.4 : 1;
  if (variant === 'dot') {
    return (
      <span
        aria-hidden={!title}
        title={title}
        style={{ ...dotStyle, background: color, opacity }}
      />
    );
  }
  return (
    <span title={title} style={{ ...pillStyle, background: color, opacity }}>
      {label ?? scope ?? 'unknown'}
    </span>
  );
}
