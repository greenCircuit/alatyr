import { useCallback, useEffect, useRef, useState } from 'react';
import cytoscape from 'cytoscape';
// @ts-ignore
import dagre from 'cytoscape-dagre';
import { useGraphStore } from '../store/graphStore';
import { namespaces, nodes as allNodes } from '../data/policies';
import type { WorkloadNode, PolicyEdge, StatusKey } from '../data/policies';
import DetailPanel from './DetailPanel';

cytoscape.use(dagre);

// ── Coverage colors (encode policy coverage, not namespace identity) ──────────
const COV = {
  workload:  '#20c997',  // green  – workload-level policies present
  namespace: '#ffc107',  // amber  – namespace-selector policies only
  none:      '#dc3545',  // red    – no policies (gap)
} as const;
type CovType = keyof typeof COV;

function computeCoverage(policyEdges: PolicyEdge[]): {
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

  const ns: Record<string, CovType> = {};
  for (const n of namespaces) {
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
): cytoscape.ElementDefinition[] {
  const coverage   = computeCoverage(policyEdges);
  const bundles    = bundleEdges(policyEdges);
  const els: cytoscape.ElementDefinition[] = [];

  // Only add a namespace box if it has at least one visible workload
  const occupiedNS = new Set(workloads.filter((w) => w.namespace).map((w) => w.namespace));

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
          ? (b.policies[0].ports?.map((p) => String(p.port)).join(', ') ?? 'all ports')
          : `${count} policies`,
        policies:  b.policies,
        hasNS:     b.hasNS,
        direction: b.direction,
      },
    });
  }

  return els;
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
      'width':            90,
      'height':           38,
      'shape':            'roundrectangle',
      'text-margin-y':    0,
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
  'internet-ingress':   { symbol: 'WAN↓', bg: '#1864ab', title: 'Reachable from internet'                         },
  'internet-egress':    { symbol: 'WAN↑', bg: '#c92a2a', title: 'Can reach internet (exfil risk)'                 },
  'no-policy':          { symbol: '!',    bg: '#e67700', title: 'No NetworkPolicy → implicit allow-all'           },
  'isolated':           { symbol: '⊘',    bg: '#495057', title: 'Effectively isolated (deny-all)'                 },
  'orphaned-selector':  { symbol: '⚠',    bg: '#862e9c', title: 'Orphaned policy: selector matches no pods'      },
  'cross-namespace':    { symbol: '⇆',    bg: '#2f9e44', title: 'Cross-namespace traffic allowed'                 },
  'dns-missing':        { symbol: 'DNS!', bg: '#a61e4d', title: 'No UDP/53 egress rule — DNS may break'           },
  'kube-api-access':    { symbol: 'k8s',  bg: '#0c8599', title: 'Has egress rule to Kubernetes API server'        },
  'ingress-exposed':    { symbol: 'ING',  bg: '#5f3dc4', title: 'Exposed via Ingress controller'                  },
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
  // Badge overlay: transform layer mirrors Cytoscape viewport (pan/zoom via CSS only)
  const badgeLayerRef = useRef<HTMLDivElement>(null);
  // Badge nodes: statuses only — positions are synced via direct DOM writes, not React state
  const [badgeNodes, setBadgeNodes]   = useState<BadgeNode[]>([]);
  const badgeDivRefs  = useRef<Map<string, HTMLDivElement>>(new Map());

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
    filteredNodes, filteredEdges, selectedNamespaces,
    selectedNodeTypes, searchQuery, showNamespaceEdges,
    selectedStatuses, toggleStatus,
    setSelectedNode, setSelectedEdges,
  } = useGraphStore();

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

    // Workload node click – toggle focus/dim
    cy.on('tap', 'node[ntype = "workload"]', (evt) => {
      const node      = evt.target as cytoscape.NodeSingular;
      const wasActive = node.hasClass('focused');

      cy.elements().removeClass('dimmed focused');

      if (!wasActive) {
        const neighborhood = node.closedNeighborhood();          // node + edges + peers
        const parents      = neighborhood.nodes().parents();     // namespace boxes
        const toFocus      = neighborhood.union(parents);
        cy.elements().not(toFocus).addClass('dimmed');
        toFocus.addClass('focused');
        setSelectedNode(node.data('workload') as WorkloadNode);
      } else {
        setSelectedNode(null);
      }
    });

    // Edge click – show policy detail
    cy.on('tap', 'edge', (evt) => {
      setSelectedEdges((evt.target as cytoscape.EdgeSingular).data('policies') as PolicyEdge[]);
    });

    // Namespace box click – clear selection
    cy.on('tap', 'node[ntype = "namespace"]', () => {
      cy.elements().removeClass('dimmed focused');
      setSelectedNode(null);
      setSelectedEdges([]);
    });

    // Background click – clear everything
    cy.on('tap', (evt) => {
      if (evt.target === cy) {
        cy.elements().removeClass('dimmed focused');
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
    const elements    = buildElements(workloads, policyEdges, selectedNamespaces);

    cy.elements().remove();
    cy.add(elements);

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const layout = cy.layout({
      name:          'dagre',
      rankDir:       'LR',
      padding:       60,
      spacingFactor: 1.4,
      nodeSep:       50,
      rankSep:       110,
      animate:       false,
    } as any);

    layout.one('layoutstop', () => {
      setBadgeNodes(
        cy.nodes('[ntype = "workload"]').toArray()
          .filter((n) => ((n.data('workload') as WorkloadNode).statuses?.length ?? 0) > 0)
          .map((n) => ({ id: n.id(), statuses: (n.data('workload') as WorkloadNode).statuses! }))
      );
      syncViewport();
      // syncBadgePositions runs via useEffect after React commits the new badge divs
    });

    layout.run();
  // filteredNodes/filteredEdges call get() internally; the listed deps cover all state that affects output
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedNamespaces, selectedNodeTypes, searchQuery, showNamespaceEdges, selectedStatuses]);

  return (
    <div style={{ flex: 1, position: 'relative', background: '#0d0f11' }}>
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

      {/* Legend */}
      <div
        className="position-absolute px-3 py-2 rounded d-flex flex-column gap-1"
        style={{ bottom: 12, left: 12, background: '#1a1e22ee', fontSize: 10, color: '#adb5bd', zIndex: 10, lineHeight: 1.6 }}
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

      <DetailPanel />
    </div>
  );
}
