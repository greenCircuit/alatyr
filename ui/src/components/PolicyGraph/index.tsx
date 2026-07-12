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
import { buildElements, buildAggregatedElements } from './parts/elements';
import { edgeEngines } from './parts/bundling';
import { buildIssueMarks, pairKey, type IssueMark } from './parts/issueMarkers';
import { STYLE } from './parts/styles';
import { StatusBadge, Legend, type BadgeNode } from './parts/legend';
import { EngineLogo } from '../../data/engineIcons';
import { SEVERITY_COLOR } from '../../data/policies';
import s from './PolicyGraph.module.css';

interface EdgeIcon { id: string; engines: string[] }
// One rendered issue marker: element id + resolved mark. Node markers pin to
// the node's top-right corner, edge markers hang below the edge midpoint.
interface ElementIssueMark extends IssueMark { id: string }

// Filled pill: tier color as background, text color picked for contrast
// (white on red, near-black on amber).
const TIER_STYLE: Record<IssueMark['tier'], { background: string; color: string }> = {
  blocking: { background: SEVERITY_COLOR.high,    color: '#fff' },
  warning:  { background: SEVERITY_COLOR.warning, color: '#1a1d20' },
};

cytoscape.use(dagre);
cytoscape.use(cola);
cytoscape.use(fcose);

