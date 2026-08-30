// Hardcoded fixtures for the DetailPanel V2 preview page. Shapes mirror the
// real WorkloadNode / PolicyEdge / ReachabilityResult so the redesign can
// swap in against the live data model without translation.

export type Severity = 'info' | 'caution' | 'warning' | 'high' | 'critical' | 'secure';

export const SEV_COLOR: Record<Severity, string> = {
  info: '#4dabf7',
  caution: '#fab005',
  warning: '#e8590c',
  high: '#c92a2a',
  critical: '#581c87',
  secure: '#2f9e44',
};

export interface StatusChip { key: string; symbol: string; severity: Severity; description: string; }

export const MOCK_STATUS: StatusChip[] = [
  { key: 'internet-ingress', symbol: 'WAN↓', severity: 'high',    description: 'reachable from internet (public-facing)' },
  { key: 'ns-full-access',   symbol: 'NS⇆',  severity: 'warning', description: 'full access to/from entire namespace' },
  { key: 'l7-applied',       symbol: 'L7',   severity: 'secure',  description: 'L7 matchers (hosts/methods/paths) gate this workload' },
];

// ── Node panel fixture ─────────────────────────────────────────────────
// State = what the rule *does* right now.
//   allows     — rule fires and permits the traffic
//   blocks     — rule fires and denies
//   inert      — rule exists but doesn't match (wrong side / eval order)
//   unenforced — policy selects workload but has no rules on this direction,
//                leaving it wide open (classic k8s NetworkPolicy footgun)
// unenforced ≠ inert — inert means the rule was written but doesn't fire;
// unenforced means no rule exists on that side at all, so nothing else applies.
export type PolicyState = 'allows' | 'blocks' | 'inert' | 'unenforced';

// Coverage = blanket-posture classification. Blanket rules render as a single
// PostureRow above per-peer PolicyCards so the whole-direction verdict is
// visible on one line instead of buried in the peer list.
//   restricted — normal rule with peers/ports/selectors
//   deny all   — catch-all deny for the whole direction
//   allow all  — catch-all allow (unusual, but backend emits it)
//   unenforced — same as PolicyState above, surfaces at posture level
export type PolicyCoverage = 'restricted' | 'deny all' | 'allow all' | 'unenforced';

export interface PolicySelector {
  labels?: Record<string, string>;   // matchLabels — how workload/peer got selected
  nsLabels?: Record<string, string>; // namespaceSelector matchLabels
  nsNames?: string[];                // explicit namespace names (Istio-style)
}

export interface PolicyPeer {
  label: string;
  namespace?: string;
  kind?: 'workload' | 'namespace' | 'external' | 'wildcard';
  selector?: PolicySelector;
  // Wildcard = k8s NetworkPolicy `from: []` or `podSelector: {} + namespaceSelector: {}`
  // — the most permissive rule; must render EXPLICITLY as "any peer everywhere"
  // instead of falling through the empty-peer-list gate as silence. Silent-lie fix.
  wildcard?: boolean;
}

export interface L7Match {
  hosts?: string[];
  methods?: string[];
  paths?: string[];
  notHosts?: string[];
  notMethods?: string[];
  notPaths?: string[];
}

export interface SelectingPolicy {
  source: string;
  name: string;
  namespace: string;
  action: 'allow' | 'deny';
  direction: 'ingress' | 'egress';
  state: PolicyState;
  coverage?: PolicyCoverage;           // blanket-posture classification
  // Rich fields mirroring backend NodePolicies / NeighborRef struct — every
  // one of these is already returned today; V2 preview should render them so
  // operator does not need to open YAML to answer "how did this policy select
  // my workload / what other workloads does it touch / on what ports".
  crossNs?: boolean;                   // policy lives in different ns from workload
  ports?: Array<{ port: number; protocol: string }>;
  anyPort?: boolean;                   // policy does not restrict ports
  nodeSelector?: PolicySelector;       // matchLabels that picked THIS workload
  peers?: PolicyPeer[];                // other endpoints this rule touches
  l7?: L7Match;                        // Istio hosts/methods/paths, if any
}

