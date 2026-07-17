// Workload-detail view: single-node click. Identity header + pin action,
// labels, effective status (hero), mesh membership, detected issues, and
// per-engine evidence (status + selecting policies + outbound rules).

import type { WorkloadNode, NodeDetail, Severity } from '../../../data/policies';
import { STATUS_CFG } from '../../../data/policies';
import s from '../DetailPanel.module.css';
import { EngineBadge, LabelStrip } from '../shared/badges';
import { NeighborList } from '../shared/rows';
import { distinctPeerCount } from '../shared/groupNeighborsByPolicy';
import { StatusBadges } from '../shared/status';
import { MeshCard } from '../shared/mesh';
import { NodeIssues } from '../shared/issues';

// Worst-severity fold over status keys — drives the hero verdict callout
// tint. Mockup principle: hero is one-per-panel, may use full tint.
const SEVERITY_RANK: Record<Severity, number> = {
  info: 0, secure: 0, caution: 1, warning: 2, high: 3, critical: 4,
};
function worstSeverity(keys: readonly string[]): Severity {
  let worst: Severity = 'info';
  for (const key of keys) {
    const cfg = STATUS_CFG[key as keyof typeof STATUS_CFG];
    if (!cfg) continue;
    if (SEVERITY_RANK[cfg.severity] > SEVERITY_RANK[worst]) worst = cfg.severity;
  }
  return worst;
}

