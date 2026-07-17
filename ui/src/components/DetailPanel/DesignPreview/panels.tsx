// V2 detail panels — mock only, no store/API wiring. Renders the three
// panel shapes (Node / Edge / Compare) using the tier-1/tier-2 surface
// system defined in tokens.module.css. Data comes from mockData.ts.

import { useState } from 'react';
import t from './tokens.module.css';
import { MOCK_NODE, MOCK_EDGE, MOCK_COMPARE, SEV_COLOR, type StatusChip, type SelectingPolicy, type PolicyPeer, type PolicySelector, type L7Match, type MeshInfo } from './mockData';

// ── Shared primitives ──────────────────────────────────────────────────

// IconClose — SVG stroke close glyph. Replaces emoji `×` character which
// varies visually across fonts (some render as small multiplication sign,
// others as heavy close). Consistent 1.5-stroke weight matches other icons.
function IconClose() {
  return (
    <button type="button" className={t.iconButton} aria-label="Close">
      <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
        <path d="M2 2 L10 10 M10 2 L2 10" />
      </svg>
    </button>
  );
}

// Staleness stamp — reserved slot in frame header. Green dot = fresh
// (<2 min), caution = 2-15 min, deny = >15 min. Data cached from informer,
// so a lagged reflector can quietly render a wrong panel — expose the age
// so the operator can distrust the panel when the reflector is dead.
function StalenessStamp({ ageSec }: { ageSec: number }) {
  const dotClass = ageSec < 120 ? '' : ageSec < 900 ? t.stalenessDotStale : t.stalenessDotDead;
  const label = ageSec < 60 ? `${ageSec}s ago`
              : ageSec < 3600 ? `${Math.round(ageSec / 60)}m ago`
              : `${Math.round(ageSec / 3600)}h ago`;
  return (
    <span
      className={t.staleness}
      title={`Data fetched ${label}. Graph builds off cached informer state; if the reflector lagged, this panel can quietly render stale reachability. Refresh when in doubt.`}
    >
      <span className={`${t.stalenessDot} ${dotClass}`} />
      {label}
    </span>
  );
}

function StatusPill({ chip }: { chip: StatusChip }) {
  return (
    <span
      className={t.statusPill}
      title={`${chip.severity}: ${chip.description}`}
      style={{ background: SEV_COLOR[chip.severity] }}
    >
      <span className={t.statusPillSymbol}>{chip.symbol}</span>
      <span>{chip.key}</span>
    </span>
  );
}

// Action icon — small colored ✓ / ✗ square prepended to a policy name so
// operator can tell allow-rules from deny-rules without reading the meta.
// Color = allow/deny severity token (NOT engine hue) so severity legibility
// stays independent of provider.
// Kind chip — structured finding classification (cross-ns-dangling, port-conflict,
// port-not-declared, etc). Same scan-value shape as StatusPill so findings read
// as a taxonomy row instead of English prose.
function FindingKindChip({ kind }: { kind: string }) {
  return <span className={t.findingKind}>{kind}</span>;
}

function ActionIcon({ action }: { action: 'allow' | 'deny' }) {
  return (
    <span className={`${t.actionIcon} ${action === 'allow' ? t.actionAllow : t.actionDeny}`}>
      {action === 'allow' ? '✓' : '✗'}
    </span>
  );
}

// Compact policy reference — engine hue chip + `ns/name` mono + YAML button.
// Used inline in findings + blocking lines so the culprit is one glance away.
function PolicyRef({
  source, name, namespace, action, unresolved,
}: { source: string; name: string; namespace: string; action?: 'allow' | 'deny'; unresolved?: boolean }) {
  return (
    <span
      className={`${t.policyRef} ${unresolved ? t.policyRefUnresolved : ''}`}
      title={unresolved ? 'Culprit policy is referenced by a finding but does not appear in the selecting-policy list for this workload. May be deleted, out of RBAC scope, or in an unfetched namespace. YAML link may 404.' : undefined}
    >
      {action && <ActionIcon action={action} />}
      <EngineChip name={source} />
      <span className={t.mono}>{namespace}/{name}</span>
      {unresolved && <span title="Culprit not resolved">?</span>}
      <ManifestLink />
    </span>
  );
}

function EngineChip({ name }: { name: string }) {
  const variant = name === 'k8s' ? t.engineK8s
                : name === 'istio' ? t.engineIstio
                : name === 'mesh' ? t.engineMesh
                : '';
  return <span className={`${t.engineChip} ${variant}`}>{name}</span>;
}

function CountCapsule({ kind, n }: { kind: 'in' | 'out'; n: number }) {
  return (
    <span className={`${t.countCapsule} ${kind === 'in' ? t.countIn : t.countOut}`}>
      <span className={t.countLabel}>{kind}</span>
      {n}
    </span>
  );
}

function ManifestLink({ label = 'YAML' }: { label?: string }) {
  return <button className={t.manifestLink} type="button">{label}</button>;
}

function RolePill({ role }: { role: 'SRC' | 'DST' }) {
  return (
    <span className={`${t.rolePill} ${role === 'SRC' ? t.roleSrc : t.roleDst}`}>{role}</span>
  );
}

// Provenance/tooling labels operators don't select on. Filter these out of
// the primary label strip; keep behind disclosure.
const SYSTEM_LABEL_PREFIXES = [
  'app.kubernetes.io/',
  'helm.sh/',
  'meta.helm.sh/',
  'kubernetes.io/',
  'k8s.io/',
  'argocd.argoproj.io/',
];

function isSystemLabel(key: string): boolean {
  return SYSTEM_LABEL_PREFIXES.some((prefix) => key.startsWith(prefix));
}

function LabelChip({ k, v }: { k: string; v: string }) {
  // Click = copy `key=value` for pasting into `kubectl -l`. Zero-chrome
  // affordance — no button, chip itself is the click target.
  const copy = () => navigator.clipboard?.writeText(`${k}=${v}`);
  return (
    <button
      type="button"
      className={t.labelChip}
      onClick={copy}
      title={`Click to copy \`${k}=${v}\``}
    >
      <span className={t.labelKey}>{k}</span>
      <span className={t.labelEq}>=</span>
      <span className={t.labelValue}>{v}</span>
    </button>
  );
}

function LabelStrip({ labels }: { labels: Array<[string, string]> }) {
  if (labels.length === 0) return null;
  // Selector = full comma-joined `k1=v1,k2=v2` — pastes straight into
  // `kubectl -l`. Chip strip renders per-key click-to-copy above.
  const selector = labels.map(([key, value]) => `${key}=${value}`).join(',');
  const copyAll = () => navigator.clipboard?.writeText(selector);
  return (
    <div className={t.labelStrip} title="Click any chip to copy `key=value` · click `copy selector` to copy the full `k1=v1,k2=v2` selector for `kubectl -l`">
      {labels.map(([key, value]) => <LabelChip key={key} k={key} v={value} />)}
      <button
        type="button"
        className={t.labelCopyAll}
        onClick={copyAll}
        title={`Copy selector: ${selector}`}
      >
        copy selector
      </button>
    </div>
  );
}

// Port list — shared render used in policy cards + edge PolicyRow. Empty +
// unrestricted = "any port" (dim italic), else blue mono chip per port.
function PortList({ ports, anyPort }: { ports?: Array<{ port: number; protocol: string }>; anyPort?: boolean }) {
  if (anyPort || !ports || ports.length === 0) {
    return <span className={t.portAny} title="Policy does not restrict ports — all TCP/UDP allowed">any port</span>;
  }
  return (
    <>{ports.map((port, index) => (
      <span key={index} className={t.portChip}>{port.port}/{port.protocol}</span>
    ))}</>
  );
}

