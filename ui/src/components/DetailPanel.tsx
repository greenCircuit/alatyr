import { useGraphStore } from '../store/graphStore';
import type { PolicyEdge } from '../data/policies';

function PolicyRow({ p }: { p: PolicyEdge }) {
  return (
    <div className="border border-secondary rounded p-2 mb-2" style={{ fontSize: 11 }}>
      <div className="d-flex justify-content-between align-items-start mb-1">
        <span className="fw-semibold text-light" style={{ wordBreak: 'break-all' }}>{p.policyName}</span>
        <span className={`badge ms-1 flex-shrink-0 ${p.level === 'namespace' ? 'bg-warning text-dark' : 'bg-secondary'}`}>
          {p.level}
        </span>
      </div>
      <div className="text-secondary mb-1">ns: {p.namespace} · {p.direction}</div>
      <div className="d-flex flex-wrap gap-1">
        {p.ports?.map((pt) => (
          <span key={pt.port} className="badge bg-info text-dark">{pt.port}/{pt.protocol}</span>
        )) ?? <span className="text-secondary">all ports</span>}
      </div>
    </div>
  );
}

export default function DetailPanel() {
  const { selectedNode, selectedEdges, setSelectedNode, setSelectedEdges } = useGraphStore();

  if (!selectedNode && selectedEdges.length === 0) return null;

  const close = () => { setSelectedNode(null); setSelectedEdges([]); };

  return (
    <div
      className="position-absolute bg-dark text-light border border-secondary rounded shadow"
      style={{ top: 12, right: 12, width: 290, maxHeight: 'calc(100vh - 24px)', overflowY: 'auto', zIndex: 10 }}
    >
      {/* header */}
      <div className="d-flex justify-content-between align-items-center px-3 py-2 border-bottom border-secondary">
        <span className="fw-bold small">
          {selectedNode
            ? 'Workload'
            : `${selectedEdges.length} polic${selectedEdges.length === 1 ? 'y' : 'ies'}`}
        </span>
        <button className="btn-close btn-close-white btn-sm" onClick={close} />
      </div>

      <div className="p-3">
        {/* workload detail */}
        {selectedNode && (
          <table className="table table-sm table-dark table-borderless mb-0">
            <tbody>
              <tr>
                <td className="text-secondary" style={{ width: 80 }}>Name</td>
                <td className="fw-semibold">{selectedNode.label}</td>
              </tr>
              <tr>
                <td className="text-secondary">Namespace</td>
                <td>{selectedNode.namespace || '—'}</td>
              </tr>
              <tr>
                <td className="text-secondary">Type</td>
                <td>{selectedNode.type}</td>
              </tr>
              <tr>
                <td className="text-secondary">Labels</td>
                <td>
                  {Object.entries(selectedNode.labels).map(([k, v]) => (
                    <span key={k} className="badge bg-secondary me-1 mb-1" style={{ fontSize: '0.7rem' }}>
                      {k}={v}
                    </span>
                  ))}
                  {Object.keys(selectedNode.labels).length === 0 && '—'}
                </td>
              </tr>
            </tbody>
          </table>
        )}

        {/* edge / bundle detail */}
        {selectedEdges.length > 0 && (
          <>
            {selectedEdges.length > 1 && (
              <div className="text-secondary mb-2" style={{ fontSize: 11 }}>
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
