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
  | 'ns-full-access'      // both ingress and egress to/from entire namespace
  | 'l7-applied';         // workload is gated by L7 matchers (hosts/methods/paths)

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
  'l7-applied',
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
  'l7-applied':        { symbol: 'L7',   severity: 'secure',   description: 'L7 matchers (hosts/methods/paths) gate this workload' },
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

export interface PolicyRef {
  source:     string;
  name:       string;
  namespace:  string;
  ruleIndex:  number;
  action?:    'allow' | 'deny';    // set for selecting-policy refs, omitted for rule contributors
  direction?: string;    // "ingress" | "egress" | "both"
}

// Labels that caused a workload to be selected on one side of a rule.
// labelSelector = pod-selector matchLabels; nsSelector = namespaceSelector matchLabels.
// matchExpressions are not surfaced yet — backend drops them today.
export interface PolicySelector {
  labelSelector?: Record<string, string>;
  nsSelector?:    Record<string, string>;
  // Plain namespace names allowed on this side (Istio from.source.namespaces).
  // Distinct from nsSelector, which is label-based namespace matching.
  namespaces?:    string[];
}

// NodeRule mirrors backend models.NodeRule. Backend resolves both endpoint IDs
// into human labels + namespaces + kinds so the UI doesn't need to look them up
// against the graph node list. Node-info payloads only populate the dst side
// (clicked node is the source); reachability payloads carry both — ingress
// rules identify the peer by srcId.
export interface NodeRule {
  direction:    string;
  ports:        Port[];
  allPorts?:    boolean;     // true when rule grants all ports (no port restriction)
  allL7?:       boolean;     // true when rule grants any L7 (no host/method/path restriction)
  l7Match?:     L7Match;
  action:       number;      // 0 = Allow, 1 = Deny
  contributor?: PolicyRef;   // singular — one policy attribution per NodeRule
  srcId?:       string;
  srcLabel?:    string;      // empty for CIDR / unresolved IDs
  srcNamespace?: string;
  srcKind?:     string;      // WorkloadNode.type of the resolved src endpoint
  dstId:        string;
  dstLabel?:    string;      // empty for CIDR / unresolved IDs
  dstNamespace?: string;
  dstKind?:     string;      // WorkloadNode.type of the resolved dst endpoint
  srcSelector?: PolicySelector;
  dstSelector?: PolicySelector;
}

// Rule mirrors backend models.Rule — the raw engine-emitted rule carried inside
// a NeighborRef. Endpoints (srcId/dstId) are real node ids; the resolved peer
// travels alongside in NeighborRef.Workload.
// Coverage mirrors backend models.Coverage — the rule's blanket posture. Blank
// on a normal peer rule; 'restricted' = specific peers (default), the rest are
// engine-detected blanket states. 'deny all' / 'allow all' / 'unenforced' arrive
// with the opposite endpoint blank (no real peer); 'allow all ns' rides a real
// namespace peer.
export type Coverage =
  | 'deny all'
  | 'allow all'
  | 'allow all ns'
  | 'restricted'
  | 'unenforced'
  | 'audit';

export interface Rule {
  srcId:        string;
  dstId:        string;
  ports:        Port[];
  direction:    string;
  contributor?: PolicyRef;
  l7Match?:     L7Match;
  action:       number;      // 0 = Allow, 1 = Deny
  coverage?:    Coverage;    // blanket posture; blank/'restricted' = specific peers
  allPorts?:    boolean;
  allL7?:       boolean;
  srcSelector?: PolicySelector;
  dstSelector?: PolicySelector;
}

// NeighborRef mirrors store.NeighborRef: one policy rule touching the clicked
// node plus the resolved peer workload. Workload is blank (id === '') for
// unresolved ids (external CIDR) — fall back to the raw id from Rule.
// NOTE: backend NeighborRef/NodeNeighbors carry no json tags, so keys are
// PascalCase (Rule/Workload/In/Out) unlike the rest of the API.
export interface NeighborRef {
  Rule:     Rule;
  Workload: WorkloadNode;
}

// NodeNeighbors mirrors store.NodeNeighbors: peers per direction. In = clicked
// node is the destination; Out = clicked node is the source. No dedup — one
// entry per rule, so multiple rules on the same pair each appear.
export interface NodeNeighbors {
  In:  NeighborRef[] | null;
  Out: NeighborRef[] | null;
}

