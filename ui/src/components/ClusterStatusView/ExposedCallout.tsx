// High-signal banner: workloads facing the internet with no workload-level
// policy. Rendered only when the count > 0 — silence when the cluster is
// clean, loud when it isn't. Always rendered red, same as the Issues stat
// card — this is a page-worthy finding, not a severity-graded one.
//
// Each chip carries TWO discoverable actions: click the label side to open
// node details, click the trailing graph link to jump to the graph. Split
// via a single bordered pill with an internal seam so the pair reads as
// "one thing + drill" instead of two peer buttons.
//
// Badges are the real internet-* status keys via the shared StatusBadges
// component (same symbols/colors as PolicyGraph + DetailPanel), not a
// custom direction pill.

import type { StatusKey, WorkloadNode } from '../../data/policies';
import { SEVERITY_COLOR } from '../../data/policies';
import { useGraphStore } from '../../store/graphStore';
import { StatusBadges } from '../DetailPanel/shared/status';
import styles from './exposed-chip.module.css';
import s from '../DetailPanel/DetailPanel.module.css';

interface ExposedCalloutProps {
  nodes:      WorkloadNode[];
  onSeeAll:   () => void;
}

const PREVIEW_MAX = 4;

const INTERNET_STATUSES: StatusKey[] = ['internet-full', 'internet-ingress', 'internet-egress'];

function internetStatusesOf(node: WorkloadNode): StatusKey[] {
  return (node.statuses ?? []).filter((key) => INTERNET_STATUSES.includes(key));
}

export default function ExposedCallout({ nodes, onSeeAll }: ExposedCalloutProps) {
  const setSelectedNode = useGraphStore((s) => s.setSelectedNode);
  const setView         = useGraphStore((s) => s.setView);

  if (nodes.length === 0) return null;

  const preview  = nodes.slice(0, PREVIEW_MAX);
  const overflow = nodes.length - preview.length;
  const color    = SEVERITY_COLOR.high;

  const openNode  = (node: WorkloadNode) => setSelectedNode(node);
  const openGraph = (node: WorkloadNode) => { setSelectedNode(node); setView('graph'); };

  return (
    <div className={s.semanticDeny} style={{ borderLeftColor: color }}>
      <div className="d-flex align-items-baseline justify-content-between gap-2">
        <div className="d-flex align-items-baseline gap-2">
          <span className={s.eyebrow} style={{ color }}>Exposed &amp; unpoliced</span>
          <span className={`${s.hero} tnum`} style={{ color }}>{nodes.length}</span>
        </div>
        <button type="button" className={s.ghostButton} onClick={onSeeAll}>
          See in workloads →
        </button>
      </div>
      <div className={s.body}>
        Facing the internet with no workload-level policy covering them.
      </div>
      <div className="d-flex flex-wrap gap-2">
        {preview.map((node) => (
          <div key={node.id} className={styles.exposedChip}>
            <button
              type="button"
              className={styles.exposedChipMain}
              onClick={() => openNode(node)}
              title={`Show ${node.label} details`}
            >
              <span className={s.section}>{node.label}</span>
              <span className={`${s.dim} ${s.mono} ms-1`}>{node.namespace}</span>
              <span className="ms-2">
                <StatusBadges keys={internetStatusesOf(node)} />
              </span>
            </button>
            <button
              type="button"
              className={styles.exposedChipAction}
              onClick={() => openGraph(node)}
              title={`Open ${node.label} in the graph`}
            >graph →</button>
          </div>
        ))}
        {overflow > 0 && (
          <button type="button" className={s.ghostButton} onClick={onSeeAll}>
            +{overflow} more
          </button>
        )}
      </div>
    </div>
  );
}