// Mesh info for a workload — SA identity + mTLS mode + revision drive every
// Istio AuthorizationPolicy match. First thing checked when Istio pod cannot
// reach the mesh. Populated per PolicySource that speaks mesh (istio today).
export type MtlsMode = 'STRICT' | 'PERMISSIVE' | 'DISABLE' | 'UNSET';

// PeerAuthentication ref — one PA that scoped the workload. `effective` marks
// the one whose scope resolution won (mesh < ns < workload; workload wins over
// ns wins over mesh). Backend-sre flag: hiding the PA chain hides "who owns the
// mode" which is 3am pager info on strict-mtls violations.
export interface PeerAuthRef {
  name: string;
  namespace: string;
  scope: 'mesh' | 'namespace' | 'workload';
  mode: MtlsMode;
  effective: boolean;
}

export interface MeshInfo {
  provider: string;                    // e.g. 'istio'
  serviceAccount: string;              // e.g. 'cluster.local/ns/frontend/sa/checkout-web'
  mtlsMode: MtlsMode;                  // effective mode (from the winning PA)
  revision?: string;                   // Istio revision label (default = 'default')
  sidecarInjected: boolean;
  // Enrolled = workload lives in a mesh-tracked ns AND has a sidecar. When
  // false, MeshBlock renders empty state ("not enrolled") instead of hiding —
  // hiding is indistinguishable from "mesh data not fetched" which is a
  // silent lie. Backend-sre flag.
  enrolled: boolean;
  principals?: string[];              // SPIFFE IDs authorized to reach this workload
  notPrincipals?: string[];           // SPIFFE IDs explicitly denied
  // Full PA chain feeding the effective mode. Rendered as a list w/ the
  // effective one marked. Missing chain = can't tell why a strict-ns workload
  // is actually permissive at runtime.
  peerAuthChain?: PeerAuthRef[];
  // Per-port mTLS overrides — an effective PA may still permit plaintext on
  // named ports (common for probes). Without these, a strict-ns w/ a permissive
  // port override is invisible in the UI.
  portOverrides?: Record<string, MtlsMode>;
  // Ambient waypoint — for waypoint-proxied workloads, names the waypoint that
  // enforces policy on their behalf. When set, AuthorizationPolicies bind to
  // the waypoint, not this workload; operator needs to know where to look.
  waypoint?: { name: string; namespace: string };
  // Non-fatal warnings on the mesh state (e.g. mode conflict between overlapping
  // PAs, missing sidecar w/ enrolled ns). Rendered as a warn list.
  mtlsIssues?: string[];
}

