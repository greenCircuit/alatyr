// Status indicators computed by the backend; UI renders them as icon badges.
export type StatusKey =
  | 'internet-ingress'    // reachable from internet (public-facing)
  | 'internet-egress'     // can reach internet (exfil risk)
  | 'internet-full'       // both ingress from + egress to internet
  | 'lan-ingress'         // reachable from a private LAN outside the cluster
  | 'lan-egress'          // can reach a private LAN outside the cluster
  | 'lan-full'            // bidirectional LAN traffic
  | 'api-server-egress'   // egress to the kube-apiserver CIDR
  | 'air-gapped'          // effective deny-all (nothing can reach it)
  | 'cross-namespace'     // cross-namespace traffic explicitly allowed
  | 'ns-egress-access'    // egress allowed to all workloads in same namespace
  | 'ns-ingress-access'   // ingress allowed from all workloads in same namespace
  | 'ns-full-access';     // both ingress and egress to/from entire namespace

export const ALL_STATUS_KEYS: StatusKey[] = [
  'air-gapped',
  'cross-namespace',
  'internet-egress',
  'internet-ingress',
  'internet-full',
  'lan-egress',
  'lan-ingress',
  'lan-full',
  'api-server-egress',
  'ns-egress-access',
  'ns-ingress-access',
  'ns-full-access',
];

// Severity scale — color is owned here, not on individual status keys.
// Adding a level (e.g. 'notable') = add one entry + reference it from any
// status. UI just renders SEVERITY_COLOR[severity]; no derivation in code.
export type Severity = 'info' | 'caution' | 'warning' | 'high' | 'critical' | 'secure';

export const SEVERITY_COLOR: Record<Severity, string> = {
  info:     '#1864ab',  // blue
  caution:  '#fab005',  // bright yellow — pushed brighter to widen contrast vs warning orange
  warning:  '#e8590c',  // orange
  high:     '#c92a2a',  // red
  critical: '#581c87',  // purple
  secure:   '#2f9e44',  // green
};

// Single source of truth for status badge presentation. Used by:
//   - PolicyGraph (tiny on-node badges)
//   - DetailPanel (larger panel badges, with text label)
//   - Legend
// Severity drives color via SEVERITY_COLOR; description is the plain-text
// tooltip (no severity prefix — the color already communicates that).
export const STATUS_CFG: Record<StatusKey, { symbol: string; severity: Severity; description: string }> = {
  'internet-full':     { symbol: 'WAN⇆', severity: 'critical', description: 'bidirectional internet traffic' },
  'internet-egress':   { symbol: 'WAN↑', severity: 'high',     description: 'can reach internet (exfil risk)' },
  'internet-ingress':  { symbol: 'WAN↓', severity: 'high',     description: 'reachable from internet' },
  'ns-full-access':    { symbol: 'NS⇆',  severity: 'warning',  description: 'full access to/from entire namespace' },
  'ns-egress-access':  { symbol: 'NS↑',  severity: 'caution',  description: 'egress to all workloads in namespace' },
  'ns-ingress-access': { symbol: 'NS↓',  severity: 'caution',  description: 'ingress from all workloads in namespace' },
  'lan-full':          { symbol: 'LAN⇆', severity: 'warning',  description: 'bidirectional LAN traffic' },
  'lan-egress':        { symbol: 'LAN↑', severity: 'caution',  description: 'egress to private LAN' },
  'lan-ingress':       { symbol: 'LAN↓', severity: 'caution',  description: 'ingress from private LAN' },
  'cross-namespace':   { symbol: '⇆',    severity: 'info',     description: 'cross-namespace traffic allowed' },
  'api-server-egress': { symbol: 'API↑', severity: 'info',     description: 'egress to Kubernetes API server' },
  'air-gapped':        { symbol: '⊘',    severity: 'secure',   description: 'effectively isolated (deny-all)' },
};

export interface WorkloadNode {
  id: string;
  label: string;
  namespace: string;
  // service    = deployment + ClusterIP service (most workloads)
  // deployment = pod/deployment with no service exposure
  // headless   = headless service (direct pod addressing, no ClusterIP)
  // external   = traffic origin outside the cluster
  type: 'service' | 'deployment' | 'headless' | 'external' | 'cronjob' | 'namespace';
  labels: Record<string, string>;

  // effective (intersection) status keys — what the workload actually
  // experiences when every engine's constraints are AND'd together
  statuses?: StatusKey[];

