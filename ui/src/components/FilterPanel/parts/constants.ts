// Label vocabularies + layout option list shared across FilterPanel dropdowns.
// Kept here so dropdown bodies stay focused on form structure.

import type { StatusKey } from '../../../data/policies';

export const STATUS_LABELS: Record<StatusKey, string> = {
  'air-gapped':        'Air-gapped (deny-all)',
  'cross-namespace':   'Cross-namespace',
  'internet-egress':   'Internet egress',
  'internet-ingress':  'Internet ingress',
  'internet-full':     'Internet full (bi-directional)',
  'lan-egress':        'LAN egress',
  'lan-ingress':       'LAN ingress',
  'lan-full':          'LAN full (bi-directional)',
  'api-server-egress': 'API server egress',
  'ns-egress-access':  'NS egress access',
  'ns-ingress-access': 'NS ingress access',
  'ns-full-access':    'NS full access',
  'l7-applied':        'L7 applied',
};

export const LAYOUTS = [
  { value: 'dagre',        label: 'Dagre (hierarchical)' },
  { value: 'fcose',        label: 'fCoSE (compound)' },
  { value: 'cola',         label: 'Cola (force)' },
  { value: 'cose',         label: 'CoSE (force)' },
  { value: 'breadthfirst', label: 'Breadth-first' },
  { value: 'grid',         label: 'Grid' },
  { value: 'circle',       label: 'Circle' },
];

export const ACTION_LABEL: Record<number, string> = { 0: 'Allow', 1: 'Deny' };

export const DIRECTION_LABEL: Record<string, string> = { ingress: 'Ingress', egress: 'Egress' };
