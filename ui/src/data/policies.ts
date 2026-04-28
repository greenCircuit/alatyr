// Status indicators computed by the backend; UI renders them as icon badges.
export type StatusKey =
  | 'internet-ingress'    // reachable from internet (public-facing)
  | 'internet-egress'     // can reach internet (exfil risk)
  | 'no-policy'           // no NetworkPolicy → implicit allow-all
  | 'isolated'            // effective deny-all (nothing can reach it)
  | 'orphaned-selector'   // targeted by a policy whose podSelector matches nothing
  | 'cross-namespace'     // cross-namespace traffic explicitly allowed
  | 'dns-missing'         // no UDP/53 egress rule — DNS will likely break
  | 'kube-api-access'     // has egress rule allowing access to Kubernetes API server
  | 'ingress-exposed';   // reachable via an Ingress controller

export interface WorkloadNode {
  id: string;
  label: string;
  namespace: string;
  // service    = deployment + ClusterIP service (most workloads)
  // deployment = pod/deployment with no service exposure
  // headless   = headless service (direct pod addressing, no ClusterIP)
  // external   = traffic origin outside the cluster
  type: 'service' | 'deployment' | 'headless' | 'external';
  labels: Record<string, string>;
  statuses?: StatusKey[];
}

export interface PolicyEdge {
  id: string;
  source: string;       // workload node id OR 'ns-<name>' for namespace-level
  target: string;
  direction: 'ingress' | 'egress' | 'both';
  policyName: string;
  namespace: string;    // namespace where the NetworkPolicy lives
  level: 'workload' | 'namespace';
  ports?: { port: number; protocol: string }[];
}

export const namespaces = ['frontend', 'backend', 'monitoring', 'database'];

export const nodes: WorkloadNode[] = [
  // frontend
  { id: 'web-app',      label: 'web-app',      namespace: 'frontend',   type: 'service',    labels: { app: 'web-app', tier: 'frontend' },   statuses: ['internet-ingress', 'cross-namespace', 'ingress-exposed'] },
  // backend
  { id: 'api-server',   label: 'api-server',   namespace: 'backend',    type: 'service',    labels: { app: 'api-server', tier: 'backend' },  statuses: ['cross-namespace', 'dns-missing', 'kube-api-access'] },
  { id: 'auth-service', label: 'auth-service', namespace: 'backend',    type: 'service',    labels: { app: 'auth-service', tier: 'backend' }, statuses: ['cross-namespace'] },
  { id: 'cache',        label: 'redis-cache',  namespace: 'backend',    type: 'deployment', labels: { app: 'cache', tier: 'cache' },         statuses: ['no-policy'] },
  { id: 'batch-worker', label: 'batch-worker', namespace: 'backend',    type: 'deployment', labels: { app: 'batch-worker', tier: 'backend' },  statuses: ['no-policy'] },
  // database
  { id: 'postgres',     label: 'postgres',     namespace: 'database',   type: 'service',    labels: { app: 'postgres', tier: 'db' },         statuses: ['isolated', 'cross-namespace'] },
  // monitoring
  { id: 'prometheus',   label: 'prometheus',   namespace: 'monitoring', type: 'deployment', labels: { app: 'prometheus' },                   statuses: ['cross-namespace', 'dns-missing'] },
  { id: 'grafana',      label: 'grafana',      namespace: 'monitoring', type: 'deployment', labels: { app: 'grafana' },                      statuses: ['internet-ingress', 'orphaned-selector', 'ingress-exposed'] },
  // external
  { id: 'internet',     label: 'Internet',     namespace: '',           type: 'external',   labels: {} },
];

export const edges: PolicyEdge[] = [
  // ── workload-level edges ───────────────────────────────────────────────
  {
    id: 'e-internet-web', source: 'internet',    target: 'web-app',
    direction: 'ingress', policyName: 'allow-ingress-web', namespace: 'frontend', level: 'workload',
    ports: [{ port: 443, protocol: 'TCP' }, { port: 80, protocol: 'TCP' }],
  },
  {
    id: 'e-web-api', source: 'web-app',           target: 'api-server',
    direction: 'egress', policyName: 'allow-frontend-to-backend', namespace: 'frontend', level: 'workload',
    ports: [{ port: 8080, protocol: 'TCP' }],
  },
  {
    id: 'e-api-auth', source: 'api-server',       target: 'auth-service',
    direction: 'egress', policyName: 'allow-api-to-auth', namespace: 'backend', level: 'workload',
    ports: [{ port: 9000, protocol: 'TCP' }],
  },
  {
    id: 'e-api-cache', source: 'api-server',      target: 'cache',
    direction: 'both', policyName: 'allow-api-cache', namespace: 'backend', level: 'workload',
    ports: [{ port: 6379, protocol: 'TCP' }],
  },
  {
    id: 'e-api-postgres', source: 'api-server',   target: 'postgres',
    direction: 'egress', policyName: 'allow-backend-to-db', namespace: 'backend', level: 'workload',
    ports: [{ port: 5432, protocol: 'TCP' }],
  },
  {
    id: 'e-auth-postgres', source: 'auth-service', target: 'postgres',
    direction: 'egress', policyName: 'allow-backend-to-db', namespace: 'backend', level: 'workload',
    ports: [{ port: 5432, protocol: 'TCP' }],
  },
  // ── batch-worker: no-port policy + multi-policy bundle demo ──────────────
  {
    id: 'e-api-batch-trigger', source: 'api-server', target: 'batch-worker',
    direction: 'egress', policyName: 'allow-api-batch-trigger', namespace: 'backend', level: 'workload',
    // no ports – policy allows all ports
  },
  {
    id: 'e-api-batch-health', source: 'api-server', target: 'batch-worker',
    direction: 'egress', policyName: 'allow-api-batch-health', namespace: 'backend', level: 'workload',
    ports: [{ port: 8080, protocol: 'TCP' }],
  },

  {
    id: 'e-grafana-prom', source: 'grafana',      target: 'prometheus',
    direction: 'egress', policyName: 'allow-grafana-prometheus', namespace: 'monitoring', level: 'workload',
    ports: [{ port: 9090, protocol: 'TCP' }],
  },

  // ── namespace-level edges (namespaceSelector only) ─────────────────────
  {
    id: 'nse-mon-frontend', source: 'ns-monitoring', target: 'ns-frontend',
    direction: 'egress', policyName: 'allow-monitoring-scrape-ns', namespace: 'monitoring', level: 'namespace',
    ports: [{ port: 8080, protocol: 'TCP' }],
  },
  {
    id: 'nse-mon-backend', source: 'ns-monitoring', target: 'ns-backend',
    direction: 'egress', policyName: 'allow-monitoring-scrape-ns', namespace: 'monitoring', level: 'namespace',
    ports: [{ port: 8080, protocol: 'TCP' }],
  },
  {
    id: 'nse-backend-db', source: 'ns-backend', target: 'ns-database',
    direction: 'egress', policyName: 'allow-backend-ns-egress-db', namespace: 'backend', level: 'namespace',
    ports: [{ port: 5432, protocol: 'TCP' }],
  },
];
