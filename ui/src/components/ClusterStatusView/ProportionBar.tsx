// Generic segmented proportion bar. Only meaningful over a PARTITION — every
// item in exactly one segment — otherwise the proportions lie. Callers own the
// bucketing; zero-count segments are skipped.

import styles from './CoverageBar.module.css';

export interface BarSegment {
  key:   string;
  count: number;
  color: string;
}

export default function ProportionBar({ segments }: { segments: BarSegment[] }) {
  const total = segments.reduce((sum, segment) => sum + segment.count, 0);
  if (total === 0) return null;
  return (
    <div className={`d-flex rounded overflow-hidden ${styles.bar}`}>
      {segments.filter((segment) => segment.count > 0).map((segment) => (
        <div
          key={segment.key}
          style={{ flexGrow: segment.count, background: segment.color }}
          title={`${segment.key}: ${segment.count} of ${total}`}
        />
      ))}
    </div>
  );
}
