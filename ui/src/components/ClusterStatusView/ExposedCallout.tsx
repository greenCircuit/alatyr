// High-signal banner: workloads facing the internet with no workload-level
// policy. Rendered only when the count > 0 — silence when the cluster is
// clean, loud when it isn't. Row-click lands on the node in the graph so an
// operator can see fan-out + edit the pod's policy immediately.

import type { WorkloadNode } from '../../data/policies';
import { useGraphStore } from '../../store/graphStore';

interface ExposedCalloutProps {
  nodes:      WorkloadNode[];
  onSeeAll:   () => void;
}

const PREVIEW_MAX = 4;

export default function ExposedCallout({ nodes, onSeeAll }: ExposedCalloutProps) {
  const setSelectedNode = useGraphStore((s) => s.setSelectedNode);
  const setView         = useGraphStore((s) => s.setView);

  if (nodes.length === 0) return null;

  const preview  = nodes.slice(0, PREVIEW_MAX);
  const overflow = nodes.length - preview.length;

  // Row click opens the DetailPanel over the status page (stays on scan view).
  // Trailing icon-button jumps to graph so operators can see fan-out when they
  // want it, without stealing the click.
  const showNode = (node: WorkloadNode) => setSelectedNode(node);
  const openInGraph = (node: WorkloadNode) => { setSelectedNode(node); setView('graph'); };

  return (
    <div className="alert alert-danger bg-dark border-danger text-danger py-2 px-3 mb-0 d-flex flex-column gap-1">
      <div className="d-flex align-items-baseline justify-content-between gap-2">
        <div className="fs-12 fw-bold text-uppercase tracking-wide">
          Exposed &amp; unpoliced <span className="badge bg-danger ms-1">{nodes.length}</span>
        </div>
        <button type="button" className="btn btn-link btn-sm p-0 text-danger fs-12" onClick={onSeeAll}>
          See in workloads →
        </button>
      </div>
      <div className="text-light fs-12">
        Facing the internet with no workload-level policy covering them.
      </div>
      <div className="d-flex flex-wrap gap-2 fs-12 mt-1">
        {preview.map((node) => (
          <span key={node.id} className="btn-group btn-group-sm" role="group">
            <button
              type="button"
              className="btn btn-sm btn-dark border border-secondary text-light py-0 px-2 fs-12 text-nowrap"
              onClick={() => showNode(node)}
              title={`Show ${node.label} details`}
            >
              {node.label} <span className="opacity-75">· {node.namespace}</span>
            </button>
            <button
              type="button"
              className="btn btn-sm btn-dark border border-secondary text-light py-0 px-2 fs-12"
              onClick={() => openInGraph(node)}
              title={`Open ${node.label} in the graph`}
            >graph →</button>
          </span>
        ))}
        {overflow > 0 && (
          <span className="align-self-center text-light opacity-75 fs-12">+{overflow} more</span>
        )}
      </div>
    </div>
  );
}
