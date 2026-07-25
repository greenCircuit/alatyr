// Label vocabularies + layout option list shared across FilterPanel dropdowns.
// Kept here so dropdown bodies stay focused on form structure.

import type { StatusKey, IssueType, Severity } from '../../../data/policies';
import type { MeshFilterValue } from '../../../store/filters';

// Single source of truth for issue-type → severity. Drives badge/accent color
// and danger-first sort across every issue surface (table, popover, rollup,
// drawer). Colors are always the same per type, so this lives in one place.
export const TYPE_SEVERITY: Record<IssueType, Severity> = {
  'policy conflict':        'high',
  'partial access':         'info',
  'mesh conflict':          'high',
  'mesh transport blocked': 'high',
  'node lockout':           'high',
  'mesh policy':            'info',
  'no dns':                 'warning',
};

// Three-tier fold of the six-value severity scale — the triage granularity
// issue chips and graph markers count at: Blocking (fires), Warning, Info
// (expected behavior, e.g. layering).
export type IssueTier = 'blocking' | 'warning' | 'info';

export const SEVERITY_TIER: Record<Severity, IssueTier> = {
  critical: 'blocking', high: 'blocking',
  warning:  'warning',  caution: 'warning',
  info:     'info',     secure:  'info',
};

export const issueTier = (type: IssueType): IssueTier => SEVERITY_TIER[TYPE_SEVERITY[type]];

export const ISSUE_TYPE_LABEL: Record<IssueType, string> = {
  'no dns':                 'No DNS egress',
  'mesh policy':            'Mesh policy hygiene',
  'mesh transport blocked': 'Mesh transport blocked',
  'policy conflict':        'Policy conflict',
  'partial access':         'Partial access',
  'mesh conflict':          'Mesh conflict',
  'node lockout':           'Node lockout',
};

export const ALL_ISSUE_TYPES: IssueType[] = [
  'policy conflict',
  'mesh conflict',
  'mesh transport blocked',
  'node lockout',
  'mesh policy',
  'no dns',
  'partial access',
];

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

// Togglable node types the backend actually emits. Excludes `namespace` —
// compound container, not filterable. Service/headless/external are defined
// in models/node.go but never produced yet.
export const ALL_NODE_TYPES = ['deployment', 'cronjob', 'cidr'] as const;

export const NODE_TYPE_LABEL: Record<string, string> = {
  deployment: 'Deployment',
  cronjob:    'CronJob',
  cidr:       'CIDR',
};

export const DIRECTION_LABEL: Record<string, string> = { ingress: 'Ingress', egress: 'Egress' };

export const MESH_FILTER_LABEL: Record<MeshFilterValue, string> = {
  'in-mesh':         'In mesh',
  'out-of-mesh':     'Out of mesh',
  'mtls-strict':     'mTLS STRICT',
  'mtls-permissive': 'mTLS PERMISSIVE',
  'mtls-disable':    'mTLS DISABLE',
  'mtls-unset':      'mTLS UNSET (default)',
};

// Two logical groups in the dropdown — membership on top, mtls verdicts below.
// Each group is a small OR-set; combined via OR across groups (node matches
// if any selected tag applies).
export const MESH_FILTER_GROUPS: { title: string; values: MeshFilterValue[] }[] = [
  { title: 'Membership', values: ['in-mesh', 'out-of-mesh'] },
  { title: 'mTLS mode',  values: ['mtls-strict', 'mtls-permissive', 'mtls-disable', 'mtls-unset'] },
];