export function WorkloadView({ node, nodeInfo, nodeInfoLoading, canPin, onPin }: {
  node:            WorkloadNode;
  nodeInfo:        NodeDetail | null;
  nodeInfoLoading: boolean;
  canPin:          boolean;
  onPin:           (node: WorkloadNode) => void;
}) {
  const worst = worstSeverity(node.statuses ?? []);
  const findingCount = (nodeInfo?.issues?.length ?? 0);
  // Hero verdict — one-line answer above the evidence. Class-based tone so
  // color lives in tokens, not inline styles.
  const heroDeny = worst === 'critical' || worst === 'high';
  const heroWarn = worst === 'warning' || findingCount > 0;
  const calloutClass = heroDeny ? s.verdictDeny : heroWarn ? s.verdictWarn : s.verdictAllow;
  const heroTextClass = heroDeny ? s.verdictTextDeny : heroWarn ? s.verdictTextWarn : s.verdictTextAllow;
  const verdictText = heroDeny ? '⚠ At risk'
                     : findingCount > 0 ? `⚠ ${findingCount} finding${findingCount === 1 ? '' : 's'}`
                     : worst === 'warning' ? '⚠ Warnings'
                     : '✓ Healthy';

  return (
    <div className="d-flex flex-column gap-3">
      {/* Identity tier1 — name (section) + ns/type dim + Pin action + labels
          inline. Selector-currency labels sit next to name because operator's
          "why does this policy match?" workflow starts on labels. System
          labels fold behind LabelStrip's built-in disclosure. */}
      <div className={s.tier1}>
        <div className="d-flex justify-content-between align-items-start gap-2">
          <div className="d-flex flex-column" style={{ minWidth: 0 }}>
            <div className={`${s.section} text-break`}>{node.label}</div>
            <div className={`${s.dim} ${s.body}`}>
              {node.namespace || '—'} · {node.type}
            </div>
          </div>
          {canPin && (
            <button
              className={`${s.primaryButton} flex-shrink-0`}
              onClick={() => onPin(node)}
            >
              Pin as source
            </button>
          )}
        </div>
        <LabelStrip labels={node.labels} />
      </div>

      {/* Verdict hero — one-line answer + status pills as evidence. Full-tint
          bg legal here (STYLEGUIDE §3, hero one-per-panel). */}
      <div className={`${s.verdictCallout} ${calloutClass}`}>
        <div className={`${s.verdictText} ${heroTextClass}`}>{verdictText}</div>
        <StatusBadges keys={node.statuses ?? []} />
      </div>

      {/* Mesh — SA identity + mTLS mode + revision + PA chain. First thing
          checked when an Istio pod isn't reaching the mesh. Placed above the
          per-engine section so operator sees it on scroll open. */}
      {nodeInfo?.mesh && Object.keys(nodeInfo.mesh).length > 0 && (
        <div>
          <div className={`${s.eyebrow} mb-1`}>Mesh</div>
          <div className="d-flex flex-column gap-2">
            {Object.entries(nodeInfo.mesh).map(([source, m]) => (
              <MeshCard key={source} source={source} membership={m} />
            ))}
          </div>
        </div>
      )}

      {/* Workload-scoped issues from nodeInfo — semantic warn shell w/ list. */}
      {nodeInfo?.issues && nodeInfo.issues.length > 0 && (
        <div className={s.semanticWarn}>
          <div className={s.section}>
            {nodeInfo.issues.length} issue{nodeInfo.issues.length > 1 ? 's' : ''} detected
          </div>
          <ul className={`ps-3 mb-0 ${s.body}`}>
            {nodeInfo.issues.map((issue, i) => <li key={i}>{issue}</li>)}
          </ul>
        </div>
      )}

      {/* Cluster findings touching this node (conflicts, lockouts) — same
          issues the drawer/table list, scoped here so operator sees them
          without leaving the panel. */}
      <NodeIssues nodeId={node.id} />

      {(() => {
        const engines = new Set<string>([
          ...Object.keys(node.statusesBySource ?? {}),
          ...Object.keys(nodeInfo?.neighbors ?? {}),
        ]);
        if (engines.size === 0) return null;
        return (
          <div>
            <div className={`${s.eyebrow} mb-1`}>Per-engine evidence</div>
            {nodeInfoLoading && (
              <div className={`${s.dim} ${s.smallText} mb-1`}>Loading neighbors…</div>
            )}
            <div className="d-flex flex-column gap-2">
              {[...engines].map((engine) => {
                const keys = node.statusesBySource?.[engine] ?? [];
                const neighbors = nodeInfo?.neighbors?.[engine];
                const inbound = neighbors?.In ?? [];
                const outbound = neighbors?.Out ?? [];
                const inCount  = distinctPeerCount(inbound, false);
                const outCount = distinctPeerCount(outbound, true);
                const hasDeny = keys.some((k) => {
                  const cfg = STATUS_CFG[k as keyof typeof STATUS_CFG];
                  return cfg && (cfg.severity === 'critical' || cfg.severity === 'high');
                });
                return (
                  <details key={engine} open={hasDeny || engines.size === 1}>
                    <summary className={s.disclosureSummary}>
                      <EngineBadge engine={engine} />
                      {inCount > 0 && (
                        <span className={`${s.countCapsule} ${s.countIn}`} title={`${inCount} inbound peer${inCount === 1 ? '' : 's'}`}>
                          in {inCount}
                        </span>
                      )}
                      {outCount > 0 && (
                        <span className={`${s.countCapsule} ${s.countOut}`} title={`${outCount} outbound peer${outCount === 1 ? '' : 's'}`}>
                          out {outCount}
                        </span>
                      )}
                      {node.statusesBySource?.[engine] && (
                        <span className={s.pushRight}>
                          <StatusBadges keys={keys} />
                        </span>
                      )}
                    </summary>
                    <div className="d-flex flex-column gap-2" style={{ marginTop: 8 }}>
                      {outbound.length > 0 && (
                        <div>
                          <div className={`${s.eyebrow} mb-1`}>Outbound ({outCount})</div>
                          <NeighborList neighbors={outbound} nodeIsSource nodeNamespace={node.namespace} />
                        </div>
                      )}
                      {inbound.length > 0 && (
                        <div>
                          <div className={`${s.eyebrow} mb-1`}>Inbound ({inCount})</div>
                          <NeighborList neighbors={inbound} nodeIsSource={false} nodeNamespace={node.namespace} />
                        </div>
                      )}
                    </div>
                  </details>
                );
              })}
            </div>
          </div>
        );
      })()}
    </div>
  );
}
