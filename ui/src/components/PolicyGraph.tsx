import { useCallback, useEffect, useRef, useState } from 'react';
import cytoscape from 'cytoscape';
// @ts-ignore
import dagre from 'cytoscape-dagre';
// @ts-ignore
import cola from 'cytoscape-cola';
// @ts-ignore
import fcose from 'cytoscape-fcose';
import { useGraphStore } from '../store/graphStore';
import type { WorkloadNode, PolicyEdge, StatusKey } from '../data/policies';
import { formatPort } from '../data/policies';
import DetailPanel from './DetailPanel';

cytoscape.use(dagre);
cytoscape.use(cola);
cytoscape.use(fcose);

// ── Coverage colors (encode policy coverage, not namespace identity) ──────────
const COV = {
  workload:  '#20c997',  // green  – workload-level policies present
  namespace: '#ffc107',  // amber  – namespace-selector policies only
  none:      '#dc3545',  // red    – no policies (gap)
} as const;
type CovType = keyof typeof COV;

function computeCoverage(policyEdges: PolicyEdge[], allNodes: WorkloadNode[]): {
  ns: Record<string, CovType>;
  workload: Set<string>;
} {
  const nodeNS      = new Map(allNodes.map((n) => [n.id, n.namespace]));
  const nsWorkload  = new Set<string>();
  const nsNamespace = new Set<string>();
  const wlCovered   = new Set<string>();

  for (const e of policyEdges) {
    if (e.level === 'workload') {
      wlCovered.add(e.source);
      wlCovered.add(e.target);
      const sNS = nodeNS.get(e.source); if (sNS) nsWorkload.add(sNS);
      const tNS = nodeNS.get(e.target); if (tNS) nsWorkload.add(tNS);
    } else {
      nsNamespace.add(e.source.replace(/^ns-/, ''));
      nsNamespace.add(e.target.replace(/^ns-/, ''));
    }
  }

  // internet-* badges fire when traffic isn't locked — implicit, not proof of a policy.
  // Only treat statuses that require an actual policy as coverage signal.
  for (const n of allNodes) {
    const protective = n.statuses?.filter((s) => s !== 'internet-egress' && s !== 'internet-ingress' && s !== 'internet-full');
    if (protective && protective.length > 0) {
      wlCovered.add(n.id);
      if (n.namespace) nsWorkload.add(n.namespace);
    }
  }

  const ns: Record<string, CovType> = {};
  const derivedNS = [...new Set(allNodes.map((n) => n.namespace).filter(Boolean))];
  for (const n of derivedNS) {
    ns[n] = nsWorkload.has(n) ? 'workload' : nsNamespace.has(n) ? 'namespace' : 'none';
  }
  return { ns, workload: wlCovered };
}

// ── Edge bundling – collapse multiple policies between same pair ───────────────
interface Bundle {
  id: string; source: string; target: string;
  policies: PolicyEdge[]; hasNS: boolean;
  direction: 'egress' | 'ingress' | 'both';
}

function aggregateDirection(policies: PolicyEdge[]): 'egress' | 'ingress' | 'both' {
  const dirs = new Set(policies.map((p) => p.direction));
  if (dirs.has('both') || (dirs.has('egress') && dirs.has('ingress'))) return 'both';
  return dirs.has('egress') ? 'egress' : 'ingress';
}

function bundleEdges(policyEdges: PolicyEdge[]): Bundle[] {
  const map = new Map<string, PolicyEdge[]>();
  for (const e of policyEdges) {
    const key = `${e.source}→${e.target}`;
    if (!map.has(key)) map.set(key, []);
    map.get(key)!.push(e);
  }
  return Array.from(map.values()).map((policies) => ({
    id:        `bnd-${policies[0].source}-${policies[0].target}`,
    source:    policies[0].source,
    target:    policies[0].target,
    policies,
    hasNS:     policies.some((p) => p.level === 'namespace'),
    direction: aggregateDirection(policies),
  }));
}

