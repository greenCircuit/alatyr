// DetailPanel orchestrator. Reads selection state from the graph store and
// dispatches to one of four views: reachability (a source is pinned),
// workload detail (single-node click), policy bundle (Policies-table row →
// one policy across many pairs), or edge detail (single src→dst pair). Each
// view lives in views/ and composes the shared/ primitives.

import { useGraphStore } from '../../store/graphStore';
import s from './DetailPanel.module.css';
import { ManifestDrawer } from './shared/ManifestModal';
import { groupEdgesByPair } from './shared/groupEdgesByPair';
import { ReachabilityView } from './views/ReachabilityView';
import { WorkloadView } from './views/WorkloadView';
import { EdgeView } from './views/EdgeView';
import { PolicyBundleView } from './views/PolicyBundleView';

export default function DetailPanel() {
  const {
    allNodes,
    selectedNode, selectedEdges, nodeInfo, nodeInfoLoading,
    reachabilitySource, reachability, reachabilityLoading, reachabilityTarget,
    edgeReachability, edgeReachabilityLoading,
    setSelectedNode, setSelectedEdges, pinReachabilitySource, clearReachability,
  } = useGraphStore();

  // Dock + manifest drawer always render so YAML can open even with no panel
  // selection (e.g. a YAML click straight from the policies table).
  const hasSelection = !!selectedNode || selectedEdges.length > 0 || !!reachabilitySource;

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

  // Policies-table bundle spans many pairs; a graph-arrow click is one pair.
  const multiPair = selectedEdges.length > 0 && groupEdgesByPair(selectedEdges).length > 1;

  return (
    <div className={s.rightDock}>
      {/* Manifest drawer sits to the LEFT of the panel; panel stays pinned right. */}
      <ManifestDrawer />

      {hasSelection && (
        <div className={`text-light border border-secondary rounded shadow ${s.panel} ${reachActive ? s.panelWide : ''}`}>
          <div className="d-flex justify-content-between align-items-center px-3 py-2 border-bottom border-secondary">
            <span className="fw-bold small">{headerLabel}</span>
            <button className="btn-close btn-close-white btn-sm" onClick={close} />
          </div>

          <div className="p-3">
            {reachabilitySource && (
              <ReachabilityView
                source={reachabilitySource}
                target={reachabilityTarget}
                loading={reachabilityLoading}
                result={reachability}
                onClear={clearReachability}
              />
            )}

            {!reachActive && selectedNode && (
              <WorkloadView
                node={selectedNode}
                nodeInfo={nodeInfo}
                nodeInfoLoading={nodeInfoLoading}
                canPin={!reachabilitySource}
                onPin={pinReachabilitySource}
              />
            )}

            {selectedEdges.length > 0 && (
              multiPair
                ? <PolicyBundleView edges={selectedEdges} nodes={allNodes} onSelect={setSelectedEdges} />
                : <EdgeView
                    edges={selectedEdges}
                    nodes={allNodes}
                    reachability={edgeReachability}
                    reachabilityLoading={edgeReachabilityLoading}
                  />
            )}
          </div>
        </div>
      )}
    </div>
  );
}
