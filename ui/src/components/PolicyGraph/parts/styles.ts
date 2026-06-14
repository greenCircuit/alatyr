// Cytoscape stylesheet for the graph. Node selectors come first so they take
// precedence on shared properties when EDGE_STYLES is spread in. Edge styles
// live in style/edgeStyles.ts so the visual language can be tuned without
// touching component code.

import { EDGE_STYLES } from '../../../style/edgeStyles';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const STYLE: any[] = [
  // Compound parent: visible box only, no label (label lives on the sibling
  // child below). Ungrabbed children prevent accidental drag inside the box;
  // the box itself remains grabbable so the user can drag the whole group.
  {
    selector: 'node[ntype = "namespace-box"]',
    style: {
      'background-color':   '#0a0c0e',
      'background-opacity': 0.65,
      'border-color':       'data(covColor)',
      'border-width':       2,
      'padding':            32,
      'shape':              'roundrectangle',
      'label':              '',
    },
  },
  // Label-only sibling child. Transparent so it floats over the box visual.
  {
    selector: 'node[ntype = "namespace"]',
    style: {
      'background-opacity': 0,
      'border-width':       0,
      'label':              'data(label)',
      'text-valign':        'center',
      'text-halign':        'center',
      'color':              'data(covColor)',
      'font-size':          11,
      'font-weight':        'bold',
      'text-transform':     'uppercase',
      'width':              'label',
      'height':             18,
      'shape':              'rectangle',
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
  ...EDGE_STYLES,
  {
    selector: '.dimmed',
    style: { 'opacity': 0.12 },
  },
  // Reachability source pin — blue, matches the SRC badge color in the panel.
  {
    selector: '.reach-src',
    style: {
      'border-color': '#0dcaf0',
      'border-width': 5,
      'background-color': '#0a3a4a',
    },
  },
  // Reachability target — amber, matches the DST badge color in the panel.
  {
    selector: '.reach-dst',
    style: {
      'border-color': '#ffc107',
      'border-width': 5,
      'background-color': '#3d3010',
    },
  },
];