export const MOCK_NODE = {
  id: 'frontend/checkout-web',
  label: 'checkout-web',
  namespace: 'frontend',
  type: 'service' as const,
  labels: {
    app: 'checkout-web',
    tier: 'edge',
    'app.kubernetes.io/instance': 'checkout',
    'app.kubernetes.io/managed-by': 'helm',
    version: 'v2.14.1',
  },
  statuses: MOCK_STATUS,
  mesh: {
    provider: 'istio',
    serviceAccount: 'cluster.local/ns/frontend/sa/checkout-web',
    mtlsMode: 'STRICT',
    revision: 'default',
    sidecarInjected: true,
    enrolled: true,
    principals: [
      'cluster.local/ns/ingress-nginx/sa/nginx-ingress',
      'cluster.local/ns/monitoring/sa/prometheus',
    ],
    notPrincipals: [
      'cluster.local/ns/legacy/sa/legacy-cart',
    ],
    peerAuthChain: [
      { name: 'default',      namespace: 'istio-system', scope: 'mesh',      mode: 'PERMISSIVE', effective: false },
      { name: 'frontend-pa',  namespace: 'frontend',     scope: 'namespace', mode: 'STRICT',     effective: true  },
    ],
    portOverrides: {
      '15020': 'DISABLE',
    },
    mtlsIssues: [
      'Port 15020 (Istio agent probes) is DISABLE while ns default is STRICT — verify this is intentional.',
    ],
  } as MeshInfo,
  perEngine: [
    {
      name: 'k8s',
      statuses: [MOCK_STATUS[0], MOCK_STATUS[1]],
      inbound: 12,
      outbound: 3,
      hasDeny: false,
      selectingPolicies: [
        {
          source: 'k8s', name: 'default-egress-open', namespace: 'frontend',
          action: 'allow', direction: 'egress', state: 'unenforced',
          coverage: 'unenforced',
          anyPort: true,
          nodeSelector: { labels: { app: 'checkout-web' } },
          peers: [],
        },
        {
          source: 'k8s', name: 'allow-frontend-ingress', namespace: 'frontend',
          action: 'allow', direction: 'ingress', state: 'allows',
          coverage: 'restricted',
          ports: [{ port: 80, protocol: 'TCP' }, { port: 443, protocol: 'TCP' }],
          nodeSelector: { labels: { app: 'checkout-web' } },
          peers: [
            { label: 'ingress-nginx', namespace: 'ingress-nginx', kind: 'workload', selector: { labels: { app: 'nginx-ingress' } } },
            { label: 'monitoring',    namespace: 'monitoring',    kind: 'namespace', selector: { nsNames: ['monitoring'] } },
          ],
        },
        {
          // Wildcard-peer case: k8s NetworkPolicy `from: []` → any peer everywhere.
          // Renders as explicit "any peer" row so the most permissive rule is the
          // most visible, not silently the emptiest.
          source: 'k8s', name: 'allow-health-checks', namespace: 'frontend',
          action: 'allow', direction: 'ingress', state: 'allows',
          coverage: 'restricted',
          ports: [{ port: 8081, protocol: 'TCP' }],
          nodeSelector: { labels: { app: 'checkout-web' } },
          peers: [
            { label: 'any peer', kind: 'wildcard', wildcard: true },
          ],
        },
        {
          source: 'k8s', name: 'allow-checkout-egress', namespace: 'frontend',
          action: 'allow', direction: 'egress', state: 'allows',
          coverage: 'restricted',
          anyPort: true,
          nodeSelector: { labels: { app: 'checkout-web', tier: 'edge' } },
          peers: [
            { label: 'payments-api', namespace: 'backend', kind: 'workload', selector: { labels: { app: 'payments-api' } } },
            { label: 'redis-cache',  namespace: 'backend', kind: 'workload', selector: { labels: { app: 'redis' } } },
          ],
        },
      ] as SelectingPolicy[],
    },
    {
      name: 'istio',
      statuses: [MOCK_STATUS[2]],
      inbound: 4,
      outbound: 1,
      hasDeny: true,
      // Multiple denies stack in real clusters (root ns default-deny + workload deny +
      // tenant deny). Model as array so the panel names every culprit, not just one.
      denyPolicies: [
        { source: 'istio', name: 'root-default-deny', namespace: 'istio-system', action: 'deny' as const, direction: 'ingress' as 'ingress' | 'egress' },
        { source: 'istio', name: 'block-legacy-api',   namespace: 'frontend',      action: 'deny' as const, direction: 'ingress' as 'ingress' | 'egress' },
      ],
      selectingPolicies: [
        {
          source: 'istio', name: 'root-default-deny', namespace: 'istio-system',
          action: 'deny', direction: 'ingress', state: 'blocks', crossNs: true,
          coverage: 'deny all',
          anyPort: true,
          nodeSelector: {},
          peers: [],
        },
        {
          source: 'istio', name: 'block-legacy-api', namespace: 'frontend',
          action: 'deny', direction: 'ingress', state: 'blocks',
          coverage: 'restricted',
          anyPort: true,
          nodeSelector: { labels: { app: 'checkout-web' } },
          peers: [
            { label: 'legacy-cart', namespace: 'legacy', kind: 'workload', selector: { labels: { app: 'legacy-cart' } } },
          ],
        },
        {
          source: 'istio', name: 'allow-mesh-callers', namespace: 'frontend',
          action: 'allow', direction: 'ingress', state: 'allows',
          coverage: 'restricted',
          ports: [{ port: 8080, protocol: 'TCP' }],
          nodeSelector: { labels: { app: 'checkout-web' } },
          peers: [
            { label: 'mesh SA cluster.local/ns/backend/sa/payments-api', kind: 'external' },
          ],
          l7: {
            hosts: ['api.checkout.svc'],
            methods: ['POST', 'PUT'],
            paths: ['/v2/checkout/*'],
            notPaths: ['/v2/checkout/admin/*'],
            notMethods: ['DELETE'],
          },
        },
      ] as SelectingPolicy[],
    },
  ],
  issues: [
    {
      severity: 'warning' as Severity,
      kind: 'cross-ns-dangling',
      text: 'ALLOW rule references deleted workload frontend/legacy-cart',
      culprit: { source: 'k8s', name: 'allow-legacy-cart-egress', namespace: 'frontend' },
    },
    {
      severity: 'caution' as Severity,
      kind: 'port-conflict',
      text: 'Selecting policies disagree on port 8080 — istio ALLOW vs k8s DENY',
      culprit: { source: 'istio', name: 'allow-mesh-callers', namespace: 'frontend' },
    },
  ],
};