// Mesh types ------------------------------------------------------------

export type MeshScope = 'unset' | 'disable' | 'strict' | 'permissive';

export interface PARef {
  namespace: string;
  name:      string;
}

export interface MtlsSource {
  namespace:     string;
  name:          string;
  meshScope:  MeshScope;
  meshSource: 'global' | 'ns' | 'workload';
  portModes?:    Record<string, MeshScope>;
}

export interface MtlsState {
  verdict:          MeshScope;
  portOverrides?:   Record<string, MeshScope>;
  effectiveSource?: PARef; // absent or empty ns/name = default applied
  sources?:         MtlsSource[];
  issues?:          string[];
}

export interface MeshMembership {
  inMesh:   boolean;
  provider?: string;
  mode?:     string;
  waypoint?: { name: string; namespace: string };
  mtls?:     MtlsState;
}

// /api/node-info response. neighbors key = engine name (k8s/istio); Mesh key =
// mesh source name (currently only "istio"). Issues = cross-cutting interop
// findings (e.g. ambient pod missing ztunnel allowance).
export interface NodeDetail {
  neighbors: Record<string, NodeNeighbors>;
  mesh?:     Record<string, MeshMembership>;
  issues?:   string[];
}

// IssueType mirrors models.IssueType — the cross-cutting conflict classes
// surfaced by /api/issues.
export type IssueType =
  | 'no dns'
  | 'mesh policy'
  | 'policy conflict'
  | 'partial access'
  | 'mesh conflict'
  | 'node lockout';

// Issue mirrors models.Issue — one whole-cluster conflict finding. Edge-scoped
// issues (policy conflict) carry src+dst so the UI can open the reachability
// panel; node-scoped issues (lockout) carry node.
export interface Issue {
  type:    IssueType;
  message: string;
  // Culprit policies that broke the path, split by direction so the UI knows
  // which side to send the operator to: egress → edit src's egress policy,
  // ingress → edit dst's ingress policy. Both are plain PolicyRef[] (already
  // deduped by policy on the backend).
  ingressCulprits?: PolicyRef[];
  egressCulprits?:  PolicyRef[];
  // Policies that ALREADY permit this direction — the satisfied side of a
  // conflict. Lets the table confirm "ingress ✓ allowed by prometheus-ingress"
  // instead of hiding the covered side and making the operator hunt for it.
  ingressAllowed?: PolicyRef[];
  egressAllowed?:  PolicyRef[];
  // Per-direction block reason — drives the remediation verb (add rule / widen
  // selector / remove deny). Empty when that direction didn't block.
  ingressReason?: DirectionReason;
  egressReason?:  DirectionReason;
  engine?: string;
  // UI-derived (not from the API): engines that contributed to this finding.
  // Set by mergeIssuesByPair when folding a same-pair path blocked by more than
  // one engine (k8s egress + istio ingress) into one row.
  engines?: string[];
  src?:    WorkloadNode;
  dst?:    WorkloadNode;
  node?:   WorkloadNode;
}

// Flatten an issue's per-direction culprits into one deduped list, tagging each
// with the direction it came from so callers can label the fix side. Backend
// already dedups within a direction; this dedups across both (a policy that
// governs both ingress and egress appears once, direction 'both').
export function issueCulprits(issue: Issue): (PolicyRef & { direction: string })[] {
  const byKey = new Map<string, PolicyRef & { direction: string }>();
  const add = (ref: PolicyRef, direction: string) => {
    const key = `${ref.source}|${ref.namespace}|${ref.name}`;
    const existing = byKey.get(key);
    if (existing) {
      if (existing.direction !== direction) existing.direction = 'both';
      return;
    }
    byKey.set(key, { ...ref, direction });
  };
  for (const ref of issue.egressCulprits ?? [])  add(ref, 'egress');
  for (const ref of issue.ingressCulprits ?? []) add(ref, 'ingress');
  return [...byKey.values()];
}

const refKey = (ref: PolicyRef) => `${ref.source}|${ref.namespace}|${ref.name}`;
const nodeKey = (node?: WorkloadNode) => (node ? `${node.namespace ?? ''}/${node.label ?? node.id}` : '');
const isBlockReason = (reason?: DirectionReason): boolean =>
  reason === 'default-deny' || reason === 'locked-no-match' || reason === 'explicit-deny';

