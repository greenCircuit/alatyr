// V2 callout: single left danger stripe carries severity. Body copy is light
// (no dueling reds). Each chip carries TWO discoverable actions: click the
// label side to open node details, click the trailing graph icon to jump to
// graph. Split via inner btn-group but chevron is subtler than the label so
// the hierarchy reads as "one thing with a secondary drill" (not two peers).

import { SEVERITY_COLOR } from '../../data/policies';
import type { ExposedNodeLite, ExposureDirection } from './mockData';
import styles from './exposed-chip.module.css';

// Direction carries severity — bidirectional is worst (internet <-> internal),
// ingress and egress are both bad but distinct triage paths. Colors mirror
// the severity scale so the operator's eye ranks the chips at a glance.
const DIRECTION_COLOR: Record<ExposureDirection, string> = {
  both:    SEVERITY_COLOR.critical,
  ingress: SEVERITY_COLOR.high,
  egress:  SEVERITY_COLOR.warning,
};

const DIRECTION_HINT: Record<ExposureDirection, string> = {
  both:    'reachable from internet AND can call out — worst case',
  ingress: 'reachable from internet',
  egress:  'can reach internet (exfil path)',
};

interface Props {
  nodes:         ExposedNodeLite[];
  onOpenNode?:   (node: ExposedNodeLite) => void;
  onOpenGraph?:  (node: ExposedNodeLite) => void;
  onSeeAll?:     () => void;
}

const PREVIEW_MAX = 4;

export default function ExposedCalloutV2({ nodes, onOpenNode, onOpenGraph, onSeeAll }: Props) {
  if (nodes.length === 0) return null;
  const preview  = nodes.slice(0, PREVIEW_MAX);
  const overflow = nodes.length - preview.length;

  return (
    <div
      className="bg-dark border border-secondary border-l-5 rounded d-flex flex-column gap-1 py-2 px-3"
      style={{ borderLeftColor: SEVERITY_COLOR.high }}
    >
      <div className="d-flex align-items-baseline justify-content-between gap-2">
        <div className="d-flex align-items-baseline gap-2">
          <span className="text-danger fs-12 fw-bold text-uppercase tracking-wide">
            Exposed &amp; unpoliced
          </span>
          <span className="text-danger tnum fs-13 fw-bold">{nodes.length}</span>
        </div>
        {onSeeAll && (
          <button
            type="button"
            className="btn btn-sm btn-link p-0 fs-12 text-secondary"
            onClick={onSeeAll}
          >See in workloads →</button>
        )}
      </div>
      <div className="text-light fs-12">
        Facing the internet with no workload-level policy covering them.
      </div>
      <div className="d-flex flex-wrap gap-2 fs-12 mt-1">
        {preview.map((node) => (
          <div key={node.id} className={styles.exposedChip}>
            <button
              type="button"
              className={styles.exposedChipMain}
              onClick={() => onOpenNode?.(node)}
              title={`Show ${node.label} details`}
            >
              <span className="text-light">{node.label}</span>
              <span className="text-secondary ms-1">· {node.namespace}</span>
              {node.port && (
                <span className="text-secondary ms-1 tnum">· :{node.port}</span>
              )}
              {node.direction && (
                <span
                  className="ms-1 text-uppercase fs-11 fw-semibold"
                  style={{ color: DIRECTION_COLOR[node.direction] }}
                  title={DIRECTION_HINT[node.direction]}
                >{node.direction === 'both' ? '⇆' : node.direction === 'ingress' ? '↓' : '↑'} {node.direction}</span>
              )}
            </button>
            <button
              type="button"
              className={styles.exposedChipAction}
              onClick={() => onOpenGraph?.(node)}
              title={`Open ${node.label} in the graph`}
            >graph →</button>
          </div>
        ))}
        {overflow > 0 && (
          <button
            type="button"
            className="btn btn-sm btn-link text-secondary fs-12 py-0 px-1"
            onClick={onSeeAll}
          >+{overflow} more</button>
        )}
      </div>
    </div>
  );
}