export default function PolicyGraph() {
  const containerRef  = useRef<HTMLDivElement>(null);
  const cyRef         = useRef<cytoscape.Core | null>(null);
  const badgeLayerRef = useRef<HTMLDivElement>(null);
  const [badgeNodes, setBadgeNodes] = useState<BadgeNode[]>([]);
  const [edgeIcons, setEdgeIcons]   = useState<EdgeIcon[]>([]);
  const [nodeIssueMarks, setNodeIssueMarks] = useState<ElementIssueMark[]>([]);
  const [edgeIssueMarks, setEdgeIssueMarks] = useState<ElementIssueMark[]>([]);
  const [legendOpen, setLegendOpen] = useState(true);
  const badgeDivRefs        = useRef<Map<string, HTMLDivElement>>(new Map());
  const edgeIconDivRefs     = useRef<Map<string, HTMLDivElement>>(new Map());
  const nodeIssueDivRefs    = useRef<Map<string, HTMLDivElement>>(new Map());
  const edgeIssueDivRefs    = useRef<Map<string, HTMLDivElement>>(new Map());

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

  // Write engine-icon div positions from each edge's midpoint (graph-space).
  // Icons are centered on the midpoint via a CSS translate, so only left/top
  // change here — the overlay layer transform handles pan/zoom.
  const syncEdgeIconPositions = useCallback(() => {
    const cy = cyRef.current;
    if (!cy) return;
    edgeIconDivRefs.current.forEach((div, id) => {
      const edge = cy.getElementById(id);
      if (edge.empty()) return;
      const mid = edge.midpoint();
      // Lift above the midpoint so the icon clears the edge's port/L7 label,
      // which Cytoscape draws centered on the midpoint.
      div.style.left = `${mid.x}px`;
      div.style.top  = `${mid.y - 16}px`;
    });
  }, []);

  // Issue markers: node marks at the node's top-right corner (status badges own
  // the top-left), edge marks below the midpoint (engine icons own above it).
  const syncIssueMarkPositions = useCallback(() => {
    const cy = cyRef.current;
    if (!cy) return;
    nodeIssueDivRefs.current.forEach((div, id) => {
      const cyNode = cy.getElementById(id);
      if (cyNode.empty()) return;
      const bb = cyNode.boundingBox({});
      div.style.left = `${bb.x2 - 2}px`;
      div.style.top  = `${bb.y1 + 2}px`;
    });
    edgeIssueDivRefs.current.forEach((div, id) => {
      const edge = cy.getElementById(id);
      if (edge.empty()) return;
      const mid = edge.midpoint();
      div.style.left = `${mid.x}px`;
      div.style.top  = `${mid.y + 16}px`;
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
    aggregateByNamespace, showEngineIcons,
    issues,
    selectedStatuses,
    selectedPolicySources, selectedActions, selectedDirections,
    selectedNode,
    setSelectedNode, setSelectedEdges,
    loading, error,
    layoutAlgorithm,
    reachabilitySource, reachabilityTarget,
    pinReachabilitySource,
  } = useGraphStore();

  // Rebuild the engine-icon list when the toggle flips or the graph is rebuilt
  // (badgeNodes changes once per layout, so it doubles as a "graph ready" cue).
  useEffect(() => {
    const cy = cyRef.current;
    if (!cy || !showEngineIcons) { setEdgeIcons([]); return; }
    setEdgeIcons(
      cy.edges().toArray().map((edge) => ({
        id:      edge.id(),
        engines: edgeEngines((edge.data('policies') as PolicyEdge[]) ?? []),
      })),
    );
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showEngineIcons, badgeNodes]);

  // Position icons after they render
  useEffect(() => { syncEdgeIconPositions(); }, [edgeIcons, syncEdgeIconPositions]);

  // Resolve issue marks against the rendered elements whenever issues arrive or
  // the graph is rebuilt (badgeNodes doubles as the "layout done" cue). A pair
  // can render as several parallel edges (allow + deny bundles) — the mark goes
  // on the first one so a conflict shows once, not once per bundle.
  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) { setNodeIssueMarks([]); setEdgeIssueMarks([]); return; }
    const marks = buildIssueMarks(issues, aggregateByNamespace);
    const nodeMarks: ElementIssueMark[] = [];
    cy.nodes().forEach((cyNode) => {
      const mark = marks.nodes.get(cyNode.id());
      if (mark) nodeMarks.push({ id: cyNode.id(), ...mark });
    });
    const edgeMarks: ElementIssueMark[] = [];
    const markedPairs = new Set<string>();
    cy.edges().forEach((edge) => {
      const key = pairKey(edge.data('source'), edge.data('target'));
      if (markedPairs.has(key)) return;
      const mark = marks.pairs.get(key);
      if (mark) {
        markedPairs.add(key);
        edgeMarks.push({ id: edge.id(), ...mark });
      }
    });
    setNodeIssueMarks(nodeMarks);
    setEdgeIssueMarks(edgeMarks);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [issues, aggregateByNamespace, badgeNodes]);

  // Position issue markers after they render
  useEffect(() => { syncIssueMarkPositions(); }, [nodeIssueMarks, edgeIssueMarks, syncIssueMarkPositions]);

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
      edgeIconDivRefs.current.forEach((div) => { div.style.opacity = '1'; });
      nodeIssueDivRefs.current.forEach((div) => { div.style.opacity = '1'; });
      edgeIssueDivRefs.current.forEach((div) => { div.style.opacity = '1'; });
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
    // Mirror dim state onto overlay divs (Cytoscape classes don't reach HTML
    // overlays). Edge icons key on the edge id, which is in focusedIds when the
    // edge is part of the focused neighborhood.
    badgeDivRefs.current.forEach((div, id) => {
      div.style.opacity = focusedIds === null || focusedIds.has(id) ? '1' : '0.12';
    });
    edgeIconDivRefs.current.forEach((div, id) => {
      div.style.opacity = focusedIds === null || focusedIds.has(id) ? '1' : '0.12';
    });
    nodeIssueDivRefs.current.forEach((div, id) => {
      div.style.opacity = focusedIds === null || focusedIds.has(id) ? '1' : '0.12';
    });
    edgeIssueDivRefs.current.forEach((div, id) => {
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

  // Re-apply dim state to overlays when the badge or engine-icon list changes
  // (after layout adds new badges, or the engine-icon toggle flips)
  useEffect(() => { applyDimming(); }, [badgeNodes, edgeIcons, nodeIssueMarks, edgeIssueMarks, applyDimming]);


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
      dragRaf = requestAnimationFrame(() => { dragRaf = null; syncBadgePositions(); syncEdgeIconPositions(); syncIssueMarkPositions(); });
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
  // showEngineIcons intentionally excluded: it's a pure overlay toggle (no
  // element/label change), so it must not trigger a relayout that reshuffles
  // the graph. The edge-icon overlay reacts to it separately below.
  }, [allNodes, allEdges, selectedNamespaces, selectedNodeTypes, selectedPolicySources, selectedActions, selectedDirections, searchQuery, showNamespaceEdges, showConnectedNamespaces, aggregateByNamespace, layoutAlgorithm, applyDimming]);

  return (
    <div className={`position-relative overflow-hidden ${s.canvas}`}>
      {loading && (
        <div className={`position-absolute inset-0 d-flex align-items-center justify-content-center ${s.centeredOverlay} ${s.loading}`}>
          Loading graph…
        </div>
      )}
      {error && (
        <div className={`position-absolute inset-0 d-flex align-items-center justify-content-center ${s.centeredOverlay} ${s.error}`}>
          {error}
        </div>
      )}
      <div ref={containerRef} className="w-100 h-100" />

      {/* Status badge overlay – graph-space coords, only CSS transform changes on pan/zoom */}
      <div className={`position-absolute inset-0 overflow-hidden ${s.overlayLayer}`}>
        <div ref={badgeLayerRef} className={`position-absolute top-0 start-0 ${s.badgeLayer}`}>
          {badgeNodes.map(({ id, statuses }) => (
            <div
              key={id}
              ref={(el) => {
                if (el) badgeDivRefs.current.set(id, el);
                else    badgeDivRefs.current.delete(id);
              }}
              className={`position-absolute d-flex ${s.badgeGroup}`}
            >
              {statuses.map((key) => <StatusBadge key={key} s={key} />)}
            </div>
          ))}

          {/* Engine provenance icons at edge midpoints. translate(-50%) centers
              on the midpoint; brand color is owned by the SVG, independent of
              the arrow's direction color. */}
          {edgeIcons.map(({ id, engines }) => (
            <div
              key={id}
              ref={(el) => {
                if (el) edgeIconDivRefs.current.set(id, el);
                else    edgeIconDivRefs.current.delete(id);
              }}
              className={`position-absolute ${s.iconChip}`}
            >
              {engines.map((engine) => <EngineLogo key={engine} engine={engine} size={13} />)}
            </div>
          ))}

          {/* Issue markers — misconfig channel, separate from status badges.
              Node marks pin top-right; edge marks flag "this allow edge doesn't
              mean traffic flows" (blocked by another engine). */}
          {nodeIssueMarks.map(({ id, tier, count }) => (
            <div
              key={`ni-${id}`}
              ref={(el) => {
                if (el) nodeIssueDivRefs.current.set(id, el);
                else    nodeIssueDivRefs.current.delete(id);
              }}
              className={`position-absolute ${s.issueMark}`}
              style={TIER_STYLE[tier]}
            >
              <span aria-hidden="true">⚠</span>
              {count > 1 && <span>{count}</span>}
            </div>
          ))}
          {edgeIssueMarks.map(({ id, tier, count }) => (
            <div
              key={`ei-${id}`}
              ref={(el) => {
                if (el) edgeIssueDivRefs.current.set(id, el);
                else    edgeIssueDivRefs.current.delete(id);
              }}
              className={`position-absolute ${s.issueMark}`}
              style={TIER_STYLE[tier]}
            >
              <span aria-hidden="true">⚠</span>
              {count > 1 && <span>{count}</span>}
            </div>
          ))}
        </div>
      </div>

      <Legend open={legendOpen} onToggle={() => setLegendOpen((o) => !o)} />
    </div>
  );
}