// ── Edge panel fixture ─────────────────────────────────────────────────
export const MOCK_EDGE = {
  src: {
    id: 'frontend/checkout-web',
    label: 'checkout-web',
    namespace: 'frontend',
    type: 'service' as const,
    labels: { app: 'checkout-web', tier: 'edge' },
    listensOn: [{ port: 8080, protocol: 'TCP' }],
  },
  dst: {
    id: 'backend/payments-api',
    label: 'payments-api',
    namespace: 'backend',
    type: 'service' as const,
    labels: { app: 'payments-api', tier: 'core' },
    listensOn: [{ port: 9090, protocol: 'TCP' }, { port: 9091, protocol: 'TCP' }],
  },
  verdict: 'deny' as const,
  blockedBy: { source: 'istio', name: 'require-mtls-strict', namespace: 'backend' },
  reason: 'Istio AuthorizationPolicy denies unmatched principals',
  engines: [
    { name: 'k8s', status: 'allow' as const },
    { name: 'istio', status: 'deny' as const },
  ],
  policies: [
    {
      source: 'k8s',
      name: 'allow-checkout-payments',
      namespace: 'backend',
      direction: 'ingress',
      ports: [{ port: 8080, protocol: 'TCP' }],
      action: 'allow' as const,
      coverage: 'restricted',
    },
    {
      source: 'istio',
      name: 'require-mtls-strict',
      namespace: 'backend',
      direction: 'ingress',
      ports: [],
      action: 'deny' as const,
      coverage: 'restricted',
    },
    {
      source: 'k8s',
      name: 'default-egress-allow',
      namespace: 'frontend',
      direction: 'egress',
      ports: [{ port: 8080, protocol: 'TCP' }],
      action: 'allow' as const,
      coverage: 'restricted',
    },
  ],
  issues: [
    {
      severity: 'high' as Severity,
      kind: 'port-not-declared',
      text: 'Allow rule permits port 8080 but destination declares no containerPort for it',
      culprit: { source: 'k8s', name: 'allow-checkout-payments', namespace: 'backend' },
    },
  ],
};

// Mesh info attached to src + dst in Compare — enables the mesh-verdict
// block. Contains own SA + mTLS mode + principal fit against the other side.
// Without this, Compare hid the Istio auth question that MeshBlock exposes
// on Node — silent lie by omission.
const MESH_SRC: MeshInfo = {
  provider: 'istio',
  serviceAccount: 'cluster.local/ns/frontend/sa/checkout-web',
  mtlsMode: 'STRICT',
  revision: 'default',
  sidecarInjected: true,
  enrolled: true,
};
const MESH_DST: MeshInfo = {
  provider: 'istio',
  serviceAccount: 'cluster.local/ns/backend/sa/payments-api',
  mtlsMode: 'STRICT',
  revision: 'default',
  sidecarInjected: true,
  enrolled: true,
  principals: ['cluster.local/ns/checkout/sa/checkout-api'],
  notPrincipals: ['cluster.local/ns/frontend/sa/checkout-web'],
};