// ── Cytoscape element builder ─────────────────────────────────────────────────
function buildElements(
  workloads: WorkloadNode[],
  policyEdges: PolicyEdge[],
  visibleNS: Set<string>,
  allNodes: WorkloadNode[],
): cytoscape.ElementDefinition[] {
  const coverage   = computeCoverage(policyEdges, allNodes);
  const bundles    = bundleEdges(policyEdges);
  const els: cytoscape.ElementDefinition[] = [];

  // Only add a namespace box if it has at least one visible workload
  const occupiedNS = new Set(workloads.filter((w) => w.namespace).map((w) => w.namespace));
  const namespaces = [...new Set(allNodes.map((n) => n.namespace).filter(Boolean))];

  // Namespace compound (parent) nodes – must appear before their children
  for (const ns of namespaces) {
    if (!visibleNS.has(ns) || !occupiedNS.has(ns)) continue;
    els.push({
      data: {
        id:       `ns-${ns}`,
        label:    ns,
        ntype:    'namespace',
        covColor: COV[coverage.ns[ns] ?? 'none'],
      },
    });
  }

  // Workload nodes
  for (const w of workloads) {
    const data: Record<string, unknown> = {
      id:       w.id,
      label:    w.label,
      ntype:    'workload',
      wtype:    w.type,
      workload: w,
      covColor: coverage.workload.has(w.id) ? COV.workload : COV.none,
    };
    if (w.namespace && visibleNS.has(w.namespace)) data.parent = `ns-${w.namespace}`;
    els.push({ data });
  }

  // Edge bundles
  for (const b of bundles) {
    const count = b.policies.length;
    els.push({
      data: {
        id:       b.id,
        source:   b.source,
        target:   b.target,
        label:    count === 1
          ? (b.policies[0].ports?.map(formatPort).join(', ') ?? 'all ports')
          : `${count} policies`,
        policies:  b.policies,
        hasNS:     b.hasNS,
        direction: b.direction,
      },
    });
  }

  return els;
}

// ── Aggregated namespace view ─────────────────────────────────────────────────
function buildAggregatedElements(
  workloads: WorkloadNode[],
  allEdges: PolicyEdge[],
  allNodes: WorkloadNode[],
): cytoscape.ElementDefinition[] {
  const coverage = computeCoverage(allEdges, allNodes);
  const els: cytoscape.ElementDefinition[] = [];

  // Look up by all nodes (not just filtered) so cross-NS workload edges are mapped correctly
  const nodeById = new Map(allNodes.map((w) => [w.id, w]));

  // Group ID: ns-{namespace} for namespaced nodes, own id for external
  const groupOf = (id: string): string => {
    const node = nodeById.get(id);
    if (!node) return id;
    return node.namespace ? `ns-${node.namespace}` : node.id;
  };

  // One aggregate node per occupied namespace
  const occupiedNS = new Set(workloads.filter((w) => w.namespace).map((w) => w.namespace));

  // Track which IDs actually exist as elements so we can drop dangling edges
  const validIds = new Set<string>();

  for (const ns of occupiedNS) {
    validIds.add(`ns-${ns}`);
    els.push({
      data: {
        id:       `ns-${ns}`,
        label:    ns,
        ntype:    'namespace-agg',
        covColor: COV[coverage.ns[ns] ?? 'none'],
      },
    });
  }

  // External nodes (no namespace) stay as individual nodes
  for (const w of workloads.filter((w) => !w.namespace)) {
    validIds.add(w.id);
    els.push({
      data: {
        id:       w.id,
        label:    w.label,
        ntype:    'workload',
        wtype:    w.type,
        workload: w,
        covColor: coverage.workload.has(w.id) ? COV.workload : COV.none,
      },
    });
  }

  // Use all edges so workload-level cross-NS policies are included, not just namespace-level ones.
  // validIds gates which namespace pairs actually appear.
  const aggMap = new Map<string, { src: string; tgt: string; policies: PolicyEdge[] }>();
  for (const e of allEdges) {
    const src = e.level === 'namespace' ? e.source : groupOf(e.source);
    const tgt = e.level === 'namespace' ? e.target : groupOf(e.target);
    if (src === tgt) continue;
    if (!validIds.has(src) || !validIds.has(tgt)) continue;
    const key = `${src}\0${tgt}`;
    if (!aggMap.has(key)) aggMap.set(key, { src, tgt, policies: [] });
    aggMap.get(key)!.policies.push(e);
  }

  for (const { src, tgt, policies } of aggMap.values()) {
    const count = policies.length;
    els.push({
      data: {
        id:        `agg-${src}-${tgt}`,
        source:    src,
        target:    tgt,
        label:     count === 1
          ? (policies[0].ports?.map(formatPort).join(', ') ?? 'all ports')
          : `${count} policies`,
        policies,
        hasNS:     policies.some((p) => p.level === 'namespace'),
        direction: aggregateDirection(policies),
      },
    });
  }

  return els;
}

