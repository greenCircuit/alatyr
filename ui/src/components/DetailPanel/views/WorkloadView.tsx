// Workload-detail view: single-node click. Identity header + pin action,
// labels, effective status (hero), detected issues, per-engine evidence
// (status + selecting policies + outbound rules), and mesh membership.

import type { CSSProperties } from 'react';
import type { WorkloadNode, NodeDetail } from '../../../data/policies';
import { SEVERITY_COLOR } from '../../../data/policies';
import s from '../DetailPanel.module.css';
import { EngineBadge } from '../shared/badges';
import { NeighborList } from '../shared/rows';
import { distinctPeerCount } from '../shared/groupNeighborsByPolicy';
import { StatusBadges } from '../shared/status';
import { MeshCard } from '../shared/mesh';
import { NodeIssues } from '../shared/issues';

export function WorkloadView({ node, nodeInfo, nodeInfoLoading, canPin, onPin }: {
  node:            WorkloadNode;
  nodeInfo:        NodeDetail | null;
  nodeInfoLoading: boolean;
  canPin:          boolean;
  onPin:           (node: WorkloadNode) => void;
}) {
  return (
    <div className="d-flex flex-column gap-3">
      {/* Identity header: name as page title, namespace + type as
          subtitle. Pin-reachability is the headline action and sits
          next to the title — promoting it from middle-of-flow. */}
      <div className="d-flex flex-column gap-1">
        <div className="d-flex justify-content-between align-items-start gap-2">
          <div className="fs-5 fw-bold text-break">{node.label}</div>
          {canPin && (
            <button
              className="btn btn-sm btn-info flex-shrink-0"
              onClick={() => onPin(node)}
            >
              Pin as reachability source
            </button>
          )}
        </div>
        <div className="d-flex align-items-center gap-2 small">
          <span className="text-secondary">{node.namespace || '—'}</span>
          <span className="badge bg-secondary">{node.type}</span>
        </div>
      </div>

      <div>
        <div className="text-uppercase text-secondary small fw-semibold mb-1">Labels</div>
        <div className="d-flex flex-wrap gap-1">
          {Object.entries(node.labels ?? {}).map(([k, v]) => (
            <span key={k} className={`badge bg-secondary ${s.badgeSm}`}>{k}={v}</span>
          ))}
          {Object.keys(node.labels ?? {}).length === 0 && '—'}
        </div>
      </div>

      {/* Effective verdict — hero. Boxed so it reads as the answer,
          not a label-value row. Per-engine cards below are evidence. */}
        <div>
          <div className="text-uppercase text-secondary small fw-semibold">Effective status</div>
          <StatusBadges keys={node.statuses ?? []} />
        </div>
      {nodeInfo?.issues && nodeInfo.issues.length > 0 && (
        <div
          className={`border rounded p-2 d-flex flex-column gap-1 ${s.calloutAccent}`}
          style={{ '--accent': SEVERITY_COLOR.warning, borderColor: SEVERITY_COLOR.warning } as CSSProperties}
        >
          <div className={`fw-semibold text-uppercase ${s.calloutAccentText}`}>
            ⚠ {nodeInfo.issues.length} issue{nodeInfo.issues.length > 1 ? 's' : ''} detected
          </div>
          <ul className={`ps-3 mb-0 ${s.smallText} text-light`}>
            {nodeInfo.issues.map((issue, i) => <li key={i}>{issue}</li>)}
          </ul>
        </div>
      )}

      {/* Cluster findings touching this node (conflicts, lockouts) — the same
          issues the drawer/table list, scoped here so the operator sees them
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
            <div className="text-uppercase text-secondary small fw-semibold mb-1">Per-engine evidence</div>
            {nodeInfoLoading && (
              <div className="text-secondary small mb-1">Loading neighbors…</div>
            )}
            <div className="d-flex flex-column gap-2">
              {[...engines].map((engine) => {
                const keys = node.statusesBySource?.[engine] ?? [];
                const neighbors = nodeInfo?.neighbors?.[engine];
                const inbound = neighbors?.In ?? [];
                const outbound = neighbors?.Out ?? [];
                return (
                  <div key={engine} className="border border-secondary rounded p-2">
                    <EngineBadge engine={engine} />
                    {node.statusesBySource?.[engine] && <StatusBadges keys={keys} />}
                    {outbound.length > 0 && (
                      <div className="mt-3">
                        <div className="text-uppercase text-secondary small fw-semibold mb-1">Outbound ({distinctPeerCount(outbound, true)})</div>
                        <NeighborList neighbors={outbound} nodeIsSource nodeNamespace={node.namespace} />
                      </div>
                    )}
                    {inbound.length > 0 && (
                      <div className="mt-3">
                        <div className="text-uppercase text-secondary small fw-semibold mb-1">Inbound ({distinctPeerCount(inbound, false)})</div>
                        <NeighborList neighbors={inbound} nodeIsSource={false} nodeNamespace={node.namespace} />
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        );
      })()}

      {nodeInfo?.mesh && Object.keys(nodeInfo.mesh).length > 0 && (
        <div>
          <div className="text-uppercase text-secondary small fw-semibold mb-1">Mesh</div>
          <div className="d-flex flex-column gap-2">
            {Object.entries(nodeInfo.mesh).map(([source, m]) => (
              <MeshCard key={source} source={source} membership={m} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
