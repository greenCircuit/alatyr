// Edge style catalog for the policy graph. One entry per visual rule;
// PolicyGraph spreads these into the Cytoscape stylesheet at mount time.
//
// To change how an edge looks, edit the matching entry here — no need to
// touch PolicyGraph component code. Each entry pairs a Cytoscape selector
// (matches against edge data fields) with the style block applied when it
// hits. Selectors below are evaluated in order; later entries override.

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export interface EdgeStyle { selector: string; style: Record<string, any> }

export const EDGE_STYLES: EdgeStyle[] = [
  // Base: shared by every edge (overridden by more specific rules below).
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

  // Direction colors. Override line + arrow + label color together so the
  // edge reads as one semantic unit.
  {
    selector: 'edge[direction = "egress"]',
    style: {
      'line-color':         '#4dabf7', // blue — traffic going out from source
      'target-arrow-color': '#4dabf7',
      'color':              '#4dabf7',
    },
  },
  {
    selector: 'edge[direction = "ingress"]',
    style: {
      'line-color':         '#f783ac', // pink — traffic allowed into target
      'target-arrow-color': '#f783ac',
      'color':              '#f783ac',
    },
  },
  {
    selector: 'edge[direction = "both"]',
    style: {
      'line-color':         '#a9e34b', // lime — bidirectional
      'target-arrow-color': '#a9e34b',
      'source-arrow-color': '#a9e34b',
      'source-arrow-shape': 'triangle',
      'color':              '#a9e34b',
    },
  },

  // Namespace-level edges: dashed on top of direction color.
  {
    selector: 'edge[?hasNS]',
    style: {
      'line-style':        'dashed',
      'line-dash-pattern': [8, 4],
      'width':             2,
    },
  },

  // ns-to-child edges: distinguish from regular cross-ns edges with a dotted
  // style + outside-to-node anchor. Keep the default bezier + auto endpoints
  // — overriding both endpoints when source is the compound parent of target
  // causes Cytoscape to drop the edge.
  {
    selector: 'edge[?nsToChild]',
    style: {
      'line-style':         'dotted',
      'width':              2.5,
      'source-endpoint':    'outside-to-node',
    },
  },

  // Deny edges: red dashed line with a triangle-cross at the target end —
  // a triangle pointing at the target with a perpendicular bar in front of
  // it. Direction is still clear (triangle) AND the bar reads as "blocked."
  // Color + shape + dash pattern give three redundant cues so zoom-out and
  // color-blind users still see it as deny.
  {
    selector: 'edge[action = 1]',
    style: {
      'line-color':         '#e03131',
      'target-arrow-color': '#e03131',
      'target-arrow-shape': 'triangle-cross',
      'arrow-scale':        1.6,
      'color':              '#ff8787',
      'line-style':         'dashed',
      'width':              3,
    },
  },
];