const PANEL_W = 302; // DetailPanel width (290) + right margin (12)

// Fit all elements into the left portion of the canvas, reserving space for the overlay panel.
function fitToArea(cy: cytoscape.Core, reserveRight: number, padding = 60) {
  const els = cy.elements();
  if (els.length === 0) return;
  const w = cy.width();
  const h = cy.height();
  const availW = w - reserveRight - padding * 2;
  const availH = h - padding * 2;
  if (availW <= 0 || availH <= 0) { cy.fit(els, padding); return; }
  const bb = els.boundingBox({});
  if (bb.w === 0 || bb.h === 0) return;
  const zoom = Math.min(availW / bb.w, availH / bb.h, cy.maxZoom());
  const clampedZoom = Math.max(zoom, cy.minZoom());
  cy.viewport({
    zoom: clampedZoom,
    pan: {
      x: padding + availW / 2 - ((bb.x1 + bb.x2) / 2) * clampedZoom,
      y: padding + availH / 2 - ((bb.y1 + bb.y2) / 2) * clampedZoom,
    },
  });
}

// ── Stylesheet ────────────────────────────────────────────────────────────────
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const STYLE: any[] = [
  {
    selector: 'node[ntype = "namespace"]',
    style: {
      'background-color':   '#0a0c0e',
      'background-opacity': 0.65,
      'border-color':       'data(covColor)',
      'border-width':       2,
      'label':              'data(label)',
      'text-valign':        'top',
      'text-halign':        'center',
      'color':              'data(covColor)',
      'font-size':          11,
      'font-weight':        'bold',
      'text-transform':     'uppercase',
      'padding':            32,
      'shape':              'roundrectangle',
      'text-margin-y':      -10,
    },
  },
  {
    selector: 'node[ntype = "namespace-agg"]',
    style: {
      'background-color': '#1a1d20',
      'border-color':     'data(covColor)',
      'border-width':     2,
      'label':            'data(label)',
      'text-valign':      'center',
      'text-halign':      'center',
      'color':            'data(covColor)',
      'font-size':        13,
      'font-weight':      'bold',
      'text-transform':   'uppercase',
      'width':            160,
      'height':           80,
      'shape':            'roundrectangle',
    },
  },
  {
    selector: 'node[ntype = "workload"]',
    style: {
      'background-color': '#1a1d20',
      'border-color':     'data(covColor)',
      'border-width':     2,
      'label':            'data(label)',
      'text-valign':      'center',
      'text-halign':      'center',
      'color':            '#e9ecef',
      'font-size':        10,
      'width':            'label',
      'height':           25,
      'padding':          12,
      'shape':            'roundrectangle',
    },
  },
  // deployment + ClusterIP service: default roundrectangle (most common)
  // deployment only (no service): rectangle – subtle "raw" feel
  {
    selector: 'node[wtype = "deployment"]',
    style: { 'shape': 'rectangle' },
  },
  // headless service: ellipse – pods addressed directly, no stable VIP
  {
    selector: 'node[wtype = "headless"]',
    style: { 'shape': 'ellipse' },
  },
  {
    selector: 'node[wtype = "external"]',
    style: {
      'shape':        'diamond',
      'border-color': '#6c757d',
      'color':        '#adb5bd',
    },
  },
  {
    selector: 'node[wtype = "cronjob"]',
    style: { 'shape': 'hexagon' },
  },
  // ── base edge ──
  {
    selector: 'edge',
    style: {
      'width':                    1.5,
      'target-arrow-shape':       'triangle',
      'arrow-scale':              1.1,
      'curve-style':              'bezier',
      'label':                    'data(label)',
      'font-size':                9,
      'text-background-color':    '#0d0f11',
      'text-background-opacity':  0.9,
      'text-background-padding':  '3px',
      'text-background-shape':    'round-rectangle',
    },
  },
  // ── direction colors ──
  {
    selector: 'edge[direction = "egress"]',
    style: {
      'line-color':         '#4dabf7',   // blue  – traffic going out from source
      'target-arrow-color': '#4dabf7',
      'color':              '#4dabf7',
    },
  },
  {
    selector: 'edge[direction = "ingress"]',
    style: {
      'line-color':         '#f783ac',   // pink  – traffic allowed into target
      'target-arrow-color': '#f783ac',
      'color':              '#f783ac',
    },
  },
  {
    selector: 'edge[direction = "both"]',
    style: {
      'line-color':         '#a9e34b',   // lime  – bidirectional
      'target-arrow-color': '#a9e34b',
      'source-arrow-color': '#a9e34b',
      'source-arrow-shape': 'triangle',
      'color':              '#a9e34b',
    },
  },
  // ── namespace-level edges: dashed on top of direction color ──
  {
    selector: 'edge[?hasNS]',
    style: {
      'line-style':        'dashed',
      'line-dash-pattern': [8, 4],
      'width':             2,
    },
  },
  {
    selector: '.dimmed',
    style: { 'opacity': 0.12 },
  },
];