function dedupRefs(refs: PolicyRef[]): PolicyRef[] {
  const seen = new Set<string>();
  const out: PolicyRef[] = [];
  for (const ref of refs) {
    const key = refKey(ref);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(ref);
  }
  return out;
}

// Prefer a blocking reason when folding two directions — the permitted/no-opinion
// side of one evaluation shouldn't overwrite the block found by the other.
function mergeReason(left?: DirectionReason, right?: DirectionReason): DirectionReason | undefined {
  if (isBlockReason(left)) return left;
  if (isBlockReason(right)) return right;
  return left ?? right;
}

// Fold issues describing the same broken path on the same workload pair into one
// row. The backend emits a policy conflict per (engine, blocked direction) — a
// pair whose egress is locked by k8s while its ingress is locked by istio lands
// as two Issues, each single-direction, single-engine. To the operator that's
// ONE unreachable path, so we key by pair identity only (type + endpoint
// label/ns) — engine is intentionally NOT in the key — and union culprits,
// reasons, and contributing engines into one row carrying both fix groups.
// Node-scoped issues (no src/dst) never merge — passed through untouched.
export function mergeIssuesByPair(issues: Issue[]): Issue[] {
  const merged = new Map<string, Issue>();
  const passthrough: Issue[] = [];
  for (const issue of issues) {
    if (!(issue.src && issue.dst)) {
      passthrough.push(issue);
      continue;
    }
    const key = `${issue.type}|${nodeKey(issue.src)}|${nodeKey(issue.dst)}`;
    const existing = merged.get(key);
    if (!existing) {
      merged.set(key, { ...issue, engines: issue.engine ? [issue.engine] : [] });
      continue;
    }
    existing.egressCulprits  = dedupRefs([...(existing.egressCulprits ?? []),  ...(issue.egressCulprits ?? [])]);
    existing.ingressCulprits = dedupRefs([...(existing.ingressCulprits ?? []), ...(issue.ingressCulprits ?? [])]);
    existing.egressAllowed   = dedupRefs([...(existing.egressAllowed ?? []),   ...(issue.egressAllowed ?? [])]);
    existing.ingressAllowed  = dedupRefs([...(existing.ingressAllowed ?? []),  ...(issue.ingressAllowed ?? [])]);
    existing.egressReason  = mergeReason(existing.egressReason,  issue.egressReason);
    existing.ingressReason = mergeReason(existing.ingressReason, issue.ingressReason);
    if (issue.engine && !existing.engines!.includes(issue.engine)) existing.engines!.push(issue.engine);
  }
  return [...merged.values(), ...passthrough];
}

// Reachability verdict between two nodes, returned by /api/reachable.
// Mirrors store.ReachabilityResult on the backend.
export type DirectionReason =
  | 'permitted'
  | 'explicit-deny'
  | 'default-deny'
  | 'locked-no-match'
  | 'no-opinion';

export interface DirectionVerdict {
  denyAllMatches?:    NodeRule[];  // deny-all lock markers (default-deny)
  allowOtherMatches?: NodeRule[];  // near-miss: allowed elsewhere, not this peer
  allowMatches?:      NodeRule[];  // permits this peer
  denyMatches?:       NodeRule[];  // blocks this peer
  reason:             DirectionReason;
  culprits?:          PolicyRef[]; // deduped policies to edit
}

export interface EngineVerdict {
  status:       'allow' | 'deny' | 'not enforced';
  egress:       DirectionVerdict;
  ingress:      DirectionVerdict;
  srcPolicies?: PolicyRef[];
  dstPolicies?: PolicyRef[];
}

// MeshVerdict mirrors models.MeshVerdict (one per mesh source).
// effectiveSource names the PA that forced the verdict (when applicable).
export interface MeshVerdict {
  verdict:         'allow' | 'deny' | 'unknown';
  reason:          string;
  effectiveSource?: PARef;
}

export interface ReachabilityResult {
  verdict: 'allow' | 'deny';
  engines: Record<string, EngineVerdict>;
  mesh?:    Record<string, MeshVerdict>;
  srcMesh?: Record<string, MeshMembership>;
  dstMesh?: Record<string, MeshMembership>;
  path?:    string;
  reason:   string;
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
  // Blanket-posture flag from the backend rule. "allow all" = the rule places
  // no peer restriction; target is empty by design, not a resolution failure.
  coverage?: Coverage;
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

