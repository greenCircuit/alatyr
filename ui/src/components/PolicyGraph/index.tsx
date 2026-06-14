// PolicyGraph orchestrator. Mounts a Cytoscape instance, wires
// pan/zoom/click handlers, runs layout when filters change, and renders the
// HTML overlay for status badges + the legend. Pure helpers (coverage,
// bundling, element building, stylesheet) live in parts/.

import { useCallback, useEffect, useRef, useState } from 'react';
import cytoscape from 'cytoscape';
// @ts-ignore
import dagre from 'cytoscape-dagre';
// @ts-ignore
import cola from 'cytoscape-cola';
// @ts-ignore
import fcose from 'cytoscape-fcose';
import { useGraphStore } from '../../store/graphStore';
import type { WorkloadNode, PolicyEdge, StatusKey } from '../../data/policies';
import DetailPanel from '../DetailPanel';
import { buildElements, buildAggregatedElements } from './parts/elements';
import { STYLE } from './parts/styles';
import { StatusBadge, Legend, type BadgeNode } from './parts/legend';

cytoscape.use(dagre);
cytoscape.use(cola);
cytoscape.use(fcose);

export default function PolicyGraph() {
  const containerRef  = useRef<HTMLDivElement>(null);
  const cyRef         = useRef<cytoscape.Core | null>(null);
  const badgeLayerRef = useRef<HTMLDivElement>(null);
  const [badgeNodes, setBadgeNodes] = useState<BadgeNode[]>([]);
  const [legendOpen, setLegendOpen] = useState(true);
  const badgeDivRefs  = useRef<Map<string, HTMLDivElement>>(new Map());

  // Refs so layoutstop callback can read latest selection state without stale closures
  const selectedNodeRef       = useRef<WorkloadNode | null>(null);
  const selectedStatusesRef   = useRef<Set<StatusKey>>(new Set());
  const reachabilitySourceRef = useRef<WorkloadNode | null>(null);
  const reachabilityTargetRef = useRef<WorkloadNode | null>(null);

  // Write badge div positions from current Cytoscape node bounding boxes (no React re-render)
  const syncBadgePositions = useCallback(() => {
    const cy = cyRef.current;
    if (!cy) return;
    badgeDivRefs.current.forEach((div, id) => {
      const n = cy.getElementById(id);
      if (n.empty()) return;
      const bb = n.boundingBox({});
      div.style.left = `${bb.x1 + 2}px`;
      div.style.top  = `${bb.y1 + 2}px`;
    });
  }, []);

  // Mirror Cytoscape viewport transform onto the badge overlay layer (no React re-render)
  const syncViewport = useCallback(() => {
    const cy = cyRef.current;
    if (!cy || !badgeLayerRef.current) return;
    const { x, y } = cy.pan();
    const z = cy.zoom();
    badgeLayerRef.current.style.transform = `translate(${x}px,${y}px) scale(${z})`;
  }, []);

  // Re-sync positions whenever badge nodes are re-rendered (after React commit)
  useEffect(() => { syncBadgePositions(); }, [badgeNodes, syncBadgePositions]);

  const {
    allNodes, allEdges,
    filteredNodes, filteredEdges, selectedNamespaces,
    selectedNodeTypes, searchQuery, showNamespaceEdges, showConnectedNamespaces,
    aggregateByNamespace,
    selectedStatuses,
    selectedPolicySources, selectedActions,
    selectedNode,
    setSelectedNode, setSelectedEdges,
    loadGraph, loadClusterState, loading, error,
    layoutAlgorithm,
    reachabilitySource, reachabilityTarget,
    pinReachabilitySource,
  } = useGraphStore();

  // Apply dimming: reach mode > node-click mode > status filter mode.
  // Reach mode suppresses dimming entirely so the operator can scan candidate
  // targets; src/dst get colored borders instead.
  const applyDimming = useCallback(() => {
    const cy = cyRef.current;
    if (!cy) return;
    cy.elements().removeClass('dimmed focused reach-src reach-dst');
    const reachSrc = reachabilitySourceRef.current;
    const reachDst = reachabilityTargetRef.current;
    if (reachSrc) {
      cy.getElementById(reachSrc.id).addClass('reach-src');
      if (reachDst) cy.getElementById(reachDst.id).addClass('reach-dst');
      badgeDivRefs.current.forEach((div) => { div.style.opacity = '1'; });
      return;
    }
    const node     = selectedNodeRef.current;
    const statuses = selectedStatusesRef.current;
    let focusedIds: Set<string> | null = null;
    if (node) {
      const cyNode = cy.getElementById(node.id);
      if (cyNode.length) {
        const neighborhood = cyNode.closedNeighborhood();
        const toFocus = neighborhood.union(neighborhood.nodes().parents());
        cy.elements().not(toFocus).addClass('dimmed');
        toFocus.addClass('focused');
        focusedIds = new Set(toFocus.map((e) => e.id()));
      }
    } else if (statuses.size > 0) {
      const matching = cy.nodes('[ntype = "workload"]').filter((n) => {
        const wl = n.data('workload') as WorkloadNode;
        return wl.statuses?.some((s) => statuses.has(s)) ?? false;
      });
      if (matching.length > 0) {
        const toFocus = matching.union(matching.parents());
        cy.elements().not(toFocus).addClass('dimmed');
        toFocus.addClass('focused');
        focusedIds = new Set(toFocus.map((e) => e.id()));
      }
    }
    // Mirror dim state onto badge overlay divs (Cytoscape classes don't reach HTML overlays)
    badgeDivRefs.current.forEach((div, id) => {
      div.style.opacity = focusedIds === null || focusedIds.has(id) ? '1' : '0.12';
    });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Keep refs in sync and re-apply dimming whenever selection or status filter changes
  useEffect(() => {
    selectedNodeRef.current       = selectedNode;
    selectedStatusesRef.current   = selectedStatuses;
    reachabilitySourceRef.current = reachabilitySource;
    reachabilityTargetRef.current = reachabilityTarget;
    applyDimming();
  }, [selectedNode, selectedStatuses, reachabilitySource, reachabilityTarget, applyDimming]);

  // Re-apply dim state to badges when the badge list changes (after layout adds new badges)
  useEffect(() => { applyDimming(); }, [badgeNodes, applyDimming]);

  useEffect(() => { loadClusterState(); loadGraph(); }, []);

  // Mount once: create Cytoscape instance and wire event handlers
  useEffect(() => {
    if (!containerRef.current) return;

    const cy = cytoscape({
      container: containerRef.current,
      style:     STYLE,
      elements:  [],
      layout:    { name: 'preset' },
      minZoom:   0.08,
      maxZoom:   4,
    });
    cyRef.current = cy;

    cy.on('viewport', syncViewport);

    // Sync badge positions when any node is dragged (namespace box or workload)
    let dragRaf: number | null = null;
    cy.on('drag', 'node', () => {
      if (dragRaf !== null) return;
      dragRaf = requestAnimationFrame(() => { dragRaf = null; syncBadgePositions(); });
    });

    // Workload / namespace node click – open detail panel for whichever node was tapped.
    // Toggle focus when the same node is clicked twice.
    cy.on('tap', 'node[ntype = "workload"], node[ntype = "namespace"], node[ntype = "namespace-agg"]', (evt) => {
      const node = evt.target as cytoscape.NodeSingular;
      const workload = node.data('workload') as WorkloadNode | undefined;
      if (!workload) return;
      const wasActive = node.hasClass('focused');
      setSelectedNode(wasActive ? null : workload);
    });

    // Right-click on a node – pin as reachability source directly. Faster than
    // clicking the node, opening the detail panel, then clicking the "Pin as
    // reachability source" button.
    cy.on('cxttap', 'node[ntype = "workload"], node[ntype = "namespace"], node[ntype = "namespace-agg"]', (evt) => {
      const node = evt.target as cytoscape.NodeSingular;
      const workload = node.data('workload') as WorkloadNode | undefined;
      if (!workload) return;
      pinReachabilitySource(workload);
    });

    // Edge click – show policy detail
    cy.on('tap', 'edge', (evt) => {
      setSelectedEdges((evt.target as cytoscape.EdgeSingular).data('policies') as PolicyEdge[]);
    });

    // Background click – clear everything
    cy.on('tap', (evt) => {
      if (evt.target === cy) {
        setSelectedNode(null);
        setSelectedEdges([]);
      }
    });

    return () => { cy.destroy(); cyRef.current = null; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Re-build elements and re-run layout whenever filters change
  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) return;

    const workloads   = filteredNodes();
    const policyEdges = filteredEdges();
    const elements    = aggregateByNamespace
      ? buildAggregatedElements(workloads, allEdges, allNodes)
      : buildElements(workloads, policyEdges, new Set(workloads.map((w) => w.namespace).filter(Boolean) as string[]), allNodes);

    cy.edges().remove();
    cy.nodes().remove();
    cy.add(elements);

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const layoutOptions: any =
      layoutAlgorithm === 'dagre' ? {
        name: 'dagre', rankDir: 'LR', padding: 60,
        spacingFactor: 1.4, nodeSep: 50, rankSep: 110, animate: false,
      } :
      layoutAlgorithm === 'cola' ? {
        name: 'cola', padding: 60, animate: false,
        nodeSpacing: 40, edgeLength: 150,
      } :
      layoutAlgorithm === 'fcose' ? {
        name: 'fcose', padding: 60, animate: false,
        idealEdgeLength: 150, nodeRepulsion: 8000,
      } :
      layoutAlgorithm === 'cose' ? {
        name: 'cose', padding: 60, animate: false,
        nodeRepulsion: 8000, idealEdgeLength: 150,
      } :
      layoutAlgorithm === 'breadthfirst' ? {
        name: 'breadthfirst', padding: 60, animate: false, directed: true,
      } :
      { name: layoutAlgorithm, padding: 60, animate: false };

    const layout = cy.layout(layoutOptions);

    layout.one('layoutstop', () => {
      // Pin the label-only ns child above its topmost workload sibling, then
      // make it ungrabbable (not locked — lock blocks compound drag from
      // pulling the child along, which makes the box resize instead of move).
      cy.nodes('[ntype = "namespace"]').forEach((node) => {
        const parent = node.parent().first();
        if (parent.empty()) return;
        const siblings = parent.children().filter((child) => child.data('ntype') === 'workload');
        if (siblings.empty()) return;
        let minTop = Infinity;
        let centerX = 0;
        let count = 0;
        siblings.forEach((sibling) => {
          const top = sibling.position('y') - sibling.height() / 2;
          if (top < minTop) minTop = top;
          centerX += sibling.position('x');
          count += 1;
        });
        node.position({
          x: centerX / count,
          y: minTop - 20,
        });
        node.ungrabify();
      });

      setBadgeNodes(
        cy.nodes('[ntype = "workload"]').toArray()
          .filter((n) => ((n.data('workload') as WorkloadNode).statuses?.length ?? 0) > 0)
          .map((n) => ({ id: n.id(), statuses: (n.data('workload') as WorkloadNode).statuses! }))
      );
      syncViewport();
      applyDimming();
    });

    layout.run();
  // filteredNodes/filteredEdges call get() internally; the listed deps cover all state that affects output
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allNodes, allEdges, selectedNamespaces, selectedNodeTypes, selectedPolicySources, selectedActions, searchQuery, showNamespaceEdges, showConnectedNamespaces, aggregateByNamespace, layoutAlgorithm, applyDimming]);

  return (
    <div style={{ flex: 1, position: 'relative', background: '#0d0f11', overflow: 'hidden' }}>
      {loading && (
        <div style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 20, color: '#adb5bd' }}>
          Loading graph…
        </div>
      )}
      {error && (
        <div style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 20, color: '#dc3545' }}>
          {error}
        </div>
      )}
      <div ref={containerRef} style={{ width: '100%', height: '100%' }} />

      {/* Status badge overlay – graph-space coords, only CSS transform changes on pan/zoom */}
      <div style={{ position: 'absolute', inset: 0, pointerEvents: 'none', overflow: 'hidden' }}>
        <div ref={badgeLayerRef} style={{ position: 'absolute', top: 0, left: 0, transformOrigin: '0 0' }}>
          {badgeNodes.map(({ id, statuses }) => (
            <div
              key={id}
              ref={(el) => {
                if (el) badgeDivRefs.current.set(id, el);
                else    badgeDivRefs.current.delete(id);
              }}
              style={{ position: 'absolute', display: 'flex', gap: 2 }}
            >
              {statuses.map((s) => <StatusBadge key={s} s={s} />)}
            </div>
          ))}
        </div>
      </div>

      <Legend open={legendOpen} onToggle={() => setLegendOpen((o) => !o)} />

      <DetailPanel />
    </div>
  );
}