// Selector block — "matched by: {label chips}" or catch-all copy. Reuses
// LabelChip so the workload's own label strip and the policy's selector
// speak the same visual language (operator diffs by eye).
function SelectorBlock({ label, selector, catchAllCopy }: { label: string; selector?: PolicySelector; catchAllCopy: string }) {
  const podLabels = Object.entries(selector?.labels ?? {});
  const nsLabels  = Object.entries(selector?.nsLabels ?? {});
  const nsNames   = selector?.nsNames ?? [];
  const isCatchAll = podLabels.length === 0 && nsLabels.length === 0 && nsNames.length === 0;
  return (
    <div className={t.selectorBlock}>
      <div className={t.selectorLabel}>{label}</div>
      {isCatchAll ? (
        <div className={`${t.body} ${t.dim}`} style={{ fontStyle: 'italic' }}>{catchAllCopy}</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          {podLabels.length > 0 && (
            <div className={t.labelStrip}>
              <span className={`${t.body} ${t.dim}`} style={{ fontSize: 10 }}>pod:</span>
              {podLabels.map(([key, value]) => <LabelChip key={`p-${key}`} k={key} v={value} />)}
            </div>
          )}
          {nsLabels.length > 0 && (
            <div className={t.labelStrip}>
              <span className={`${t.body} ${t.dim}`} style={{ fontSize: 10 }}>ns:</span>
              {nsLabels.map(([key, value]) => <LabelChip key={`n-${key}`} k={key} v={value} />)}
            </div>
          )}
          {nsNames.length > 0 && (
            <div className={t.labelStrip}>
              <span className={`${t.body} ${t.dim}`} style={{ fontSize: 10 }}>from ns:</span>
              {nsNames.map((name) => (
                <span key={`nn-${name}`} className={t.labelChip}><span className={t.labelValue}>{name}</span></span>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// L7 match block — Istio hosts/methods/paths + their notX exclusions.
// Negated fields (notHosts/notMethods/notPaths) render in deny color so a
// silent Istio exclusion doesn't hide inside a permissive-looking rule.
// Only rendered when at least one dimension is populated.
function L7Row({ l7 }: { l7: L7Match }) {
  const rows: Array<[string, string[], boolean]> = [];
  if (l7.hosts?.length)      rows.push(['hosts',   l7.hosts,      false]);
  if (l7.methods?.length)    rows.push(['methods', l7.methods,    false]);
  if (l7.paths?.length)      rows.push(['paths',   l7.paths,      false]);
  if (l7.notHosts?.length)   rows.push(['hosts',   l7.notHosts,   true]);
  if (l7.notMethods?.length) rows.push(['methods', l7.notMethods, true]);
  if (l7.notPaths?.length)   rows.push(['paths',   l7.notPaths,   true]);
  if (rows.length === 0) return null;
  return (
    <div className={t.l7Block}>
      <div className={t.selectorLabel}>L7 match</div>
      {rows.map(([key, values, negate], rowIndex) => (
        <div key={`${key}-${negate ? 'not' : 'is'}-${rowIndex}`} className={t.l7Row}>
          <span className={t.l7Key}>{negate ? `not ${key}` : `${key}`}:</span>
          {values.map((value, index) => (
            <span key={index} className={negate ? t.l7ValueNeg : t.l7Value}>{value}</span>
          ))}
        </div>
      ))}
    </div>
  );
}

// Peer row — one endpoint a policy touches beyond the clicked workload.
// Arrow direction reflects the policy direction (egress → workload reaches
// peer; ingress ← peer reaches workload). Peer-side selector renders under
// so operator sees which labels the policy matched on that end.
function PeerRow({ peer, direction }: { peer: PolicyPeer; direction: 'ingress' | 'egress' }) {
  const arrowClass = direction === 'egress' ? t.peerArrow : t.peerArrowIn;
  const arrow = direction === 'egress' ? '→' : '←';
  // Wildcard peer = the most permissive rule form. Render with an explicit
  // warn-toned callout so operator can't miss it — the silent empty-peers
  // fallthrough was the bug we're fixing here.
  if (peer.wildcard) {
    return (
      <div className={t.peerRow}>
        <div className={t.peerHeadline}>
          <span className={arrowClass}>{arrow}</span>
          <span className={t.peerLabel} style={{ color: 'var(--warn)' }}>any peer everywhere</span>
          <span className={t.peerKind}>wildcard</span>
        </div>
        <div className={`${t.body} ${t.dim}`} style={{ paddingLeft: 14, fontStyle: 'italic' }}>
          k8s `from: []` or `podSelector: {} + namespaceSelector: {}` — matches every workload in every namespace.
        </div>
      </div>
    );
  }
  return (
    <div className={t.peerRow}>
      <div className={t.peerHeadline}>
        <span className={arrowClass}>{arrow}</span>
        <span className={t.peerLabel}>{peer.label}</span>
        {peer.namespace && <span className={t.peerNs}>/ {peer.namespace}</span>}
        {peer.kind && <span className={t.peerKind}>{peer.kind}</span>}
      </div>
      {peer.selector && (
        <div style={{ paddingLeft: 14 }}>
          <SelectorBlock label="matched by" selector={peer.selector} catchAllCopy="any peer" />
        </div>
      )}
    </div>
  );
}

// PolicyCard — one policy's full rendering in Node per-engine section.
// Header w/ ActionIcon + name + engine + cross-ns badge + YAML; state chip
// on the right. Body: direction + ports + node-selector (how this workload
// got matched) + peer list (collapsible over threshold) + optional L7.
// State tints the whole card so blocks/allows/inert reads at a glance.
const PEER_COLLAPSE_THRESHOLD = 4;
function PolicyCard({ policy, workloadNs }: { policy: SelectingPolicy; workloadNs: string }) {
  const stateClass = policy.state === 'blocks' ? t.policyCardBlocks
                   : policy.state === 'allows' ? t.policyCardAllows
                   : policy.state === 'unenforced' ? t.policyCardUnenforced
                   : t.policyCardInert;
  const peers = policy.peers ?? [];
  const [peersOpen, setPeersOpen] = useState(peers.length <= PEER_COLLAPSE_THRESHOLD);
  const catchAllCopy = `any workload in ${policy.namespace}`;
  return (
    <div className={`${t.policyCard} ${stateClass}`}>
      <div className={t.policyCardHeader}>
        <div className={t.policyMetaRow}>
          <ActionIcon action={policy.action} />
          <span className={t.section}>{policy.name}</span>
          <EngineChip name={policy.source} />
          <span className={`${t.body} ${t.dim}`} style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>
            {policy.namespace}
          </span>
          {policy.crossNs && (
            <span className={t.policyCrossNs} title={`Policy lives in ${policy.namespace}, not this workload's namespace (${workloadNs})`}>
              cross-ns
            </span>
          )}
        </div>
        <div className={t.policyMetaRow}>
          <span
            className={t.body}
            style={{
              color: policy.state === 'blocks' ? 'var(--deny)'
                   : policy.state === 'allows' ? 'var(--allow)'
                   : policy.state === 'unenforced' ? 'var(--warn)'
                   : '#7a848e',
              fontWeight: 700,
            }}
            title={
              policy.state === 'unenforced'
                ? 'Policy selects this workload but has no rules on this direction — leaves it wide open. Classic k8s NetworkPolicy footgun.'
                : policy.state === 'inert'
                ? 'Rule exists but does not match — wrong side or eval order'
                : undefined
            }
          >
            {policy.state}
          </span>
          <ManifestLink />
        </div>
      </div>
      <div className={t.policyMetaRow}>
        <span
          className={t.body}
          style={{ color: policy.direction === 'egress' ? 'var(--role-src)' : 'var(--role-dst)' }}
        >
          {policy.direction === 'egress' ? '↑' : '↓'} {policy.direction}
        </span>
        <span className={`${t.body} ${t.dim}`}>ports:</span>
        <PortList ports={policy.ports} anyPort={policy.anyPort} />
      </div>
      {policy.nodeSelector && (
        <SelectorBlock
          label="this workload matched by"
          selector={policy.nodeSelector}
          catchAllCopy={catchAllCopy}
        />
      )}
      {peers.length > 0 && (
        <div>
          <button
            type="button"
            onClick={() => setPeersOpen((open) => !open)}
            className={t.disclosureSummary}
            style={{ background: 'transparent', border: 0, color: '#7a848e', padding: 0, fontSize: 10, fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', cursor: 'pointer' }}
          >
            <span>{peersOpen ? '▾' : '▸'}</span>
            {policy.direction === 'egress' ? 'to' : 'from'} {peers.length} peer{peers.length > 1 ? 's' : ''}
          </button>
          {peersOpen && (
            <div style={{ marginTop: 4 }}>
              {peers.map((peer, index) => <PeerRow key={index} peer={peer} direction={policy.direction} />)}
            </div>
          )}
        </div>
      )}
      {policy.l7 ? (
        <L7Row l7={policy.l7} />
      ) : policy.source === 'istio' ? (
        // Istio policy without L7 = L4-only. Absence is ambiguous otherwise —
        // "L7 got dropped in the UI" reads identical to "no L7 constraint".
        // Make it explicit so operator can trust the panel.
        <div className={t.l4Only} title="Policy operates at L4 (TCP/port only). No host/method/path constraint is enforced.">
          <span>L4 only</span>
          <span className={t.dim}>· no host / method / path constraint</span>
        </div>
      ) : null}
    </div>
  );
}

// PostureRow — one-line whole-direction verdict for a blanket-posture policy
// (deny all / allow all / unenforced). Renders ABOVE the per-peer PolicyCard
// list so the operator sees the direction-wide answer first without reading
// through peer cards. Mirrors live `PostureRow` (`rows.tsx:512`).
const POSTURE_COVERAGES: Array<SelectingPolicy['coverage']> = ['deny all', 'allow all', 'unenforced'];
function isPosture(policy: SelectingPolicy): boolean {
  return !!policy.coverage && POSTURE_COVERAGES.includes(policy.coverage);
}
function PostureRow({ policy, workloadNs }: { policy: SelectingPolicy; workloadNs: string }) {
  const coverage = policy.coverage!;
  const shellClass = coverage === 'deny all' ? t.postureDenyAll
                   : coverage === 'unenforced' ? t.postureUnenforced
                   : t.postureAllowAll;
  const arrow = policy.direction === 'egress' ? '↑' : '↓';
  const arrowColor = policy.direction === 'egress' ? 'var(--role-src)' : 'var(--role-dst)';
  return (
    <div className={`${t.postureRow} ${shellClass}`} title={
      coverage === 'unenforced'
        ? 'No rules on this direction — nothing else applies'
        : coverage === 'deny all'
        ? 'Catch-all deny for this direction'
        : 'Catch-all allow for this direction'
    }>
      <span className={t.postureArrow} style={{ color: arrowColor }}>{arrow}</span>
      <span className={t.postureCoverage}>{coverage}</span>
      <span className={`${t.body} ${t.dim}`}>{policy.direction}</span>
      {/* Spacer + policy ref on the right. `flex: 1` spacer collapses cleanly
          when the row wraps, so the policy ref lands right below direction
          instead of getting margin-pushed off-alignment. */}
      <span style={{ flex: 1 }} />
      <PolicyRef
        source={policy.source}
        name={policy.name}
        namespace={policy.namespace}
        action={policy.action}
      />
      {policy.crossNs && (
        <span className={t.policyCrossNs} title={`Policy lives in ${policy.namespace}, not this workload's namespace (${workloadNs})`}>
          cross-ns
        </span>
      )}
    </div>
  );
}

// MeshBlock — SA identity + mTLS mode + revision + sidecar-injected. First
// thing checked when Istio pod isn't reaching the mesh. Live has this as
// `MeshCard`; V2 was dropping it entirely per backend-sre gap review.
function MeshBlock({ mesh }: { mesh: MeshInfo }) {
  // Not-enrolled empty state — mesh-tracked ns but workload has no sidecar.
  // Hiding this block instead of rendering "not enrolled" was the silent lie:
  // operator could not tell "not in mesh" from "mesh data not fetched".
  if (!mesh.enrolled) {
    return (
      <div className={t.tier1}>
        <div className={t.policyMetaRow}>
          <div className={t.eyebrow} style={{ flex: 1 }}>Mesh · {mesh.provider}</div>
          <span className={t.notEnrolled} title="Namespace is mesh-tracked but this workload has no sidecar. Istio AuthorizationPolicies cannot enforce on this workload — plaintext callers reach it directly.">
            not enrolled
          </span>
        </div>
        <div className={`${t.body} ${t.dim}`}>
          Not in mesh — no SA identity, no mTLS, no AuthorizationPolicy enforcement.
          Add sidecar injection (`istio-injection=enabled` label on ns or pod annotation) if this workload should participate.
        </div>
      </div>
    );
  }
  const mtlsColor = mesh.mtlsMode === 'STRICT' ? 'var(--allow)'
                  : mesh.mtlsMode === 'DISABLE' ? 'var(--deny)'
                  : mesh.mtlsMode === 'PERMISSIVE' ? 'var(--caution)'
                  : '#7a848e';
  return (
    <div className={t.tier1}>
      <div className={t.policyMetaRow}>
        <div className={t.eyebrow} style={{ flex: 1 }}>Mesh · {mesh.provider}</div>
        {!mesh.sidecarInjected && (
          <span className={t.policyCrossNs} title="No sidecar injected — this workload is not in the mesh">
            no sidecar
          </span>
        )}
      </div>
      <div className={t.selectorBlock}>
        <div className={t.selectorLabel}>Service account</div>
        <div className={`${t.body} ${t.mono}`} style={{ color: '#e6ecf2' }}>{mesh.serviceAccount}</div>
      </div>
      <div className={t.policyMetaRow}>
        <span className={t.selectorLabel}>mTLS</span>
        <span
          className={t.body}
          style={{ color: mtlsColor, fontWeight: 700 }}
          title={
            mesh.mtlsMode === 'STRICT'      ? 'STRICT — inbound plaintext rejected; caller must present valid mesh certificate. Safe default for zero-trust posture.'
          : mesh.mtlsMode === 'PERMISSIVE'  ? 'PERMISSIVE — accepts both mTLS and plaintext. Migration mode; leaves the door open to plaintext callers.'
          : mesh.mtlsMode === 'DISABLE'    ? 'DISABLE — mTLS off entirely; all traffic plaintext. AuthorizationPolicies matching principals will fail because there is no verified identity.'
          : 'UNSET — no PeerAuthentication applies; namespace/mesh default takes effect. Verify parent to know the actual mode.'
          }
        >{mesh.mtlsMode}</span>
        {mesh.revision && (
          <>
            <span className={t.metaSep}>·</span>
            <span className={t.selectorLabel}>revision</span>
            <span className={`${t.body} ${t.mono}`}>{mesh.revision}</span>
          </>
        )}
      </div>
      {/* Principals — SPIFFE IDs authorized (allow) or denied (notPrincipals)
          on ingress AuthorizationPolicies. Backend-sre flagged this as half
          the Istio Auth question — operator sees own SA above; principals
          show which callers the mesh expects. */}
      {(mesh.principals?.length || mesh.notPrincipals?.length) ? (
        <div className={t.selectorBlock}>
          <div className={t.selectorLabel}>Ingress principals</div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {mesh.principals?.map((principal) => (
              <span key={principal} className={t.principalChip} title="Allowed principal">{principal}</span>
            ))}
            {mesh.notPrincipals?.map((principal) => (
              <span key={principal} className={`${t.principalChip} ${t.principalChipDeny}`} title="Explicitly denied principal (notPrincipals)">
                ✗ {principal}
              </span>
            ))}
          </div>
        </div>
      ) : null}
      {/* Port-mode overrides — an effective PA may still permit plaintext on
          named ports (probes). Without this row, strict-ns w/ permissive-port
          override is invisible. Backend-sre gap C4. */}
      {mesh.portOverrides && Object.keys(mesh.portOverrides).length > 0 && (
        <div className={t.selectorBlock}>
          <div className={t.selectorLabel} title="Per-port mTLS mode overrides — override the effective PA on specific ports. Common for probe ports.">
            Port mTLS overrides
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {Object.entries(mesh.portOverrides).map(([port, mode]) => (
              <span key={port} className={t.portOverrideChip} data-mode={mode}>
                {port}: {mode}
              </span>
            ))}
          </div>
        </div>
      )}
      {/* PA chain — every PeerAuthentication scoping this workload. Effective
          one marked; others render dim so operator can see the resolution.
          Backend-sre gap C4: missing chain hides why a strict-ns workload is
          actually permissive at runtime. */}
      {mesh.peerAuthChain && mesh.peerAuthChain.length > 0 && (
        <div className={t.selectorBlock}>
          <div className={t.selectorLabel}>PeerAuthentication chain</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            {mesh.peerAuthChain.map((peerAuth) => (
              <div
                key={`${peerAuth.namespace}/${peerAuth.name}`}
                className={peerAuth.effective ? t.paChainEffective : t.paChainRow}
              >
                <span className={t.paChainScope}>{peerAuth.scope}</span>
                <PolicyRef source="istio" name={peerAuth.name} namespace={peerAuth.namespace} />
                <span className={t.paChainMode} data-mode={peerAuth.mode}>{peerAuth.mode}</span>
                {peerAuth.effective && <span className={t.paChainBadge}>effective</span>}
              </div>
            ))}
          </div>
        </div>
      )}
      {/* Waypoint (ambient mode) — for waypoint-proxied workloads, AuthorizationPolicies
          bind to the waypoint, not this workload. Naming the waypoint tells the
          operator where to look. Backend-sre gap C5. */}
      {mesh.waypoint && (
        <div className={t.policyMetaRow}>
          <span className={t.selectorLabel} title="Ambient waypoint proxy — enforces policy on this workload's behalf. AuthorizationPolicies bind to the waypoint.">
            Waypoint
          </span>
          <PolicyRef source="istio" name={mesh.waypoint.name} namespace={mesh.waypoint.namespace} />
        </div>
      )}
      {/* Non-fatal mesh issues — mode conflicts, missing sidecar w/ enrolled ns.
          Rendered as caution list. */}
      {mesh.mtlsIssues && mesh.mtlsIssues.length > 0 && (
        <div className={t.selectorBlock}>
          <div className={t.selectorLabel} style={{ color: 'var(--warn)' }}>Mesh issues</div>
          <ul className={t.mtlsIssueList}>
            {mesh.mtlsIssues.map((issue, index) => (
              <li key={index} className={t.body}>{issue}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

// Endpoint card used by Edge SRC/DST + Compare SRC/DST. Labels + listens both
// visible inline so operator can join policy selectors against workload without
// unfolding disclosures.
interface EndpointLike {
  label: string;
  namespace: string;
  type: string;
  labels: Record<string, string>;
  listensOn: Array<{ port: number; protocol: string }>;
}
function EndpointCard({ role, endpoint }: { role: 'SRC' | 'DST'; endpoint: EndpointLike }) {
  const selectorLabels = Object.entries(endpoint.labels).filter(([key]) => !isSystemLabel(key));
  // Ordering per SRE consult: name → labels → ns/type → listens. Labels
  // sit at card-line-2 because operator's "why does the policy select
  // this?" workflow starts on labels, and click-to-copy shortcuts the
  // kubectl -l flow. ns/type stay below as sharper cross-ns disambiguator.
  return (
    <div className={t.tier1}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <RolePill role={role} />
        <span className={t.section}>{endpoint.label}</span>
      </div>
      <LabelStrip labels={selectorLabels} />
      <div className={`${t.body} ${t.dim}`}>{endpoint.namespace} · {endpoint.type}</div>
      <div className={t.body}>
        <span className={t.dim}>listens: </span>
        {endpoint.listensOn.map((p) => `${p.port}/${p.protocol}`).join(', ')}
      </div>
    </div>
  );
}

// ── Node panel V2 ──────────────────────────────────────────────────────
// Layout order: identity → hero verdict + issues (fused when severity high) →
// per-engine collapsed summary → details in disclosure (labels, mesh).

export function NodePanelV2() {
  const node = MOCK_NODE;
  // Effective severity — folds status chips AND per-engine deny signal.
  // Prior version reduced over `node.statuses` only, so a workload with an
  // istio deny in `perEngine` but a warn-max status chip rendered "warn"
  // while istio was actively blocking. Silent lie. Any engine that blocks
  // (via `hasDeny` OR a selecting policy in `blocks` state) lifts the
  // effective verdict to critical regardless of the status-chip max.
  const statusRank = { info: 0, secure: 0, caution: 1, warning: 2, high: 3, critical: 4 } as const;
  const worstStatus = node.statuses.reduce<StatusChip['severity']>((acc, chip) => {
    return statusRank[chip.severity] > statusRank[acc] ? chip.severity : acc;
  }, 'info');
  const denyEngines = node.perEngine.filter(
    (engine) => engine.hasDeny || engine.selectingPolicies.some((p) => p.state === 'blocks'),
  );
  const anyEngineBlocks = denyEngines.length > 0;
  // When no engine has a deny, default-open the engine w/ most peers so the
  // panel doesn't read empty below the verdict. Operator opened the panel to
  // see evidence — give them the loudest evidence for free.
  const topByPeersEngine = anyEngineBlocks
    ? null
    : node.perEngine.reduce<typeof node.perEngine[number] | null>(
        (best, engine) => (best === null || (engine.inbound + engine.outbound) > (best.inbound + best.outbound)) ? engine : best,
        null,
      );
  const effectiveWorst: StatusChip['severity'] = anyEngineBlocks ? 'critical' : worstStatus;
  const verdictClass = effectiveWorst === 'high' || effectiveWorst === 'critical' ? t.deny
                     : effectiveWorst === 'warning' ? t.warn
                     : t.allow;

  // One-line hero verdict — cross-panel consistency w/ Edge + Compare which
  // both open w/ verdictText. Text composed off the deny signal + finding
  // count so operator gets the pager answer without reading pill taxonomy.
  const findingCount = node.issues.length;
  const findingLabel = `${findingCount} finding${findingCount === 1 ? '' : 's'}`;
  const heroText = anyEngineBlocks
    ? `✗ Blocked by ${denyEngines.map((e) => e.name).join(', ')}${findingCount ? ` · ${findingLabel}` : ''}`
    : (worstStatus === 'high' || worstStatus === 'critical')
      ? `⚠ At risk${findingCount ? ` · ${findingLabel}` : ''}`
      : (worstStatus === 'warning' || findingCount > 0)
        ? `⚠ ${findingLabel}`
        : '✓ Healthy';
  const heroClass = anyEngineBlocks || worstStatus === 'high' || worstStatus === 'critical'
    ? t.verdictDeny
    : (worstStatus === 'warning' || findingCount > 0)
      ? t.verdictWarn
      : t.verdictAllow;

  const selectorLabels: Array<[string, string]> = [];
  const systemLabels: Array<[string, string]> = [];
  for (const [key, value] of Object.entries(node.labels)) {
    (isSystemLabel(key) ? systemLabels : selectorLabels).push([key, value]);
  }

  // Resolution set — every policy the workload's engines actually see.
  // Findings referencing a policy NOT in this set = unresolved (may be
  // deleted, out of RBAC scope, in unfetched ns). Style dim + `?` so
  // operator doesn't chase a broken YAML link.
  const resolvedPolicyKeys = new Set(
    node.perEngine.flatMap((engine) =>
      engine.selectingPolicies.map((policy) => `${policy.source}/${policy.namespace}/${policy.name}`),
    ),
  );

  return (
    <div className={t.frame} style={{ width: 480 }}>
      <div className={t.frameHeader}>
        <span>Workload</span>
        <div className={t.frameHeaderMeta}>
          <StalenessStamp ageSec={42} />
          <IconClose />
        </div>
      </div>
      <div className={`${t.frameBody} ${t.tokens}`}>
        {/* Identity — name is hero, ns/type quiet subtitle, action promoted.
            Selector labels inline right below: they are the join key policies
            bind against, operator needs them at a glance. System/tooling
            labels get folded into the disclosure at the bottom. */}
        <div className={t.tier1}>
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8, justifyContent: 'space-between' }}>
            <div>
              {/* Name = the label. Verdict = the answer. Demoting name from
                  hero (18px) to section (15px) hands hero size to the verdict
                  callout below so operator's eye lands on the answer first. */}
              <div className={t.section}>{node.label}</div>
              <div className={`${t.body} ${t.dim}`} style={{ marginTop: 2 }}>
                {node.namespace} · {node.type}
              </div>
            </div>
            <button className={t.primaryButton} type="button">Pin as source</button>
          </div>
          <LabelStrip labels={selectorLabels} />
        </div>

        {/* Verdict hero — one-line summary above the pill row, mirroring
            Edge + Compare panels so operator scans Node → Edge → Compare w/o
            re-learning callout shape. Text derived off perEngine deny signal
            (not just status chips) to close the silent-lie gap. */}
        <div className={`${t.verdictCallout} ${verdictClass}`}>
          <div className={`${t.verdictText} ${heroClass}`}>{heroText}</div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
            {node.statuses.map((chip) => <StatusPill key={chip.key} chip={chip} />)}
          </div>
        </div>

        {/* Mesh block — SA + mTLS + revision. First check when an Istio
            workload is not reaching the mesh. */}
        {node.mesh && <MeshBlock mesh={node.mesh} />}

        {/* Findings — single channel. Each finding names the culprit policy
            + YAML button so operator can go straight to the manifest that
            caused the finding. */}
        {node.issues.length > 0 && (
          <div className={t.semanticWarn}>
            <div className={t.section}>
              {node.issues.length} finding{node.issues.length > 1 ? 's' : ''}
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }} className={t.body}>
              {node.issues.map((issue, index) => (
                <div key={index} style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, flexWrap: 'wrap' }}>
                    <span style={{ color: SEV_COLOR[issue.severity], fontWeight: 700 }}>●</span>
                    {issue.kind && <FindingKindChip kind={issue.kind} />}
                    <span style={{ flex: 1, minWidth: 0 }}>{issue.text}</span>
                    <button type="button" className={t.openReachButton} title="Open reachability panel scoped to this finding — compare this workload with the culprit's target so the blocker map is one click away.">
                      Reachability →
                    </button>
                  </div>
                  {issue.culprit && (
                    <div style={{ paddingLeft: 14 }}>
                      <PolicyRef
                        {...issue.culprit}
                        unresolved={!resolvedPolicyKeys.has(`${issue.culprit.source}/${issue.culprit.namespace}/${issue.culprit.name}`)}
                      />
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Per-engine — collapsed summary rows. Default-open only if deny.
            No .tier1 wrapper on the section, no .tier2 wrapper on open
            body: each PolicyCard/PostureRow/semanticDeny already carries
            its own surface + stripe; extra boxes = wall-of-cards. */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--gap-sm)' }}>
          <div className={t.eyebrow}>Per-engine evidence</div>
          {node.perEngine.map((engine) => (
            <details key={engine.name} open={engine.hasDeny || engine === topByPeersEngine}>
              <summary className={t.disclosureSummary}>
                <EngineChip name={engine.name} />
                <span style={{ display: 'flex', gap: 4, flex: 1 }}>
                  <CountCapsule kind="in" n={engine.inbound} />
                  <CountCapsule kind="out" n={engine.outbound} />
                </span>
                <div style={{ display: 'flex', gap: 4 }}>
                  {engine.statuses.map((chip) => <StatusPill key={chip.key} chip={chip} />)}
                </div>
              </summary>
              <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 'var(--gap-sm)' }}>
                {engine.hasDeny && engine.denyPolicies && engine.denyPolicies.length > 0 && (
                  <div className={t.semanticDeny}>
                    <div className={t.eyebrow} style={{ marginBottom: 6 }}>
                      Blocking ({engine.denyPolicies.length})
                    </div>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                      {engine.denyPolicies.map((policy) => (
                        <div
                          key={`${policy.namespace}/${policy.name}`}
                          style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}
                        >
                          <span
                            className={t.body}
                            style={{ color: policy.direction === 'egress' ? 'var(--role-src)' : 'var(--role-dst)', fontWeight: 600 }}
                          >
                            {policy.direction === 'egress' ? '↑' : '↓'} {policy.direction}
                          </span>
                          <PolicyRef {...policy} action="deny" />
                        </div>
                      ))}
                    </div>
                  </div>
                )}
                {/* Selecting policies — evidence for this engine's status.
                    Each row = action icon + engine + ns/name + direction +
                    state + YAML. State surfaces "inert" (root policy that
                    doesn't fire) vs "allows/blocks" so operator can tell
                    which of the 3 selectors are actually acting. */}
                <div className={t.eyebrow} style={{ marginTop: 2 }}>
                  Selecting policies ({engine.selectingPolicies.length})
                </div>
                {engine.selectingPolicies.length === 0 && (
                  <div className={`${t.body} ${t.dim}`}>
                    No {engine.name} policies select this workload — default posture applies.
                  </div>
                )}
                {(() => {
                  // Split three ways: posture rules (blanket coverage) as
                  // one-line PostureRow ABOVE per-peer cards; restricted
                  // (peer-specific) then further split into Denied / Allowed
                  // sections w/ distinct-peer counts. Deny always renders
                  // before allow — eval order (deny wins) is the teaching
                  // moment for a 3am pager. Mirrors live rows.tsx:537-565.
                  const posture = engine.selectingPolicies.filter(isPosture);
                  const restricted = engine.selectingPolicies.filter((p) => !isPosture(p));
                  const deny  = restricted.filter((p) => p.action === 'deny');
                  const allow = restricted.filter((p) => p.action !== 'deny');
                  const distinctPeerCount = (policies: SelectingPolicy[]): number => {
                    const keys = new Set<string>();
                    for (const policy of policies) {
                      for (const peer of policy.peers ?? []) {
                        keys.add(`${peer.namespace ?? ''}/${peer.label}/${peer.kind ?? ''}`);
                      }
                    }
                    return keys.size;
                  };
                  const denyPeerCount  = distinctPeerCount(deny);
                  const allowPeerCount = distinctPeerCount(allow);
                  return (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                      {posture.length > 0 && (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                          {posture.map((policy) => (
                            <PostureRow
                              key={`posture-${policy.namespace}/${policy.name}`}
                              policy={policy}
                              workloadNs={node.namespace}
                            />
                          ))}
                        </div>
                      )}
                      {deny.length > 0 && (
                        <>
                          <div className={t.eyebrow} style={{ marginTop: 2, color: 'var(--deny)' }}>
                            ⛔ Denied by policy ({deny.length}{denyPeerCount > 0 ? ` · ${denyPeerCount} peer${denyPeerCount === 1 ? '' : 's'}` : ''})
                          </div>
                          {deny.map((policy) => (
                            <PolicyCard
                              key={`deny-${policy.namespace}/${policy.name}`}
                              policy={policy}
                              workloadNs={node.namespace}
                            />
                          ))}
                        </>
                      )}
                      {allow.length > 0 && (
                        <>
                          <div className={t.eyebrow} style={{ marginTop: 2, color: 'var(--allow)' }}>
                            ✓ Allowed by policy ({allow.length}{allowPeerCount > 0 ? ` · ${allowPeerCount} peer${allowPeerCount === 1 ? '' : 's'}` : ''})
                          </div>
                          {allow.map((policy) => (
                            <PolicyCard
                              key={`allow-${policy.namespace}/${policy.name}`}
                              policy={policy}
                              workloadNs={node.namespace}
                            />
                          ))}
                        </>
                      )}
                    </div>
                  );
                })()}
                <div className={`${t.body} ${t.dim}`} style={{ marginTop: 4 }}>
                  {engine.inbound} inbound peers · {engine.outbound} outbound peers
                </div>
              </div>
            </details>
          ))}
        </div>

        {/* Provenance labels — helm/argocd/k8s.io noise. Behind disclosure so
            they stop drowning the selector labels shown above. */}
        {systemLabels.length > 0 && (
          <details>
            <summary className={`${t.disclosureSummary} ${t.eyebrow}`}>
              Show {systemLabels.length} system label{systemLabels.length > 1 ? 's' : ''}
            </summary>
            <div style={{ marginTop: 8 }}>
              <LabelStrip labels={systemLabels} />
            </div>
          </details>
        )}
      </div>
    </div>
  );
}

// ── Edge panel V2 ──────────────────────────────────────────────────────
// Layout: hero verdict banner (deny → "blocked by X" surfaced) → side-by-side
// endpoints w/ arrow between → policies w/ meta strip in header.

export function EdgePanelV2() {
  const edge = MOCK_EDGE;
  const isDeny = edge.verdict === 'deny';

  return (
    <div className={t.frame} style={{ width: 480 }}>
      <div className={t.frameHeader}>
        <span>Connection</span>
        <div className={t.frameHeaderMeta}>
          <StalenessStamp ageSec={310} />
          <IconClose />
        </div>
      </div>
      <div className={`${t.frameBody} ${t.tokens}`}>
        {/* Verdict hero — banner sits above endpoints w/ tinted bg. Blocked-by
            policy promoted so operator sees which control to touch. */}
        <div className={`${t.verdictCallout} ${isDeny ? t.deny : t.allow}`}>
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 10, flexWrap: 'wrap' }}>
            <span className={`${t.verdictText} ${isDeny ? t.verdictDeny : t.verdictAllow}`}>
              {isDeny ? '✗ blocked' : '✓ reachable'}
            </span>
            <div className={t.metaStrip}>
              {edge.engines.map((engine, index) => (
                <span key={engine.name} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                  <EngineChip name={engine.name} />
                  <ActionIcon action={engine.status} />
                  {index < edge.engines.length - 1 && <span className={t.metaSep} style={{ marginLeft: 2 }}>·</span>}
                </span>
              ))}
            </div>
          </div>
          {isDeny && edge.blockedBy && (
            <div className={t.body} style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
              <strong>Blocked by:</strong>
              <PolicyRef {...edge.blockedBy} action="deny" />
            </div>
          )}
          <div className={`${t.body} ${t.dim}`}>{edge.reason}</div>
        </div>

        {/* Endpoints — side-by-side w/ arrow. Directional relationship visible.
            Labels shown inline: this is the join key for the selecting policies
            below, so operator can trace policy match without unfolding anything. */}
        <div className={t.endpointRow}>
          <EndpointCard role="SRC" endpoint={edge.src} />
          <div className={t.endpointArrow}>→</div>
          <EndpointCard role="DST" endpoint={edge.dst} />
        </div>

        {/* Findings on this pair — same shape as Node findings: text +
            culprit PolicyRef + YAML so operator sees which policy caused it. */}
        {edge.issues.length > 0 && (
          <div className={t.semanticWarn}>
            <div className={t.section}>Findings</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }} className={t.body}>
              {edge.issues.map((issue, index) => (
                <div key={index} style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, flexWrap: 'wrap' }}>
                    <span style={{ color: SEV_COLOR[issue.severity], fontWeight: 700 }}>●</span>
                    {issue.kind && <FindingKindChip kind={issue.kind} />}
                    <span style={{ flex: 1, minWidth: 0 }}>{issue.text}</span>
                    <button type="button" className={t.openReachButton} title="Open reachability panel scoped to this pair — same SRC/DST already selected.">
                      Reachability →
                    </button>
                  </div>
                  {issue.culprit && (
                    <div style={{ paddingLeft: 14 }}>
                      <PolicyRef {...issue.culprit} />
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Policies — meta strip in header, no repeated label-value rows.
            No .tier1 wrapper: each row already carries its own surface. */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--gap-sm)' }}>
          <div className={t.eyebrow}>Policies ({edge.policies.length})</div>
          {edge.policies.map((policy, index) => {
            const isDenyPol = policy.action === 'deny';
            // Port intersection is always vs DST listens — regardless of policy
            // direction. Prior version compared egress rules against SRC's own
            // listens, which are irrelevant to what SRC can reach. Off-by-role
            // bug flagged by backend-sre.
            const deadLetter = policy.ports.length > 0 && !policy.ports.some(
              (rp) => edge.dst.listensOn.some((lp) => lp.port === rp.port),
            );
            return (
              <div key={index} className={isDenyPol ? t.semanticDeny : t.tier2}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', gap: 8 }}>
                  <div className={t.body} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                    <ActionIcon action={policy.action} />
                    <span className={t.section}>{policy.name}</span>
                    <span className={t.dim}>· {policy.namespace}</span>
                  </div>
                  <div className={t.metaStrip}>
                    <EngineChip name={policy.source} />
                    <span className={t.body} style={{ color: policy.direction === 'egress' ? 'var(--role-src)' : 'var(--role-dst)' }}>
                      {policy.direction === 'egress' ? '↑' : '↓'} {policy.direction}
                    </span>
                    <ManifestLink />
                  </div>
                </div>
                <div className={t.body}>
                  <span className={t.dim}>ports: </span>
                  {policy.ports.length === 0
                    ? <span className={t.dim}>all</span>
                    : policy.ports.map((p) => `${p.port}/${p.protocol}`).join(', ')}
                  {deadLetter && (
                    <span
                      style={{ marginLeft: 8, color: 'var(--caution)' }}
                      title="Policy permits this port but no declared containerPort on the destination advertises it. containerPorts are declarative only — the workload may still bind other ports at runtime. Verify with `kubectl get pod -o yaml` or the Service targetPort before treating this as broken."
                    >
                      ⚠ no declared listener on this port
                    </span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

// MeshSideCard — Compare engine grid src/dst column mesh block. Renders one
// side of the mesh identity: SA + mTLS + sidecar + (dst-only) principals.
// Only istio engine rows carry this — k8s row omits the block. Compact
// version of NodePanel MeshBlock so operator scans one column at a time.
function MeshSideCard({ side, mesh }: { side: 'SRC' | 'DST'; mesh: MeshInfo }) {
  const mtlsColor = mesh.mtlsMode === 'STRICT' ? 'var(--allow)'
                  : mesh.mtlsMode === 'DISABLE' ? 'var(--deny)'
                  : mesh.mtlsMode === 'PERMISSIVE' ? 'var(--caution)'
                  : '#7a848e';
  const enrolled = mesh.enrolled && mesh.sidecarInjected;
  return (
    <div className={t.tier2} style={{ marginTop: 6 }}>
      <div className={t.policyMetaRow}>
        <EngineChip name="mesh" />
        <span className={t.eyebrow} style={{ flex: 1 }}>{side} mesh</span>
        {!enrolled && (
          <span className={t.notEnrolled} title="Sidecar missing or ns not mesh-tracked — AuthorizationPolicies cannot enforce">
            not enrolled
          </span>
        )}
      </div>
      <div className={t.selectorBlock}>
        <div className={t.selectorLabel}>SA</div>
        <div className={`${t.body} ${t.mono}`} style={{ color: '#e6ecf2', wordBreak: 'break-all' }}>
          {mesh.serviceAccount}
        </div>
      </div>
      <div className={t.policyMetaRow}>
        <span className={t.selectorLabel}>mTLS</span>
        <span className={t.body} style={{ color: mtlsColor, fontWeight: 700 }}
          title={
            mesh.mtlsMode === 'STRICT' ? 'STRICT — inbound plaintext rejected; caller must present valid mesh certificate.'
          : mesh.mtlsMode === 'PERMISSIVE' ? 'PERMISSIVE — accepts both mTLS and plaintext.'
          : mesh.mtlsMode === 'DISABLE' ? 'DISABLE — mTLS off entirely; all traffic plaintext.'
          : 'UNSET — no PeerAuthentication applies; ns/mesh default takes effect.'
          }
        >{mesh.mtlsMode}</span>
        {mesh.revision && (
          <>
            <span className={t.metaSep}>·</span>
            <span className={t.selectorLabel}>rev</span>
            <span className={`${t.body} ${t.mono}`}>{mesh.revision}</span>
          </>
        )}
      </div>
      {/* Principals only meaningful on DST — AuthorizationPolicy principals list
          gates ingress. SRC has no equivalent field. */}
      {side === 'DST' && (mesh.principals?.length || mesh.notPrincipals?.length) ? (
        <div className={t.selectorBlock}>
          <div className={t.selectorLabel}>Ingress principals</div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {mesh.principals?.map((principal) => (
              <span key={principal} className={t.principalChip} title="Allowed principal">{principal}</span>
            ))}
            {mesh.notPrincipals?.map((principal) => (
              <span key={principal} className={`${t.principalChip} ${t.principalChipDeny}`} title="Explicitly denied (notPrincipals)">
                ✗ {principal}
              </span>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}

// DirectionVerdictRow — per-engine verdict-detail cell breakdown. Traffic is
// evaluated per-direction (ingress = DST-side rules, egress = SRC-side rules)
// so a single-verdict row hid which axis flipped the answer. Render both.
function DirectionVerdictRow({
  direction, verdict, reason,
}: { direction: 'ingress' | 'egress'; verdict: 'allow' | 'deny'; reason: string }) {
  const shellClass = verdict === 'deny' ? t.semanticDeny : t.semanticAllow;
  const arrowColor = direction === 'egress' ? 'var(--role-src)' : 'var(--role-dst)';
  return (
    <div className={shellClass}>
      <div className={t.policyMetaRow}>
        <span className={t.body} style={{ color: arrowColor, fontWeight: 700 }}
          title={direction === 'ingress'
            ? 'Ingress = DST-side rules evaluate the incoming SRC. Failure here means DST refused the caller.'
            : 'Egress = SRC-side rules evaluate the outbound target. Failure here means SRC was locked from reaching DST.'
          }
        >
          {direction === 'egress' ? '↑' : '↓'} {direction}
        </span>
        <span style={{ flex: 1 }} />
        <span className={t.body} style={{ color: verdict === 'deny' ? 'var(--deny)' : 'var(--allow)', fontWeight: 700 }}>
          {verdict === 'deny' ? '✗ deny' : '✓ allow'}
        </span>
      </div>
      <div className={`${t.body} ${t.dim}`} style={{ marginTop: 2 }}>{reason}</div>
    </div>
  );
}

// MeshAxisRow — mesh axis inside per-engine verdict detail. Same three-axis
// derivation as the old top-level MeshComparisonBlock, condensed to fit
// alongside ingress/egress rows in one column.
function MeshAxisRow({ src, dst, verdict, reason }: { src: MeshInfo; dst: MeshInfo; verdict: 'allow' | 'deny' | 'warn'; reason: string }) {
  const shellClass = verdict === 'deny' ? t.semanticDeny : verdict === 'warn' ? t.semanticWarn : t.semanticAllow;
  const color = verdict === 'deny' ? 'var(--deny)' : verdict === 'warn' ? 'var(--caution)' : 'var(--allow)';
  // Recompute axes so tooltips explain the derivation (backend supplies the
  // top-line verdict + reason; the axis breakdown is UI-derived from mesh).
  const srcEnrolled = src.enrolled && src.sidecarInjected;
  const dstEnrolled = dst.enrolled && dst.sidecarInjected;
  const enrollmentOk = srcEnrolled && dstEnrolled;
  const mtlsOk =
    dst.mtlsMode === 'STRICT' ? (src.mtlsMode === 'STRICT' || src.mtlsMode === 'PERMISSIVE')
    : dst.mtlsMode === 'DISABLE' ? (src.mtlsMode === 'DISABLE' || src.mtlsMode === 'PERMISSIVE')
    : true;
  const mtlsUnknown = dst.mtlsMode === 'UNSET' || src.mtlsMode === 'UNSET';
  const srcSA = src.serviceAccount;
  const onDeny = dst.notPrincipals?.includes(srcSA) ?? false;
  const onAllow = dst.principals?.includes(srcSA) ?? false;
  const noList = !(dst.principals?.length || dst.notPrincipals?.length);
  const principalVerdict: 'deny' | 'allow' | 'implicit' | 'not-listed' =
    onDeny ? 'deny'
    : onAllow ? 'allow'
    : noList ? 'implicit'
    : 'not-listed';

  const AxisMini = ({
    label, ok, warn, detail,
  }: { label: string; ok: boolean; warn?: boolean; detail: string }) => (
    <div className={t.policyMetaRow} style={{ gap: 6 }}>
      <span style={{ color: ok ? 'var(--allow)' : warn ? 'var(--caution)' : 'var(--deny)', fontWeight: 700, minWidth: 10 }}>
        {ok ? '✓' : warn ? '⚠' : '✗'}
      </span>
      <span className={t.eyebrow} style={{ minWidth: 76 }}>{label}</span>
      <span className={`${t.body} ${t.dim}`} style={{ flex: 1, minWidth: 0 }}>{detail}</span>
    </div>
  );

  return (
    <div className={shellClass}>
      <div className={t.policyMetaRow}>
        <EngineChip name="mesh" />
        <span className={t.body} style={{ fontWeight: 700, flex: 1 }}>Mesh</span>
        <span className={t.body} style={{ color, fontWeight: 700 }}>
          {verdict === 'deny' ? '✗ deny' : verdict === 'warn' ? '⚠ caveat' : '✓ allow'}
        </span>
      </div>
      <div className={`${t.body} ${t.dim}`} style={{ marginTop: 2, marginBottom: 4 }}>{reason}</div>
      <AxisMini
        label="enrollment"
        ok={enrollmentOk}
        detail={
          !srcEnrolled && !dstEnrolled ? 'both sides not in mesh'
          : !srcEnrolled ? 'SRC not enrolled — no sidecar to carry SPIFFE identity'
          : !dstEnrolled ? 'DST not enrolled — AuthorizationPolicies cannot enforce'
          : 'both sidecars injected'
        }
      />
      <AxisMini
        label="mTLS"
        ok={mtlsOk && !mtlsUnknown}
        warn={mtlsUnknown}
        detail={`SRC ${src.mtlsMode} → DST ${dst.mtlsMode}${mtlsUnknown ? ' — UNSET falls back to ns default' : (!mtlsOk ? ' — incompatible' : '')}`}
      />
      <AxisMini
        label="principal fit"
        ok={principalVerdict === 'allow' || principalVerdict === 'implicit'}
        warn={principalVerdict === 'not-listed'}
        detail={
          principalVerdict === 'deny' ? 'SRC SA on notPrincipals — explicit deny'
          : principalVerdict === 'allow' ? 'SRC SA on principals — explicit allow'
          : principalVerdict === 'implicit' ? 'no principal list on DST — mesh default applies'
          : 'SRC SA not on principals nor notPrincipals — DST rules may not match'
        }
      />
    </div>
  );
}

// SelectingPolicyRow — Compare engine grid cell row. Mirrors NodePanel
// PolicyCard header: action icon + PolicyRef + direction arrow + coverage
// capsule (deny all/allow all/restricted/unenforced) + ports + state + one-
// line verdictDetail. Enables operator to tell ingress selectors from egress
// and see which policies are actually firing without opening YAML.
interface CompareSelectingPolicy {
  source: string;
  name: string;
  namespace: string;
  action: 'allow' | 'deny';
  state: 'allows' | 'blocks' | 'inert' | 'unenforced';
  direction: 'ingress' | 'egress';
  coverage?: 'restricted' | 'deny all' | 'allow all' | 'unenforced';
  ports?: Array<{ port: number; protocol: string }>;
  anyPort?: boolean;
  verdictDetail?: string;
}
function SelectingPolicyRow({ policy }: { policy: CompareSelectingPolicy }) {
  const shellClass = policy.state === 'blocks' ? t.semanticDeny
                   : policy.state === 'allows' ? t.semanticAllow
                   : policy.state === 'unenforced' ? t.semanticWarn
                   : t.tier2;
  const stateColor = policy.state === 'blocks' ? 'var(--deny)'
                   : policy.state === 'allows' ? 'var(--allow)'
                   : policy.state === 'unenforced' ? 'var(--warn)'
                   : '#7a848e';
  const arrowColor = policy.direction === 'egress' ? 'var(--role-src)' : 'var(--role-dst)';
  return (
    <div className={shellClass}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
        <PolicyRef
          source={policy.source}
          name={policy.name}
          namespace={policy.namespace}
          action={policy.action}
        />
      </div>
      <div className={t.policyMetaRow} style={{ marginTop: 4 }}>
        <span
          className={t.body}
          style={{ color: arrowColor, fontWeight: 600 }}
          title={`Direction: ${policy.direction}`}
        >
          {policy.direction === 'egress' ? '↑' : '↓'} {policy.direction}
        </span>
        {policy.coverage && policy.coverage !== 'restricted' && (
          <span className={t.postureCoverage} title={
            policy.coverage === 'deny all' ? 'Catch-all deny for this direction'
            : policy.coverage === 'allow all' ? 'Catch-all allow for this direction'
            : 'No rules on this direction — leaves it wide open'
          }>{policy.coverage}</span>
        )}
        <span className={`${t.body} ${t.dim}`}>ports:</span>
        <PortList ports={policy.ports} anyPort={policy.anyPort} />
        <span style={{ flex: 1 }} />
        <span className={t.body} style={{ color: stateColor, fontWeight: 700 }}
          title={
            policy.state === 'inert' ? 'Rule exists but does not fire — wrong side or eval order'
            : policy.state === 'unenforced' ? 'Selects workload but has no rules on this direction'
            : undefined
          }
        >{policy.state}</span>
      </div>
      {policy.verdictDetail && (
        <div className={`${t.body} ${t.dim}`} style={{ marginTop: 4 }}>{policy.verdictDetail}</div>
      )}
    </div>
  );
}

// PortsSummarySection — SRC egress vs DST ingress per engine. Highlights
// port mismatches (SRC opens 8080, DST accepts 9090) which the headline
// `reachablePorts` list hides on deny verdicts. Live PortsSummary
// (ReachabilityView.tsx:165-187). Skips engines w/ no port data.
interface SidePorts {
  blocked: boolean;
  allPorts: boolean;
  ports: Array<{ port: number; protocol: string }>;
}
function SidePortBadges({ side }: { side: SidePorts }) {
  if (side.blocked)  return <span className={`${t.body} ${t.dim}`}>blocked</span>;
  if (side.allPorts) return <span className={t.portBadgeAll} title="No port restriction — all ports allowed">all ports</span>;
  if (side.ports.length === 0) return <span className={`${t.body} ${t.dim}`}>none</span>;
  return (
    <>
      {side.ports.map((port, index) => (
        <span key={index} className={t.portBadge}>{port.port}/{port.protocol}</span>
      ))}
    </>
  );
}
function PortsSummarySection({ engines }: {
  engines: Array<{ name: string; srcEgressPorts?: SidePorts; dstIngressPorts?: SidePorts }>;
}) {
  const withPorts = engines.filter((engine) => engine.srcEgressPorts && engine.dstIngressPorts);
  if (withPorts.length === 0) return null;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <div className={t.eyebrow}>Ports (per engine)</div>
      {withPorts.map((engine) => (
        <div key={engine.name} className={t.portsSummaryRow}>
          <EngineChip name={engine.name} />
          <span className={t.portsSummaryLabel}>SRC egress</span>
          <span className={t.portsSummaryChips}>
            <SidePortBadges side={engine.srcEgressPorts!} />
          </span>
          <span className={`${t.body} ${t.dim}`}>→</span>
          <span className={t.portsSummaryLabel}>DST ingress</span>
          <span className={t.portsSummaryChips}>
            <SidePortBadges side={engine.dstIngressPorts!} />
          </span>
        </div>
      ))}
    </div>
  );
}

// ── Compare panel V2 ───────────────────────────────────────────────────
// Layout: verdict headline (2-line) → blockers (Tier 1 answer) → ports summary
// → engine-major grid (row per engine, 3 cols: src selecting | dst selecting | verdict).

export function ComparePanelV2() {
  const cmp = MOCK_COMPARE;
  const isDeny = cmp.verdict === 'deny';
  // Swap SRC ↔ DST — reverse-direction inspection. Mock: no-op onclick; in
  // wire-up, swaps the pinned src + dst in graphStore and re-runs reachability.
  const swapSides = () => {
    // wire-up: graphStore.swapReachabilityPair()
  };

  return (
    <div className={t.frame} style={{ width: 960 }}>
      <div className={t.frameHeader}>
        <span>Reachability</span>
        <div className={t.frameHeaderMeta}>
          <StalenessStamp ageSec={1200} />
          <IconClose />
        </div>
      </div>
      <div className={`${t.frameBody} ${t.tokens}`}>
        {/* Verdict headline — 2-line composition. Line 1: badge + sentence.
            Line 2: subsystem chips + bidirectional. */}
        <div className={`${t.verdictCallout} ${isDeny ? t.deny : t.allow}`}>
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap' }}>
            <span className={`${t.verdictText} ${isDeny ? t.verdictDeny : t.verdictAllow}`}>
              {isDeny ? '✗ cannot reach' : '✓ can reach'}
            </span>
            <span className={t.section}>
              <span className={`${t.roleDot} ${t.roleDotSrc}`} /> {cmp.src.label}
              <span className={t.dim}> → </span>
              <span className={`${t.roleDot} ${t.roleDotDst}`} /> {cmp.dst.label}
            </span>
          </div>
          <div className={t.metaStrip}>
            {cmp.engines.map((engine, index) => (
              <span key={engine.name} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                <EngineChip name={engine.name} />
                <ActionIcon action={engine.verdict} />
                {index < cmp.engines.length - 1 && <span className={t.metaSep} style={{ marginLeft: 2 }}>·</span>}
              </span>
            ))}
          </div>
          {/* Bidirectional promoted out of metaStrip — reverse-direction verdict
              is a distinct concept, not another engine chip. Own row w/ eyebrow
              so operator recognizes forward vs reverse split. */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <span className={t.eyebrow}>Reverse</span>
            {cmp.bidirectional.reverseError ? (
              /* Reverse-direction fetch failed — explicit "unavailable" chip so
                 operator doesn't misread absence as "no bidirectional info". */
              <span
                className={t.body}
                style={{ color: 'var(--warn)', fontWeight: 700 }}
                title="Reverse-direction (DST → SRC) fetch failed. Verdict unknown; treat as unconfirmed."
              >
                ⚠ reverse unavailable
              </span>
            ) : (
              <span
                className={t.body}
                style={{ color: cmp.bidirectional.reverse === 'allow' ? 'var(--allow)' : 'var(--deny)', fontWeight: 700 }}
                title="Forward = SRC → DST (blockers listed below). Reverse = DST → SRC is evaluated independently and may block for different reasons."
              >
                {cmp.bidirectional.label}
              </span>
            )}
            <button
              type="button"
              className={t.reverseButton}
              onClick={swapSides}
              title="Swap SRC ↔ DST and re-evaluate reachability. Reverse direction is evaluated independently — different policies may block."
            >
              ⇄ Swap SRC ↔ DST
            </button>
            {!cmp.bidirectional.reverseError && cmp.bidirectional.forward !== cmp.bidirectional.reverse ? (
              <span className={`${t.body} ${t.dim}`}>
                Reverse ({cmp.bidirectional.reverse}) differs from forward. Swap to see reverse blockers.
              </span>
            ) : !cmp.bidirectional.reverseError && cmp.bidirectional.reverse === 'deny' ? (
              <span className={`${t.body} ${t.dim}`}>
                Blockers below = SRC → DST only. Reverse also blocked, possibly for different reasons — swap to inspect.
              </span>
            ) : null}
          </div>
          {/* Ports shown ONLY on allow — hidden on deny (theoretical noise). */}
          {!isDeny && cmp.reachablePorts.length > 0 && (
            <div className={t.body}>
              <span className={t.dim}>reachable on: </span>
              {cmp.reachablePorts.map((p) => `${p.port}/${p.protocol}`).join(', ')}
            </div>
          )}
        </div>

        {/* Endpoint pair — labels visible so operator can trace policy
            selectors without unfolding. Same shape as Edge panel. Arrow is
            clickable in Compare: swaps SRC/DST and re-runs reachability. */}
        <div className={t.endpointRow}>
          <EndpointCard role="SRC" endpoint={cmp.src} />
          <button
            type="button"
            className={t.endpointArrowButton}
            onClick={swapSides}
            title="Swap SRC ↔ DST — re-run reachability with reversed direction"
            aria-label="Swap SRC and DST"
          >⇄</button>
          <EndpointCard role="DST" endpoint={cmp.dst} />
        </div>

        {/* Visibility caveat — RBAC or missing CRDs make one side invisible.
            Promoted above blockers because a "deny" verdict computed from a
            half-visible policy set is a silent lie: allow rules the operator
            cannot see could flip the answer. Backend-sre gap fix. */}
        {(() => {
          const missing = cmp.engines
            .flatMap((engine) => [
              engine.srcUnavailable ? { engine: engine.name, side: 'SRC' as const, reason: engine.srcUnavailable.reason } : null,
              engine.dstUnavailable ? { engine: engine.name, side: 'DST' as const, reason: engine.dstUnavailable.reason } : null,
            ])
            .filter((entry): entry is { engine: string; side: 'SRC' | 'DST'; reason: string } => entry !== null);
          if (missing.length === 0) return null;
          return (
            <div className={t.semanticWarn}>
              <div className={t.section}>
                Verdict computed with partial visibility
              </div>
              <div className={`${t.body} ${t.dim}`}>
                Some policies were not readable — allow rules that could flip the
                answer may exist but stay invisible. Fix access before trusting
                the verdict.
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 4 }}>
                {missing.map((entry, index) => (
                  <div key={index} style={{ display: 'flex', gap: 8, alignItems: 'flex-start', flexWrap: 'wrap' }}>
                    <RolePill role={entry.side} />
                    <EngineChip name={entry.engine} />
                    <span className={t.body} style={{ flex: 1, minWidth: 0 }}>{entry.reason}</span>
                  </div>
                ))}
              </div>
            </div>
          );
        })()}

        {/* Blockers — top-of-scroll answer. Each blocker card carries its
            own surface + stripe; no outer .tier1 wrap. */}
        {isDeny && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--gap-sm)' }}>
            <div className={t.section}>{cmp.reason}</div>
            {cmp.blockers.map((blocker, index) => (
              <div key={index} className={t.semanticDeny}>
                <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                  <RolePill role={blocker.side} />
                  <span className={`${t.body} ${t.dim}`}>{blocker.direction}</span>
                  <PolicyRef
                    source={blocker.engine}
                    name={blocker.culprit.name}
                    namespace={blocker.culprit.namespace}
                    action="deny"
                  />
                </div>
                <div className={`${t.body} ${t.dim}`}>reason: {blocker.reason}</div>
                {/* PeerAuthentication culprit — for mesh-mTLS blockers, the PA
                    forcing the block. Without this, an mTLS blocker has no path
                    to the manifest that caused it. */}
                {blocker.paSource && (
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                    <span className={`${t.body} ${t.dim}`}>Forced by PA:</span>
                    <PolicyRef
                      source="istio"
                      name={blocker.paSource.name}
                      namespace={blocker.paSource.namespace}
                      action="deny"
                    />
                  </div>
                )}
                {blocker.deleteToAllow.length > 0 && (
                  <div className={t.body}>
                    <div className={t.eyebrow} style={{ marginTop: 4 }}>Matching subrule</div>
                    {blocker.deleteToAllow.map((rule, ruleIndex) => (
                      <div key={ruleIndex} className={t.mono} style={{ marginTop: 2 }}>
                        {rule.rule}
                      </div>
                    ))}
                    <div className={`${t.body} ${t.dim}`} style={{ marginTop: 4, fontSize: 11 }}>
                      Removing this subrule unblocks the pair. Does not delete the
                      surrounding policy — edit <span className={t.mono}>{blocker.culprit.namespace}/{blocker.culprit.name}</span>{' '}
                      and drop only this entry.
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Ports summary — SRC egress vs DST ingress per engine. Shows
            port-mismatch cases ("SRC opens 8080, DST accepts 9090") without
            unfolding the engine grid. Live PortsSummary (ReachabilityView.tsx:165). */}
        <PortsSummarySection engines={cmp.engines} />

        {/* Engine-major grid — row per engine. Eye sweeps left→right per
            engine instead of top→bottom per column. */}
        <details open={isDeny}>
          <summary className={`${t.disclosureSummary} ${t.eyebrow}`}>
            Per-engine detail
          </summary>
          <div className={t.engineGrid} style={{ marginTop: 10 }}>
            <div className={t.eyebrow}>Src selecting</div>
            <div className={t.eyebrow}>Dst selecting</div>
            <div className={t.eyebrow}>Verdict detail</div>
            {cmp.engines.map((engine) => (
              <div key={engine.name} className={t.engineGridRow}>
                <div className={t.tier2}>
                  <EngineChip name={engine.name} />
                  {/* Distinguish `no policies match` from `we could not fetch`.
                      Empty `[]` renders as "none"; explicit `srcUnavailable`
                      renders as caution card w/ reason. Silent-lie fix. */}
                  {engine.srcUnavailable ? (
                    <div className={t.engineUnavailable} title={engine.srcUnavailable.reason}>
                      <strong>src side unavailable</strong>
                      <span>{engine.srcUnavailable.reason}</span>
                    </div>
                  ) : engine.srcSelecting.length === 0 ? (
                    <span className={`${t.body} ${t.dim}`}>none</span>
                  ) : engine.srcSelecting.map((policy, index) => (
                    <SelectingPolicyRow key={index} policy={policy} />
                  ))}
                  {engine.mesh && <MeshSideCard side="SRC" mesh={engine.mesh.src} />}
                </div>
                <div className={t.tier2}>
                  <EngineChip name={engine.name} />
                  {engine.dstUnavailable ? (
                    <div className={t.engineUnavailable} title={engine.dstUnavailable.reason}>
                      <strong>dst side unavailable</strong>
                      <span>{engine.dstUnavailable.reason}</span>
                    </div>
                  ) : engine.dstSelecting.length === 0 ? (
                    <span className={`${t.body} ${t.dim}`}>none</span>
                  ) : engine.dstSelecting.map((policy, index) => (
                    <SelectingPolicyRow key={index} policy={policy} />
                  ))}
                  {engine.mesh && <MeshSideCard side="DST" mesh={engine.mesh.dst} />}
                </div>
                {/* Verdict detail — per-direction breakdown + top-line verdict.
                    Traffic evaluated separately on ingress + egress; single
                    verdict word hid which axis flipped the answer. */}
                <div className={engine.verdict === 'deny' ? t.semanticDeny : t.semanticAllow}>
                  <div className={t.policyMetaRow}>
                    <span className={t.body} style={{ fontWeight: 700, flex: 1 }}>Overall</span>
                    <span className={t.body} style={{ color: engine.verdict === 'deny' ? 'var(--deny)' : 'var(--allow)', fontWeight: 700 }}>
                      {engine.verdict === 'deny' ? '✗ deny' : '✓ allow'}
                    </span>
                  </div>
                  <div className={`${t.body} ${t.dim}`}>{engine.verdictReason}</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 8 }}>
                    <DirectionVerdictRow
                      direction="ingress"
                      verdict={engine.ingressVerdict.verdict}
                      reason={engine.ingressVerdict.reason}
                    />
                    <DirectionVerdictRow
                      direction="egress"
                      verdict={engine.egressVerdict.verdict}
                      reason={engine.egressVerdict.reason}
                    />
                    {engine.mesh && (
                      <MeshAxisRow
                        src={engine.mesh.src}
                        dst={engine.mesh.dst}
                        verdict={engine.mesh.verdict}
                        reason={engine.mesh.reason}
                      />
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </details>
      </div>
    </div>
  );
}