// ── Compare panel fixture ──────────────────────────────────────────────
export const MOCK_COMPARE = {
  src: { ...MOCK_EDGE.src, mesh: MESH_SRC as MeshInfo | undefined },
  dst: { ...MOCK_EDGE.dst, mesh: MESH_DST as MeshInfo | undefined },
  verdict: 'deny' as const,
  reason: 'Blocked on 2 axes (all must clear)',
  reachablePorts: [] as { port: number; protocol: string }[],
  // `reverseError: true` renders "⚠ reverse unavailable" — surfaces a failed
  // reverse fetch instead of quietly showing forward-only. Silent-lie surface.
  bidirectional: {
    forward: 'deny' as 'allow' | 'deny',
    reverse: 'deny' as 'allow' | 'deny' | undefined,
    label: '✗ blocked both ways',
    reverseError: false,
  },
  blockers: [
    {
      engine: 'istio',
      side: 'DST' as const,
      direction: 'ingress',
      reason: 'explicit-deny',
      culprit: { name: 'require-mtls-strict', namespace: 'backend' },
      // paSource surfaces the PeerAuthentication forcing an mTLS block. Without
      // this the operator has no path from a mesh-mTLS blocker to the PA that
      // caused it. Mesh only.
      paSource: undefined as { name: string; namespace: string } | undefined,
      deleteToAllow: [
        { rule: 'from: notPrincipals: [cluster.local/ns/frontend/sa/checkout-web]', policy: 'require-mtls-strict' },
      ],
    },
    {
      engine: 'mesh',
      side: 'DST' as const,
      direction: 'mesh',
      reason: 'mtls-strict-plaintext-caller',
      culprit: { name: 'strict-mtls', namespace: 'backend' },
      paSource: { name: 'strict-mtls', namespace: 'backend' } as { name: string; namespace: string } | undefined,
      deleteToAllow: [],
    },
    {
      engine: 'k8s',
      side: 'SRC' as const,
      direction: 'egress',
      reason: 'locked-no-match',
      culprit: { name: 'default-deny-egress', namespace: 'frontend' },
      paSource: undefined as { name: string; namespace: string } | undefined,
      deleteToAllow: [],
    },
  ],
  engines: [
    {
      name: 'k8s',
      srcSelecting: [
        {
          source: 'k8s', name: 'default-deny-egress', namespace: 'frontend',
          action: 'deny' as const, state: 'blocks' as const,
          direction: 'egress' as 'ingress' | 'egress',
          coverage: 'deny all' as PolicyCoverage,
          anyPort: true,
          verdictDetail: 'catch-all egress deny — no allow rule matches this DST',
        },
      ],
      dstSelecting: [
        {
          source: 'k8s', name: 'allow-checkout-payments', namespace: 'backend',
          action: 'allow' as const, state: 'allows' as const,
          direction: 'ingress' as 'ingress' | 'egress',
          coverage: 'restricted' as PolicyCoverage,
          ports: [{ port: 8080, protocol: 'TCP' }],
          verdictDetail: 'ingress allow on 8080/TCP — DST accepts, but SRC-side deny wins',
        },
      ],
      verdict: 'deny' as 'allow' | 'deny',
      verdictReason: 'egress default-deny selects src, no matching allow',
      // Per-direction breakdown — traffic evaluated separately on ingress
      // (DST side) and egress (SRC side). Blockers can hit only one direction.
      // Rendering both lets operator see which axis flipped the verdict.
      ingressVerdict: { verdict: 'allow' as 'allow' | 'deny', reason: 'DST accepts on 8080/TCP via allow-checkout-payments' },
      egressVerdict:  { verdict: 'deny'  as 'allow' | 'deny', reason: 'SRC default-deny-egress catches all — no matching allow to this DST' },
      srcUnavailable: undefined as { reason: string } | undefined,
      dstUnavailable: undefined as { reason: string } | undefined,
      // Per-engine port sides — what SRC's egress opens vs what DST's ingress
      // accepts. Answers "port mismatch or full access?" without unfolding
      // rule cards. Live PortsSummary (ReachabilityView.tsx:165-187).
      srcEgressPorts:  { blocked: true,  allPorts: false, ports: [] as Array<{ port: number; protocol: string }> },
      dstIngressPorts: { blocked: false, allPorts: false, ports: [{ port: 8080, protocol: 'TCP' }] },
      // K8s engine has no mesh concept — mesh block omitted for this row.
      mesh: undefined as { src: MeshInfo; dst: MeshInfo; verdict: 'allow' | 'deny' | 'warn'; reason: string } | undefined,
    },
    {
      name: 'istio',
      srcSelecting: [] as Array<{
        source: string; name: string; namespace: string;
        action: 'allow' | 'deny'; state: PolicyState;
        direction: 'ingress' | 'egress'; coverage?: PolicyCoverage;
        anyPort?: boolean; ports?: Array<{ port: number; protocol: string }>;
        verdictDetail?: string;
      }>,
      // Src side unreadable — simulates RBAC blocking us from listing
      // AuthorizationPolicies in the source workload's namespace. Empty
      // `srcSelecting: []` would silently claim "no policies" which is
      // indistinguishable from "engine failed to load". Distinguish now.
      srcUnavailable: { reason: 'RBAC denied listing AuthorizationPolicies in namespace `frontend`. Grant read on authorization.istio.io/authorizationpolicies to see src-side selectors.' } as { reason: string } | undefined,
      dstSelecting: [
        {
          source: 'istio', name: 'require-mtls-strict', namespace: 'backend',
          action: 'deny' as const, state: 'blocks' as const,
          direction: 'ingress' as 'ingress' | 'egress',
          coverage: 'restricted' as PolicyCoverage,
          anyPort: true,
          verdictDetail: 'DENY on notPrincipals — SRC SA cluster.local/ns/frontend/sa/checkout-web on deny list',
        },
        {
          source: 'istio', name: 'allow-payments-api', namespace: 'backend',
          action: 'allow' as const, state: 'inert' as const,
          direction: 'ingress' as 'ingress' | 'egress',
          coverage: 'restricted' as PolicyCoverage,
          ports: [{ port: 9090, protocol: 'TCP' }],
          verdictDetail: 'ALLOW exists but does not fire — evaluated after DENY match',
        },
      ],
      verdict: 'deny' as 'allow' | 'deny',
      verdictReason: 'explicit deny on notPrincipals',
      // Istio has no egress AuthorizationPolicy concept (Istio egress = Sidecar
      // resource, out of scope for reachability today). Ingress verdict carries
      // the whole answer; egress row explicitly marked N/A so operator does
      // not read the absent row as "no problem there".
      ingressVerdict: { verdict: 'deny' as 'allow' | 'deny', reason: 'DENY on notPrincipals fires — SRC SA on deny list; ALLOW inert (eval order)' },
      egressVerdict:  { verdict: 'allow' as 'allow' | 'deny', reason: 'Istio AuthorizationPolicy has no egress semantics — treated as pass-through' },
      // SRC side unreadable ⇒ egress ports unknown, not empty. Distinguish
      // "we couldn't list" from "no ports allowed" — silent-lie surface.
      srcEgressPorts:  { blocked: false, allPorts: true,  ports: [] as Array<{ port: number; protocol: string }> },
      dstIngressPorts: { blocked: false, allPorts: false, ports: [{ port: 9090, protocol: 'TCP' }] },
      // Mesh block scoped to istio row — SA + mTLS + principals per side +
      // computed axis verdict. K8s row has mesh: undefined so this section
      // renders only where meaningful.
      mesh: {
        src: MESH_SRC,
        dst: MESH_DST,
        verdict: 'deny' as 'allow' | 'deny' | 'warn',
        reason: 'SRC SA on notPrincipals — mesh denies before AuthorizationPolicy rules evaluate',
      },
      dstUnavailable: undefined as { reason: string } | undefined,
    },
  ],
};