// ── Status badge definitions ──────────────────────────────────────────────────
const STATUS_CFG: Record<StatusKey, { symbol: string; bg: string; title: string }> = {
  'internet-full':     { symbol: 'WAN⇆', bg: '#581c87', title: 'Critical: bidirectional internet traffic' },
  'internet-egress':   { symbol: 'WAN↑', bg: '#c92a2a', title: 'High: can reach internet (exfil risk)'    },
  'internet-ingress':  { symbol: 'WAN↓', bg: '#c92a2a', title: 'High: reachable from internet'            },
  'lan-full':          { symbol: 'LAN⇆', bg: '#e8590c', title: 'Warning: bidirectional LAN traffic' },
  'lan-egress':        { symbol: 'LAN↑', bg: '#f59f00', title: 'Caution: egress to private LAN' },
  'lan-ingress':       { symbol: 'LAN↓', bg: '#f59f00', title: 'Caution: ingress from private LAN' },
  'api-server-egress': { symbol: 'API↑', bg: '#1864ab', title: 'Info: egress to Kubernetes API server' },
  'ns-full-access':    { symbol: 'NS⇆',  bg: '#e8590c', title: 'Warning: full access to/from entire namespace' },
  'ns-egress-access':  { symbol: 'NS↑',  bg: '#f59f00', title: 'Caution: egress to all workloads in namespace'  },
  'ns-ingress-access': { symbol: 'NS↓',  bg: '#f59f00', title: 'Caution: ingress from all workloads in namespace' },
  'cross-namespace':   { symbol: '⇆',    bg: '#1864ab', title: 'Info: cross-namespace traffic allowed' },
  'air-gapped':        { symbol: '⊘',    bg: '#2f9e44', title: 'Secure: effectively isolated (deny-all)' },
};

