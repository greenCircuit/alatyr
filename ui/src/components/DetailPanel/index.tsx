// DetailPanel orchestrator. Reads selection state from the graph store and
// dispatches to one of three views: reachability (when a source is pinned),
// workload detail (single-node click), or edge list (one or more selected
// edges). Each view delegates row/badge rendering to parts/ siblings.

import { useGraphStore } from '../../store/graphStore';
import { SEVERITY_COLOR } from '../../data/policies';
import s from './DetailPanel.module.css';
import { PolicyRow, RuleRow, PolicyRefRow } from './parts/policy';
import { MeshCard } from './parts/mesh';
import { ReachabilityPane } from './parts/reachability';
import { StatusBadges } from './parts/status';

export default function DetailPanel() {
  const {
    selectedNode, selectedEdges, nodeInfo, nodeInfoLoading,
    reachabilitySource, reachability, reachabilityLoading, reachabilityTarget,
    setSelectedNode, setSelectedEdges, pinReachabilitySource, clearReachability,
  } = useGraphStore();

  if (!selectedNode && selectedEdges.length === 0 && !reachabilitySource) return null;

  const close = () => { setSelectedNode(null); setSelectedEdges([]); clearReachability(); };
  // Wide layout kicks in once we have *something* reachability-related to render
  // (loading or result). Avoids stretching the panel before the user has clicked
  // the target node.
  const reachActive = !!reachabilitySource && (reachabilityLoading || !!reachability);
  const headerLabel = reachActive
    ? 'Reachability'
    : selectedNode
      ? 'Workload'
      : `${selectedEdges.length} polic${selectedEdges.length === 1 ? 'y' : 'ies'}`;

  return (
    <div className={`text-light border border-secondary rounded shadow ${s.panel} ${reachActive ? s.panelWide : ''}`}>
      <div className="d-flex justify-content-between align-items-center px-3 py-2 border-bottom border-secondary">
        <span className="fw-bold small">{headerLabel}</span>
        <button className="btn-close btn-close-white btn-sm" onClick={close} />
      </div>

      <div className="p-3">
        {reachabilitySource && (
          <div className="border border-info rounded p-2 mb-2">
            <div className="d-flex justify-content-between align-items-center">
              <div>
                <span className="badge bg-info text-dark me-2">SRC</span>
                <span className="fw-semibold">{reachabilitySource.label}</span>
                <span className="text-secondary"> / {reachabilitySource.namespace || '—'}</span>
              </div>
              <button className="btn btn-sm btn-outline-light" onClick={clearReachability}>
                Cancel
              </button>
            </div>
            <div className={`text-secondary mt-1 ${s.smallText}`}>
              {reachabilityTarget
                ? <>Target: {reachabilityTarget.label}</>
                : <>Click another node to check reachability</>}
            </div>
          </div>
        )}

        {reachabilityLoading && (
          <div className="text-secondary small mb-2">Computing reachability…</div>
        )}

        {reachability && reachabilitySource && reachabilityTarget && (
          <ReachabilityPane
            src={reachabilitySource}
            dst={reachabilityTarget}
            result={reachability}
          />
        )}

        {!reachActive && selectedNode && (
          <div className="d-flex flex-column gap-2">
            <div className={s.fieldRow}>
              <div className={s.fieldLabel}>Name:</div>
              <div className="fw-semibold text-break">{selectedNode.label}</div>
            </div>
            <div className={s.fieldRow}>
              <div className={s.fieldLabel}>Namespace:</div>
              <div>{selectedNode.namespace || '—'}</div>
            </div>
            <div className={s.fieldRow}>
              <div className={s.fieldLabel}>Type:</div>
              <div>{selectedNode.type}</div>
            </div>

            {nodeInfo?.issues && nodeInfo.issues.length > 0 && (
              <div
                className="border rounded p-2 d-flex flex-column gap-1"
                style={{ borderLeftColor: SEVERITY_COLOR.warning, borderLeftWidth: 5, borderColor: SEVERITY_COLOR.warning }}
              >
                <div
                  className="fw-semibold text-uppercase"
                  style={{ color: SEVERITY_COLOR.warning }}
                >
                  ⚠ {nodeInfo.issues.length} issue{nodeInfo.issues.length > 1 ? 's' : ''} detected
                </div>
                <ul className={`ps-3 mb-0 ${s.smallText} text-light`}>
                  {nodeInfo.issues.map((issue, i) => <li key={i}>{issue}</li>)}
                </ul>
              </div>
            )}

            {!reachabilitySource && (
              <button
                className="btn btn-sm btn-outline-info align-self-start"
                onClick={() => pinReachabilitySource(selectedNode)}
              >
                Pin as reachability source
              </button>
            )}
            <div>
              <div className="text-secondary">Labels</div>
              <div className="d-flex flex-column align-items-start gap-1 mt-1">
                {Object.entries(selectedNode.labels).map(([k, v]) => (
                  <span key={k} className={`badge bg-secondary ${s.badgeSm}`}>{k}={v}</span>
                ))}
                {Object.keys(selectedNode.labels).length === 0 && '—'}
              </div>
            </div>

            <div>
              <div className="text-secondary">Status (effective)</div>
              <StatusBadges keys={selectedNode.statuses ?? []} />
            </div>

            {(() => {
              const engines = new Set<string>([
                ...Object.keys(selectedNode.statusesBySource ?? {}),
                ...Object.keys(nodeInfo?.policies ?? {}),
              ]);
              if (engines.size === 0) return null;
              return (
                <div>
                  <div className="text-secondary">By policy engine</div>
                  {nodeInfoLoading && (
                    <div className="text-secondary small mt-1">Loading rules + policies…</div>
                  )}
                  <div className="d-flex flex-column gap-2 mt-1">
                    {[...engines].map((engine) => {
                      const keys = selectedNode.statusesBySource?.[engine] ?? [];
                      const info = nodeInfo?.policies?.[engine];
                      return (
                        <div key={engine} className="border border-secondary rounded p-2 mt-4">
                          <div className={`fw-semibold`}>{engine}</div>
                          <StatusBadges keys={keys} />
                          {info?.policies && info.policies.length > 0 && (
                            <div className="mt-2">
                              <div className="mt-4">Selecting policies</div>
                              {info.policies.map((policyRef, i) => <PolicyRefRow key={i} policyRef={policyRef} />)}
                            </div>
                          )}
                          {info?.rules && info.rules.length > 0 && (
                            <div className="mt-2">
                              <div className="mt-4">Outbound rules</div>
                              {info.rules.map((rule, i) => <RuleRow key={i} rule={rule} />)}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              );
            })()}

            {nodeInfo?.mesh && Object.keys(nodeInfo.mesh).length > 0 && (
              <div>
                <div className="text-secondary">Mesh</div>
                <div className="d-flex flex-column gap-2 mt-1">
                  {Object.entries(nodeInfo.mesh).map(([source, m]) => (
                    <MeshCard key={source} source={source} membership={m} />
                  ))}
                </div>
              </div>
            )}

          </div>
        )}

        {selectedEdges.length > 0 && (
          <>
            {selectedEdges.length > 1 && (
              <div className={`text-secondary mb-2 ${s.smallText}`}>
                {selectedEdges.length} policies on this connection
              </div>
            )}
            <div className={selectedEdges.length > 1 ? s.policyGrid : ''}>
              {selectedEdges.map((p) => <PolicyRow key={p.id} p={p} />)}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