  // per-engine breakdown, keyed by PolicySource.Name() (e.g. "k8s", "istio").
  // shown in the detail panel so operators see what each engine emits before
  // intersection.
  statusesBySource?: Record<string, StatusKey[]>;
}

export interface Port {
  port: number;       // 0 when the rule uses a named port
  endPort?: number;   // set when the rule specifies a range (port..endPort inclusive)
  name?: string;      // set for named ports (e.g. "http"); not resolved against pod specs
  protocol: string;
}

export interface L7Match {
  hosts?: string[];
  methods?: string[];
  paths?: string[];
  notHosts?: string[];
  notMethods?: string[];
  notPaths?: string[];
}

export interface PolicyEdge {
  id: string;
  source: string;       // workload node id OR 'ns-<name>' for namespace-level
  target: string;
  direction: 'ingress' | 'egress' | 'both';
  policyName: string;
  namespace: string;    // namespace where the NetworkPolicy lives
  level: 'workload' | 'namespace';
  ports?: Port[];
  policySource: string; // engine that produced this edge: "k8s", "istio", ...
  l7Matches?: L7Match[];
  action?: number;      // 0 = Allow, 1 = Deny
}

// Short edge-label summary of L7 matchers. Returns null when no L7 data so
// callers can decide whether to append a separator.
export function formatL7Summary(l7?: L7Match[]): string | null {
  if (!l7 || l7.length === 0) return null;
  const methods = new Set<string>();
  const paths = new Set<string>();
  const hosts = new Set<string>();
  for (const block of l7) {
    block.methods?.forEach((m) => methods.add(m));
    block.paths?.forEach((p) => paths.add(p));
    block.hosts?.forEach((h) => hosts.add(h));
  }
  const parts: string[] = [];
  if (methods.size) parts.push([...methods].join(','));
  if (paths.size) parts.push([...paths].join(','));
  if (hosts.size && !paths.size) parts.push([...hosts].join(','));
  return parts.length ? `L7: ${parts.join(' ')}` : 'L7';
}

export function formatPort(p: Port): string {
  if (p.name) return p.name;
  if (p.endPort) return `${p.port}-${p.endPort}`;
  return String(p.port);
}

// Backend emits a {port: 0, protocol: 'TCP'} sentinel when a NetworkPolicy
// has no ports: block — it's structurally needed at the rule layer but means
// "all ports allowed." Strip those before display so the UI doesn't show "0".
export function realPorts(ports?: Port[]): Port[] | undefined {
  if (!ports) return undefined;
  const filtered = ports.filter((p) => p.port !== 0 || p.name || p.endPort);
  return filtered.length > 0 ? filtered : undefined;
}

export const namespaces = ['frontend', 'backend', 'monitoring', 'database'];

export const nodes: WorkloadNode[] = [
  // frontend
  { id: 'web-app',      label: 'web-app',      namespace: 'frontend',   type: 'service',    labels: { app: 'web-app', tier: 'frontend' },   statuses: ['internet-ingress', 'cross-namespace'] },
  // backend
  { id: 'api-server',   label: 'api-server',   namespace: 'backend',    type: 'service',    labels: { app: 'api-server', tier: 'backend' },  statuses: ['cross-namespace', 'ns-egress-access'] },
  { id: 'auth-service', label: 'auth-service', namespace: 'backend',    type: 'service',    labels: { app: 'auth-service', tier: 'backend' }, statuses: ['cross-namespace'] },
  { id: 'cache',        label: 'redis-cache',  namespace: 'backend',    type: 'deployment', labels: { app: 'cache', tier: 'cache' },         statuses: [] },
  { id: 'batch-worker', label: 'batch-worker', namespace: 'backend',    type: 'deployment', labels: { app: 'batch-worker', tier: 'backend' },  statuses: [] },
  // database
  { id: 'postgres',     label: 'postgres',     namespace: 'database',   type: 'service',    labels: { app: 'postgres', tier: 'db' },         statuses: ['air-gapped', 'cross-namespace'] },
  // monitoring
  { id: 'prometheus',   label: 'prometheus',   namespace: 'monitoring', type: 'deployment', labels: { app: 'prometheus' },                   statuses: ['cross-namespace'] },
  { id: 'grafana',      label: 'grafana',      namespace: 'monitoring', type: 'deployment', labels: { app: 'grafana' },                      statuses: ['internet-ingress', 'ns-full-access'] },
  // external
  { id: 'internet',     label: 'Internet',     namespace: '',           type: 'external',   labels: {} },
];

