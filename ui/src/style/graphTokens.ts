// Graph-layer color tokens. Cytoscape's stylesheet is JS, not CSS — it does
// not resolve `var(--foo)`. Mirror the tokens defined in `colors.css` here
// so graph edge/node styles can consume the same vocabulary. When updating
// a hex, change BOTH files.

export const GRAPH_TOKENS = {
  // Edge direction
  egress:      '#4dabf7', // --color-info (also reused for direction=egress)
  ingress:     '#f783ac',
  bidir:       '#a9e34b',

  // Verdict
  deny:        '#e03131',
  denyInk:     '#ff8787',

  // Except carve-out
  except:      '#fab005', // --color-except
  exceptInk:   '#ffe08a', // --color-except-ink

  // CIDR peer node
  cidr:        '#7aa2c8', // --color-cidr — slate-cyan, off egress-blue lane
  cidrInk:     '#cde4ff', // --color-cidr-ink
  cidrFill:    '#0f1a24', // dark node fill (unchanged)
} as const;
