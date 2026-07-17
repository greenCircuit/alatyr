// High-signal banner: workloads facing the internet with no workload-level
// policy. Rendered only when the count > 0 — silence when the cluster is
// clean, loud when it isn't.
//
// Each chip carries TWO discoverable actions: click the label side to open
// node details, click the trailing graph link to jump to the graph. Split
// via a single bordered pill with an internal seam so the pair reads as
// "one thing + drill" instead of two peer buttons.
//
// Direction (ingress/egress/both) is derived from the node's internet-*
// status keys so an operator can triage before clicking.

import type { WorkloadNode } from '../../data/policies';
import { useGraphStore } from '../../store/graphStore';
import styles from './exposed-chip.module.css';
import s from '../DetailPanel/DetailPanel.module.css';

interface ExposedCalloutProps {
  nodes:      WorkloadNode[];
  onSeeAll:   () => void;
}

const PREVIEW_MAX = 4;

type ExposureDirection = 'ingress' | 'egress' | 'both';

const DIRECTION_OUTLINE: Record<ExposureDirection, string> = {
  both:    s.dirOutlineBoth,
  ingress: s.dirOutlineIngress,
  egress:  s.dirOutlineEgress,
};

const DIRECTION_HINT: Record<ExposureDirection, string> = {
  both:    'reachable from internet AND can call out — worst case',
  ingress: 'reachable from internet',
  egress:  'can reach internet (exfil path)',
};

const DIRECTION_GLYPH: Record<ExposureDirection, string> = {
  both:    '⇆',
  ingress: '↓',
  egress:  '↑',
};

function directionOf(node: WorkloadNode): ExposureDirection | undefined {
  const keys = node.statuses ?? [];
  if (keys.includes('internet-full')) return 'both';
  if (keys.includes('internet-ingress')) return 'ingress';
  if (keys.includes('internet-egress')) return 'egress';
  return undefined;
}

export default function ExposedCallout({ nodes, onSeeAll }: ExposedCalloutProps) {
  const setSelectedNode = useGraphStore((s) => s.setSelectedNode);
  const setView         = useGraphStore((s) => s.setView);

  if (nodes.length === 0) return null;

  const preview  = nodes.slice(0, PREVIEW_MAX);
  const overflow = nodes.length - preview.length;

  const openNode  = (node: WorkloadNode) => setSelectedNode(node);
  const openGraph = (node: WorkloadNode) => { setSelectedNode(node); setView('graph'); };

  return (
    <div className={s.semanticDeny}>
      <div className="d-flex align-items-baseline justify-content-between gap-2">
        <div className="d-flex align-items-baseline gap-2">
          <span className={`${s.eyebrow} ${s.eyebrowDeny}`}>Exposed &amp; unpoliced</span>
          <span className={`${s.hero} ${s.verdictTextDeny} tnum`}>{nodes.length}</span>
        </div>
        <button type="button" className={s.ghostButton} onClick={onSeeAll}>
          See in workloads →
        </button>
      </div>
      <div className={s.body}>
        Facing the internet with no workload-level policy covering them.
      </div>
      <div className="d-flex flex-wrap gap-2">
        {preview.map((node) => {
          const direction = directionOf(node);
          return (
            <div key={node.id} className={styles.exposedChip}>
              <button
                type="button"
                className={styles.exposedChipMain}
                onClick={() => openNode(node)}
                title={`Show ${node.label} details`}
              >
                <span className={s.section}>{node.label}</span>
                <span className={`${s.dim} ${s.mono} ms-1`}>· {node.namespace}</span>
                {direction && (
                  <span
                    className={`ms-2 ${s.dirOutline} ${DIRECTION_OUTLINE[direction]}`}
                    title={DIRECTION_HINT[direction]}
                  >{DIRECTION_GLYPH[direction]} {direction}</span>
                )}
              </button>
              <button
                type="button"
                className={styles.exposedChipAction}
                onClick={() => openGraph(node)}
                title={`Open ${node.label} in the graph`}
              >graph →</button>
            </div>
          );
        })}
        {overflow > 0 && (
          <button type="button" className={s.ghostButton} onClick={onSeeAll}>
            +{overflow} more
          </button>
        )}
      </div>
    </div>
  );
}
