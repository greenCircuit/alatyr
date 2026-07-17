// Generic segmented proportion bar. Only meaningful over a PARTITION — every
// item in exactly one segment — otherwise the proportions lie. Callers own the
// bucketing; zero-count segments are skipped. Height is overridable so the
// same bar can render as a primary posture chart (14px) or a secondary
// micro-strip (8px) when a chip row beneath already carries the detail.

export interface BarSegment {
  key:   string;
  count: number;
  color: string;
}

export default function ProportionBar({ segments, height = 14 }: {
  segments: BarSegment[]; height?: number;
}) {
  const total = segments.reduce((sum, segment) => sum + segment.count, 0);
  if (total === 0) return null;
  return (
    <div
      className="d-flex w-100 overflow-hidden"
      style={{ height, gap: 1, borderRadius: 3 }}
    >
      {segments.filter((segment) => segment.count > 0).map((segment) => (
        <div
          key={segment.key}
          style={{ flexGrow: segment.count, background: segment.color, minWidth: 2 }}
          title={`${segment.key}: ${segment.count} of ${total}`}
        />
      ))}
    </div>
  );
}
