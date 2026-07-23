// Tiny status glyph library replacing emoji in headings (styleguide §8 bans
// them — inconsistent rendering across OS + poor legibility at 12-14px).
// All icons inherit currentColor so the caller's text color drives them.

interface IconProps {
  size?: number;
  title?: string;
}

function iconProps({ size = 12, title }: IconProps) {
  return {
    width: size,
    height: size,
    viewBox: '0 0 16 16',
    fill: 'currentColor',
    'aria-hidden': !title,
    role: title ? 'img' : undefined,
    'aria-label': title,
    style: { verticalAlign: '-2px', flexShrink: 0 } as React.CSSProperties,
  };
}

// Warning triangle — replaces ⚠ in verdict headers.
export function WarnIcon(props: IconProps = {}) {
  return (
    <svg {...iconProps(props)}>
      <path d="M8 1.6 15 14H1L8 1.6zm0 4v4.2m0 1.6v1.2" stroke="currentColor" strokeWidth="1.4" fill="none" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

// Check mark — replaces ✓ in "healthy" headers.
export function CheckIcon(props: IconProps = {}) {
  return (
    <svg {...iconProps(props)}>
      <path d="M2.5 8.5l3.5 3.5 7.5-8" stroke="currentColor" strokeWidth="1.8" fill="none" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

// Shield — replaces ⛨ next to the mesh-overlay toggle.
export function ShieldIcon(props: IconProps = {}) {
  return (
    <svg {...iconProps(props)}>
      <path d="M8 1.5 2.5 3.5V8c0 3 2.4 5.4 5.5 6.4 3.1-1 5.5-3.4 5.5-6.4V3.5L8 1.5z" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
    </svg>
  );
}
