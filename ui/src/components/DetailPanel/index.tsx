// DetailPanel orchestrator. Reads selection state from the graph store and
// dispatches to one of three views: reachability (when a source is pinned),
// workload detail (single-node click), or edge list (one or more selected
// edges). Each view delegates row/badge rendering to parts/ siblings.

import type { CSSProperties } from 'react';
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
          <div className="d-flex flex-column gap-3">
            {/* Identity header: name as page title, namespace + type as
                subtitle. Pin-reachability is the headline action and sits
                next to the title — promoting it from middle-of-flow. */}
            <div className="d-flex flex-column gap-1">
              <div className="d-flex justify-content-between align-items-start gap-2">
                <div className="fs-5 fw-bold text-break">{selectedNode.label}</div>
                {!reachabilitySource && (
                  <button
                    className="btn btn-sm btn-info flex-shrink-0"
                    onClick={() => pinReachabilitySource(selectedNode)}
                  >
                    Pin as reachability source
                  </button>
                )}
              </div>
              <div className="d-flex align-items-center gap-2 small">
                <span className="text-secondary">{selectedNode.namespace || '—'}</span>
                <span className="badge bg-secondary">{selectedNode.type}</span>
              </div>
            </div>

            <div>
              <div className="text-uppercase text-secondary small fw-semibold mb-1">Labels</div>
              <div className="d-flex flex-wrap gap-1">
                {Object.entries(selectedNode.labels).map(([k, v]) => (
                  <span key={k} className={`badge bg-secondary ${s.badgeSm}`}>{k}={v}</span>
                ))}
                {Object.keys(selectedNode.labels).length === 0 && '—'}
              </div>
            </div>

            {/* Effective verdict — hero. Boxed so it reads as the answer,
                not a label-value row. Per-engine cards below are evidence. */}
            <div className="border border-secondary rounded p-3">
              <div className="text-uppercase text-secondary small fw-semibold mb-2">Effective status</div>
              <StatusBadges keys={selectedNode.statuses ?? []} />
            </div>

            {nodeInfo?.issues && nodeInfo.issues.length > 0 && (
              <div
                className={`border rounded p-2 d-flex flex-column gap-1 ${s.calloutAccent}`}
                style={{ '--accent': SEVERITY_COLOR.warning, borderColor: SEVERITY_COLOR.warning } as CSSProperties}
              >
                <div className={`fw-semibold text-uppercase ${s.calloutAccentText}`}>
                  ⚠ {nodeInfo.issues.length} issue{nodeInfo.issues.length > 1 ? 's' : ''} detected
                </div>
                <ul className={`ps-3 mb-0 ${s.smallText} text-light`}>
                  {nodeInfo.issues.map((issue, i) => <li key={i}>{issue}</li>)}
                </ul>
              </div>
            )}

            {(() => {
              const engines = new Set<string>([
                ...Object.keys(selectedNode.statusesBySource ?? {}),
                ...Object.keys(nodeInfo?.policies ?? {}),
              ]);
              if (engines.size === 0) return null;
              return (
                <div>
                  <div className="text-uppercase text-secondary small fw-semibold mb-1">Per-engine evidence</div>
                  {nodeInfoLoading && (
                    <div className="text-secondary small mb-1">Loading rules + policies…</div>
                  )}
                  <div className="d-flex flex-column gap-2">
                    {[...engines].map((engine) => {
                      const keys = selectedNode.statusesBySource?.[engine] ?? [];
                      const info = nodeInfo?.policies?.[engine];
                      return (
                        <div key={engine} className="border border-secondary rounded p-2">
                          <EngineBadge engine={engine} />
                          {selectedNode.statusesBySource?.[engine] && <StatusBadges keys={keys} />}
                          {info?.policies && info.policies.length > 0 && (
                            <div className="mt-3">
                              <div className="text-uppercase text-secondary small fw-semibold mb-1">Selecting policies</div>
                              {info.policies.map((policyRef, i) => <PolicyRefRow key={i} policyRef={policyRef} />)}
                            </div>
                          )}
                          {info?.rules && info.rules.length > 0 && (
                            <div className="mt-3">
                              <div className="text-uppercase text-secondary small fw-semibold mb-1">Outbound rules</div>
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
                <div className="text-uppercase text-secondary small fw-semibold mb-1">Mesh</div>
                <div className="d-flex flex-column gap-2">
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
