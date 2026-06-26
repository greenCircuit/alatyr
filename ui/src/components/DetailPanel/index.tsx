// DetailPanel orchestrator. Reads selection state from the graph store and
// dispatches to one of three views: reachability (when a source is pinned),
// workload detail (single-node click), or edge list (one or more selected
// edges). Each view delegates row/badge rendering to parts/ siblings.

import { useGraphStore } from '../../store/graphStore';
import { SEVERITY_COLOR } from '../../data/policies';
import s from './DetailPanel.module.css';
import { PolicyRow, RuleRow, PolicyRefRow, EngineBadge, EdgeReachabilityBanner, EndpointCard, AffectedPairsList, PolicyHeader, groupEdgesByPair } from './parts/policy';
import { MeshCard } from './parts/mesh';
import { ReachabilityPane } from './parts/reachability';
import { StatusBadges } from './parts/status';

export default function DetailPanel() {
  const {
    allNodes,
    selectedNode, selectedEdges, nodeInfo, nodeInfoLoading,
    reachabilitySource, reachability, reachabilityLoading, reachabilityTarget,
    edgeReachability, edgeReachabilityLoading,
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
      : 'Connection';

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
                          <EngineBadge engine={engine} />
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

        {selectedEdges.length > 0 && (() => {
          // Bundle from a graph-edge click always shares one (src, dst) pair.
          // Bundle from a Policies-table click groups by policy and can span
          // many pairs — in that case show the pair list and let the user
          // narrow, instead of lying with endpoint cards from edges[0] only.
          const first = selectedEdges[0];
          const pairs = groupEdgesByPair(selectedEdges);
          const multiPair = pairs.length > 1;
          // For ns-level edges, first.source/target are the `ns-<name>` nodes —
          // resolved here so EndpointCard can render labels and ns labels just
          // like a workload edge. Without this the panel showed neither side.
          const src = multiPair ? undefined : allNodes.find((n) => n.id === first.source);
          const dst = multiPair ? undefined : allNodes.find((n) => n.id === first.target);
          if (multiPair) {
            // Policies-table bundle — one policy, many pairs. The PolicyHeader
            // carries shared identity (name/engine/ns/action/directions) once
            // and AffectedPairsList stands in for the per-rule view.
            return (
              <>
                <PolicyHeader edges={selectedEdges} />
                <AffectedPairsList pairs={pairs} nodes={allNodes} onSelect={setSelectedEdges} />
              </>
            );
          }
          return (
            <>
              <EdgeReachabilityBanner result={edgeReachability} loading={edgeReachabilityLoading} />
              <div className="d-flex flex-column gap-1 mb-3">
                <EndpointCard role="src" node={src} />
                <EndpointCard role="dst" node={dst} />
              </div>
              <div className={`text-secondary mb-2 ${s.smallText}`}>
                Policies ({selectedEdges.length})
              </div>
              <div className={selectedEdges.length > 1 ? s.policyGrid : ''}>
                {selectedEdges.map((p) => <PolicyRow key={p.id} p={p} />)}
              </div>
            </>
          );
        })()}
      </div>
    </div>
  );
}
