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

// Dot side length per size — xs for dense legends/chips, sm for standalone rows.
const DOT_PX: Record<'xs' | 'sm', number> = { xs: 8, sm: 10 };

const dotStyle: React.CSSProperties = {
  display: 'inline-block',
  borderRadius: 2,
  flexShrink: 0,
};

export function MtlsChip({
  scope, label, variant = 'pill', size = 'sm', muted = false, title,
}: {
  scope:    MtlsScope | 'unknown' | undefined;
  label?:   string;
  variant?: 'pill' | 'dot';
  size?:    'xs' | 'sm';
  muted?:   boolean;
  title?:   string;
}) {
  const color = colorFor(scope);
  const opacity = muted ? 0.4 : 1;
  if (variant === 'dot') {
    const px = DOT_PX[size];
    return (
      <span
        aria-hidden={!title}
        title={title}
        style={{ ...dotStyle, width: px, height: px, background: color, opacity }}
      />
    );
  }
  return (
    <span title={title} style={{ ...pillStyle, background: color, opacity }}>
      {label ?? scope ?? 'unknown'}
    </span>
  );
}
