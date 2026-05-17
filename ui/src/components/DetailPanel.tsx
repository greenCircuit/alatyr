import { useGraphStore } from '../store/graphStore';
import type { PolicyEdge } from '../data/policies';
import { formatPort } from '../data/policies';
import s from './DetailPanel.module.css';

function PolicyRow({ p }: { p: PolicyEdge }) {
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-2">
        <div className="fw-semibold text-light text-break">{p.policyName}</div>
        <span className={`badge flex-shrink-0 ${p.level === 'namespace' ? 'bg-warning text-dark' : 'bg-secondary'}`}>
          {p.level}
        </span>
      </div>
      <div className="mb-1">
        <div className="text-secondary">Namespace</div>
        <div>{p.namespace}</div>
      </div>
      <div className="mb-1">
        <div className="text-secondary">Direction</div>
        <div>{p.direction}</div>
      </div>
      <div>
        <div className="text-secondary">Ports</div>
        <div className="d-flex flex-column align-items-start gap-1 mt-1">
          {p.ports?.map((pt, i) => (
            <span key={i} className="badge bg-info text-dark">{formatPort(pt)}/{pt.protocol}</span>
          )) ?? <div>all ports</div>}
        </div>
      </div>
    </div>
  );
}

export default function DetailPanel() {
  const { selectedNode, selectedEdges, setSelectedNode, setSelectedEdges } = useGraphStore();

  if (!selectedNode && selectedEdges.length === 0) return null;

  const close = () => { setSelectedNode(null); setSelectedEdges([]); };

  return (
    <div className={`text-light border border-secondary rounded shadow ${s.panel}`}>
      <div className="d-flex justify-content-between align-items-center px-3 py-2 border-bottom border-secondary">
        <span className="fw-bold small">
          {selectedNode
            ? 'Workload'
            : `${selectedEdges.length} polic${selectedEdges.length === 1 ? 'y' : 'ies'}`}
        </span>
        <button className="btn-close btn-close-white btn-sm" onClick={close} />
      </div>

      <div className="p-3">
        {selectedNode && (
          <div className="d-flex flex-column gap-2">
            <div>
              <div className="text-secondary">Name</div>
              <div className="fw-semibold text-break">{selectedNode.label}</div>
            </div>
            <div>
              <div className="text-secondary">Namespace</div>
              <div>{selectedNode.namespace || '—'}</div>
            </div>
            <div>
              <div className="text-secondary">Type</div>
              <div>{selectedNode.type}</div>
            </div>
            <div>
              <div className="text-secondary">Labels</div>
              <div className="d-flex flex-column align-items-start gap-1 mt-1">
                {Object.entries(selectedNode.labels).map(([k, v]) => (
                  <span key={k} className={`badge bg-secondary ${s.badgeSm}`}>{k}={v}</span>
                ))}
                {Object.keys(selectedNode.labels).length === 0 && '—'}
              </div>
            </div>
          </div>
        )}

        {selectedEdges.length > 0 && (
          <>
            {selectedEdges.length > 1 && (
              <div className={`text-secondary mb-2 ${s.smallText}`}>
                {selectedEdges.length} policies on this connection
              </div>
            )}
            {selectedEdges.map((p) => <PolicyRow key={p.id} p={p} />)}
          </>
        )}
      </div>
    </div>
  );
}
