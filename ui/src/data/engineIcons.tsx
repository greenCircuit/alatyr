// Brand-colored SVG marks for each policy engine. Self-colored (brand fill on
// transparent) so they read correctly both on the dark detail panel and over
// graph arrows — the arrow's direction color never bleeds into the logo.
// Inline SVG keeps them in the JS bundle (no served image assets) for the
// single-binary embed. Simplified but recognizable: k8s helm, Istio sail.

import type { ReactNode } from 'react';
import { engineMeta } from './engines';
import styles from './engineIcons.module.css';

interface LogoProps { color: string; size: number }

// Kubernetes helm: blue heptagon with a white 7-spoke wheel.
function K8sLogo({ color, size }: LogoProps) {
  const vertices = '8,1 13.47,3.64 14.82,9.56 11.04,14.31 4.96,14.31 1.18,9.56 2.53,3.64';
  const points = vertices.split(' ').map((p) => p.split(',').map(Number));
  return (
    <svg viewBox="0 0 16 16" width={size} height={size} aria-hidden="true">
      <polygon points={vertices} fill={color} />
      <g stroke="#fff" strokeWidth={0.7} fill="none">
        {points.map(([x, y], i) => (
          <line key={i} x1={8} y1={8} x2={8 + (x - 8) * 0.62} y2={8 + (y - 8) * 0.62} />
        ))}
      </g>
    </svg>
  );
}

// Istio sail: two blue sails over a hull — the project's sailboat mark.
function IstioLogo({ color, size }: LogoProps) {
  return (
    <svg viewBox="0 0 16 16" width={size} height={size} aria-hidden="true">
      <path d="M7.4 1 L7.4 11.5 L2 11.5 Z" fill={color} />
      <path d="M8.6 4 L13 11.5 L8.6 11.5 Z" fill={color} opacity={0.7} />
      <rect x={1.5} y={12.4} width={13} height={1.6} rx={0.8} fill={color} />
    </svg>
  );
}

// Calico: a paw print — four toes over a pad, nodding at the project's cat
// mark without redrawing the leopard head (unreadable at 14px). Toe spread
// kept tight and pad kept fat so the glyph reads as one dense mass at size=10
// where the k8s heptagon and Istio sail are single-unit reads.
function CalicoLogo({ color, size }: LogoProps) {
  const toes = [
    { cx: 4.3, cy: 5.4 },
    { cx: 6.9, cy: 3.7 },
    { cx: 9.1, cy: 3.7 },
    { cx: 11.7, cy: 5.4 },
  ];
  return (
    <svg viewBox="0 0 16 16" width={size} height={size} aria-hidden="true">
      {toes.map(({ cx, cy }) => (
        <ellipse key={cx} cx={cx} cy={cy} rx={1.9} ry={2.4} fill={color} />
      ))}
      <path d="M8 14.8c-3 0-5-1.55-5-3.6C3 8.6 5.4 6.9 8 6.9s5 1.7 5 4.3c0 2.05-2 3.6-5 3.6Z" fill={color} />
    </svg>
  );
}

// Unknown engine: brand-tinted rounded square with the first letter.
function FallbackLogo({ color, size, letter }: LogoProps & { letter: string }) {
  return (
    <svg viewBox="0 0 16 16" width={size} height={size} aria-hidden="true">
      <rect x={1} y={1} width={14} height={14} rx={3} fill={color} />
      <text x={8} y={11.5} textAnchor="middle" fontSize={9} fontWeight={700} fill="#fff">
        {letter}
      </text>
    </svg>
  );
}

export function EngineLogo({ engine, size = 14 }: { engine: string; size?: number }) {
  const { color } = engineMeta(engine);
  if (engine === 'k8s') return <K8sLogo color={color} size={size} />;
  // pa (PeerAuthentication) is an Istio CRD, not a separate engine — same sail.
  if (engine === 'istio' || engine === 'pa') return <IstioLogo color={color} size={size} />;
  if (engine === 'calico') return <CalicoLogo color={color} size={size} />;
  return <FallbackLogo color={color} size={size} letter={engine.charAt(0).toUpperCase()} />;
}

// Engine provenance chip — brand hue tinted bg + hue dot + logo + name mono.
// Mockup shape: dot + tinted-bg carry engine identity; no border, no white pill.
// `suffix` appends after the name (e.g. ": 3" for the workloads count rollup).
export function EngineBadge({ engine, size = 12, suffix }: { engine: string; size?: number; suffix?: ReactNode }) {
  const { label, color, short } = engineMeta(engine);
  return (
    <span
      className={styles.engineChip}
      title={label}
      style={{ ['--engine-color' as never]: color }}
    >
      <span className={styles.engineDot} />
      <EngineLogo engine={engine} size={size} /> {short ?? engine}{suffix}
    </span>
  );
}
