// Static mock data for the design preview route. Zero store, zero API.
// Numbers picked to exercise all interesting states: KPI + severity + overflow.

import { SEVERITY_COLOR, type Severity } from '../../data/policies';

// Exposure vector — port + direction lets an operator triage before clicking.
// ingress = internet can reach it, egress = it can call out, both = worst.
export type ExposureDirection = 'ingress' | 'egress' | 'both';
export interface ExposedNodeLite {
  id:         string;
  label:      string;
  namespace:  string;
  port?:      string;
  direction?: ExposureDirection;
}

export const mockStats = {
  workloads:          247,
  policies:           38,
  namespacesSelected: 8,
  namespacesTotal:    12,
  issues:             17,
  issuesBlocking:     12,
  exposedCount:       6,
  exposedNamespaces:  2,
};

export const mockExposed: ExposedNodeLite[] = [
  { id: '1', label: 'checkout-api',      namespace: 'payments',      port: '443',      direction: 'both'    },
  { id: '2', label: 'auth-gateway',      namespace: 'platform',      port: '8443',     direction: 'ingress' },
  { id: '3', label: 'legacy-webhook',    namespace: 'ingest',        port: '80',       direction: 'ingress' },
  { id: '4', label: 'admin-console',     namespace: 'platform',      port: '3000',     direction: 'ingress' },
  { id: '5', label: 'metrics-collector', namespace: 'observability', port: '9090',     direction: 'egress'  },
  { id: '6', label: 'grafana',           namespace: 'observability', port: '3000',     direction: 'ingress' },
];

// Full coverage class list — every class stays visible even at zero so
// "we checked, it's zero" reads as posture signal (matches A's behavior).
export type CoverageClass = 'restricted' | 'deny all' | 'allow all ns' | 'allow all' | 'unenforced' | 'audit';

export const COVERAGE_CLASSES: CoverageClass[] = [
  'restricted', 'deny all', 'allow all ns', 'allow all', 'unenforced', 'audit',
];

export const COVERAGE_DESCRIPTION: Record<CoverageClass, string> = {
  'restricted':   'rules name specific peers',
  'deny all':     'default-deny lock — this policy allows nothing',
  'allow all ns': 'opens traffic to/from an entire namespace',
  'allow all':    'no peer restriction — open to everything',
  'unenforced':   'policy names the direction but adds no constraint',
  'audit':        'audit mode — observed, not enforced',
};

export const mockCoverage: Record<CoverageClass, number> & { total: number } = {
  'restricted':   47,
  'deny all':     12,
  'allow all ns': 6,
  'allow all':    3,
  'unenforced':   0,
  'audit':        0,
  total:          68,
};

// Defense-in-depth: workloads covered by only one of the enabled engines.
// Real page shows this under the engine rollup — B lost it, restore.
export const mockSingleEngineCount = 14;

export interface SeveritySegment { key: string; count: number; color: string; severity: Severity | 'none'; }

export const mockSeverity: SeveritySegment[] = [
  { key: 'critical',   count: 4,  color: SEVERITY_COLOR.critical, severity: 'critical' },
  { key: 'high',       count: 22, color: SEVERITY_COLOR.high,     severity: 'high' },
  { key: 'warning',    count: 41, color: SEVERITY_COLOR.warning,  severity: 'warning' },
  { key: 'caution',    count: 63, color: SEVERITY_COLOR.caution,  severity: 'caution' },
  { key: 'info',       count: 88, color: SEVERITY_COLOR.info,     severity: 'info' },
  { key: 'secure',     count: 21, color: SEVERITY_COLOR.secure,   severity: 'secure' },
  { key: 'no signals', count: 8,  color: '#495057',               severity: 'none' },
];

export interface RiskyRow {
  id:         string;
  workload:   string;
  namespace:  string;
  worstTier:  Severity;
  worstLabel: string;
  issueCount: number;
}

export const mockRisky: RiskyRow[] = [
  { id: 'r1', workload: 'checkout-api',   namespace: 'payments',      worstTier: 'critical', worstLabel: 'internet-full', issueCount: 5 },
  { id: 'r2', workload: 'auth-gateway',   namespace: 'platform',      worstTier: 'high',     worstLabel: 'internet-ingress', issueCount: 3 },
  { id: 'r3', workload: 'legacy-webhook', namespace: 'ingest',        worstTier: 'high',     worstLabel: 'internet-egress', issueCount: 2 },
  { id: 'r4', workload: 'redis-primary',  namespace: 'data',          worstTier: 'warning',  worstLabel: 'lan-full',     issueCount: 1 },
  { id: 'r5', workload: 'batch-runner',   namespace: 'ingest',        worstTier: 'warning',  worstLabel: 'ns-full-access', issueCount: 1 },
  { id: 'r6', workload: 'grafana',        namespace: 'observability', worstTier: 'caution',  worstLabel: 'ns-egress-access', issueCount: 0 },
];

export interface NamespaceRow {
  namespace:  string;
  workloads:  number;
  policies:   number;
  coverage:   number; // 0-100
  worst:      Severity | 'none';
  hasGap:     boolean;
  hasStale:   boolean;
}

export const mockNamespaces: NamespaceRow[] = [
  { namespace: 'payments',      workloads: 42, policies: 11, coverage: 88, worst: 'critical', hasGap: true,  hasStale: false },
  { namespace: 'platform',      workloads: 56, policies: 14, coverage: 71, worst: 'high',     hasGap: true,  hasStale: false },
  { namespace: 'ingest',        workloads: 33, policies: 5,  coverage: 42, worst: 'high',     hasGap: true,  hasStale: true  },
  { namespace: 'observability', workloads: 21, policies: 3,  coverage: 33, worst: 'caution',  hasGap: false, hasStale: false },
  { namespace: 'data',          workloads: 18, policies: 3,  coverage: 55, worst: 'warning',  hasGap: false, hasStale: false },
  { namespace: 'kube-system',   workloads: 27, policies: 2,  coverage: 12, worst: 'none',     hasGap: false, hasStale: false },
];
