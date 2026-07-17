// Rule-coverage posture: segmented proportion bar + a chip per coverage class.
// Zero-count chips stay visible (muted) — "no allow-all policies" is the
// signal an operator came here for; hiding it would look like the class
// isn't checked.

import type { Coverage } from '../../data/policies';
import { SEVERITY_COLOR } from '../../data/policies';
import type { CoverageStats } from '../../store/clusterStats';
import { COVERAGE_CLASSES } from '../../store/clusterStats';
import ProportionBar from './ProportionBar';

// Explicit color map, not severity — coverage classes aren't severities.
// Green = tight, blue = locked, yellow/red = open, gray = no-op.
const COVERAGE_COLOR: Record<Coverage, string> = {
  'restricted':   SEVERITY_COLOR.secure,
  'deny all':     SEVERITY_COLOR.info,
  'allow all ns': SEVERITY_COLOR.caution,
  'allow all':    SEVERITY_COLOR.high,
  'unenforced':   '#868e96',
  'audit':        SEVERITY_COLOR.warning,
};

const COVERAGE_DESCRIPTION: Record<Coverage, string> = {
  'restricted':   'rules name specific peers',
  'deny all':     'default-deny lock — this policy allows nothing',
  'allow all ns': 'opens traffic to/from an entire namespace',
  'allow all':    'no peer restriction — open to everything',
  'unenforced':   'policy names the direction but adds no constraint',
  'audit':        'audit mode — observed, not enforced',
};

export default function CoverageBar({ stats }: { stats: CoverageStats }) {
  const total = COVERAGE_CLASSES.reduce((sum, coverageClass) => sum + stats.byClass[coverageClass], 0);
  return (
    <div className="d-flex flex-column gap-2">
      {total > 0 ? (
        <ProportionBar segments={COVERAGE_CLASSES.map((coverageClass) => ({
          key: coverageClass, count: stats.byClass[coverageClass], color: COVERAGE_COLOR[coverageClass],
        }))} />
      ) : (
        <div className="text-secondary fs-12">No policies in the current scope.</div>
      )}
      <div className="d-flex align-items-center flex-wrap gap-3 fs-12">
        {COVERAGE_CLASSES.map((coverageClass) => {
          const count = stats.byClass[coverageClass];
          const zero  = count === 0;
          return (
            <span
              key={coverageClass}
              className="d-inline-flex align-items-center gap-1"
              title={COVERAGE_DESCRIPTION[coverageClass]}
            >
              <span
                className="swatch-dot"
                style={{ background: COVERAGE_COLOR[coverageClass], opacity: zero ? 0.4 : 1 }}
              />
              <span className={zero ? 'text-secondary' : 'text-light'}>{coverageClass}</span>
              <span className={`tnum ${zero ? 'text-secondary opacity-50' : 'text-secondary'}`}>{count}</span>
            </span>
          );
        })}
      </div>
    </div>
  );
}