interface BadgeNode { id: string; statuses: StatusKey[] }

function StatusBadge({ s }: { s: StatusKey }) {
  const { symbol, bg, title } = STATUS_CFG[s];
  return (
    <span
      title={title}
      style={{
        display: 'inline-block',
        background: bg,
        color: '#fff',
        fontSize: 7,
        fontWeight: 700,
        lineHeight: 1,
        padding: '2px 3px',
        borderRadius: 3,
        letterSpacing: '0.02em',
        cursor: 'default',
        userSelect: 'none',
        whiteSpace: 'nowrap',
      }}
    >
      {symbol}
    </span>
  );
}

// ── Legend dot ────────────────────────────────────────────────────────────────
function Dot({ color, label }: { color: string; label: string }) {
  return (
    <span className="d-flex align-items-center gap-1">
      <span style={{ width: 10, height: 10, borderRadius: '50%', background: color, display: 'inline-block', flexShrink: 0 }} />
      {label}
    </span>
  );
}

// ── Main component ────────────────────────────────────────────────────────────
export default function PolicyGraph() {
  const containerRef  = useRef<HTMLDivElement>(null);
  const cyRef         = useRef<cytoscape.Core | null>(null);
  const badgeLayerRef = useRef<HTMLDivElement>(null);
  const [badgeNodes, setBadgeNodes]   = useState<BadgeNode[]>([]);
  const [legendOpen, setLegendOpen]   = useState(true);
  const badgeDivRefs  = useRef<Map<string, HTMLDivElement>>(new Map());

  // Refs so layoutstop callback can read latest selection state without stale closures
  const selectedNodeRef      = useRef<WorkloadNode | null>(null);
  const selectedStatusesRef  = useRef<Set<StatusKey>>(new Set());

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
    selectedStatuses, toggleStatus,
    selectedNode, selectedEdges,
    setSelectedNode, setSelectedEdges,
    loadGraph, loadClusterState, loading, error,
    layoutAlgorithm,
  } = useGraphStore();

  // Apply dimming: node-click mode takes priority, then status filter mode
  const applyDimming = useCallback(() => {
    const cy = cyRef.current;
    if (!cy) return;
    cy.elements().removeClass('dimmed focused');
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
    selectedNodeRef.current     = selectedNode;
    selectedStatusesRef.current = selectedStatuses;
    applyDimming();
  }, [selectedNode, selectedStatuses, applyDimming]);

  // Re-apply dim state to badges when the badge list changes (after layout adds new badges)
  useEffect(() => { applyDimming(); }, [badgeNodes, applyDimming]);

  // Re-fit viewport when overlay panel opens/closes — canvas size unchanged so no resize needed
  const panelOpen = selectedNode !== null || selectedEdges.length > 0;
  const prevPanelOpen = useRef(panelOpen);
  useEffect(() => {
    if (panelOpen === prevPanelOpen.current) return;
    prevPanelOpen.current = panelOpen;
    const cy = cyRef.current;
    if (!cy) return;
    fitToArea(cy, panelOpen ? PANEL_W : 0);
    syncViewport();
  }, [panelOpen, syncViewport]);

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

    // Workload node click – toggle focus/dim (dimming handled reactively via applyDimming)
    cy.on('tap', 'node[ntype = "workload"]', (evt) => {
      const node      = evt.target as cytoscape.NodeSingular;
      const wasActive = node.hasClass('focused');
      setSelectedNode(wasActive ? null : node.data('workload') as WorkloadNode);
    });

    // Edge click – show policy detail
    cy.on('tap', 'edge', (evt) => {
      setSelectedEdges((evt.target as cytoscape.EdgeSingular).data('policies') as PolicyEdge[]);
    });

    // Namespace box click – clear selection
    cy.on('tap', 'node[ntype = "namespace"]', () => {
      setSelectedNode(null);
      setSelectedEdges([]);
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
      setBadgeNodes(
        cy.nodes('[ntype = "workload"]').toArray()
          .filter((n) => ((n.data('workload') as WorkloadNode).statuses?.length ?? 0) > 0)
          .map((n) => ({ id: n.id(), statuses: (n.data('workload') as WorkloadNode).statuses! }))
      );
      syncViewport();
      applyDimming();
      // syncBadgePositions runs via useEffect after React commits the new badge divs
    });

    layout.run();
  // filteredNodes/filteredEdges call get() internally; the listed deps cover all state that affects output
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allNodes, allEdges, selectedNamespaces, selectedNodeTypes, searchQuery, showNamespaceEdges, showConnectedNamespaces, aggregateByNamespace, layoutAlgorithm, applyDimming]);

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

      {/* Legend — toggle button stays fixed; panel slides in/out beside it */}
      <div
        className="position-absolute d-flex align-items-end"
        style={{ bottom: 12, left: 12, zIndex: 10, gap: 6 }}
      >
        <button
          className="btn btn-sm btn-outline-secondary flex-shrink-0"
          onClick={() => setLegendOpen((o) => !o)}
          title={legendOpen ? 'Hide legend' : 'Show legend'}
          style={{ alignSelf: 'flex-end' }}
        >
          {legendOpen ? '‹' : '›'}
        </button>

        {/* Clip wrapper — overflow:hidden lets transform slide look clean */}
        <div style={{ overflow: 'hidden', maxHeight: 'calc(100% - 24px)' }}>
          <div
            className="px-3 py-2 rounded d-flex flex-column gap-1"
            style={{
              background: '#1a1e22ee', fontSize: 10, color: '#adb5bd', lineHeight: 1.6,
              maxHeight: 'calc(100vh - 80px)', overflowY: 'auto', boxSizing: 'border-box',
              transform: legendOpen ? 'translateX(0)' : 'translateX(calc(-100% - 6px))',
              transition: 'transform 0.25s ease',
            }}
          >
            <div className="fw-semibold mb-1" style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6c757d' }}>
              Policy coverage
            </div>
            <Dot color={COV.workload}  label="workload policies applied" />
            <Dot color={COV.namespace} label="namespace-selector only" />
            <Dot color={COV.none}      label="no policies (gap)" />
            <div className="mt-1 pt-1 border-top border-secondary" style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6c757d' }}>
              Arrows
            </div>
            <Dot color="#4dabf7" label="egress" />
            <Dot color="#f783ac" label="ingress" />
            <Dot color="#a9e34b" label="both" />
            <div className="d-flex gap-2 mt-1">
              <span style={{ color: '#adb5bd' }}>── workload</span>
              <span style={{ color: '#adb5bd' }}>╌╌ namespace</span>
            </div>
            <div className="mt-1 pt-1 border-top border-secondary" style={{ fontSize: 9, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6c757d' }}>
              Badges
            </div>
            {(Object.entries(STATUS_CFG) as [StatusKey, typeof STATUS_CFG[StatusKey]][]).map(([key, cfg]) => {
              const active = selectedStatuses.has(key);
              const dimmed = selectedStatuses.size > 0 && !active;
              return (
                <span
                  key={key}
                  className="d-flex align-items-center gap-1"
                  title={cfg.title}
                  onClick={() => toggleStatus(key)}
                  style={{ cursor: 'pointer', opacity: dimmed ? 0.4 : 1, transition: 'opacity 0.15s' }}
                >
                  <span style={{
                    background: cfg.bg, color: '#fff',
                    fontSize: 7, fontWeight: 700, padding: '2px 3px', borderRadius: 3, flexShrink: 0,
                    outline: active ? '1.5px solid #fff' : 'none',
                    outlineOffset: 1,
                  }}>
                    {cfg.symbol}
                  </span>
                  <span style={{ fontSize: 9 }}>{cfg.title}</span>
                </span>
              );
            })}
          </div>
        </div>
      </div>

      <DetailPanel />
    </div>
  );
}
