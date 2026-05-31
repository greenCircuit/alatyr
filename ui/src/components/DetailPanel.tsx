import { useGraphStore } from '../store/graphStore';
import type {
  PolicyEdge,
  StatusKey,
  L7Match,
  NodeRule,
  PolicyRef,
  ReachabilityResult,
  EngineVerdict,
  DirectionVerdict,
  WorkloadNode,
} from '../data/policies';
import { formatPort, realPorts, STATUS_CFG, SEVERITY_COLOR } from '../data/policies';
import s from './DetailPanel.module.css';

// Match graph arrow colors so panel → canvas is a visual hand-off.
// Same hex values as style/edgeStyles.ts.
const DIR_COLOR: Record<string, { tint: string; arrow: string }> = {
  egress:  { tint: '#4dabf7', arrow: '↑' },
  ingress: { tint: '#f783ac', arrow: '↓' },
  both:    { tint: '#a9e34b', arrow: '↕' },
};

function DirectionBadge({ direction }: { direction: string }) {
  const { tint, arrow } = DIR_COLOR[direction] ?? { tint: '#adb5bd', arrow: '' };
  return (
    <span className="badge" style={{ background: tint, color: '#1a1d20' }}>
      {arrow} {direction}
    </span>
  );
}

// L7 matchers contribute six possible field sets (hosts/methods/paths +
// their notX exclusions). Render only the ones a policy actually populated
// so empty rules don't litter the panel.
function L7Block({ blocks }: { blocks: L7Match[] }) {
  const nonEmpty = blocks.filter(
    (b) =>
      (b.hosts?.length ?? 0) +
        (b.methods?.length ?? 0) +
        (b.paths?.length ?? 0) +
        (b.notHosts?.length ?? 0) +
        (b.notMethods?.length ?? 0) +
        (b.notPaths?.length ?? 0) >
      0,
  );
  if (nonEmpty.length === 0) return null;

  const fieldRow = (label: string, items?: string[], negate = false) => {
    if (!items || items.length === 0) return null;
    return (
      <div className="d-flex flex-column align-items-start gap-1 mt-1">
        <div className="text-secondary">{negate ? `not ${label}` : label}</div>
        <div className="d-flex flex-wrap gap-1">
          {items.map((v, i) => (
            <span
              key={i}
              className={`badge ${negate ? 'bg-danger' : 'bg-primary'} text-light`}
            >
              {v}
            </span>
          ))}
        </div>
      </div>
    );
  };

  return (
    <div className="mt-2">
      <div className="text-secondary">L7</div>
      {nonEmpty.map((block, blockIndex) => (
        <div
          key={blockIndex}
          className="border border-secondary rounded p-2 mt-1"
        >
          {fieldRow('hosts', block.hosts)}
          {fieldRow('methods', block.methods)}
          {fieldRow('paths', block.paths)}
          {fieldRow('hosts', block.notHosts, true)}
          {fieldRow('methods', block.notMethods, true)}
          {fieldRow('paths', block.notPaths, true)}
        </div>
      ))}
    </div>
  );
}

// Badges reuse the same color/symbol from STATUS_CFG so the detail panel,
// graph node overlays, and (future) legend stay in lockstep.
function StatusBadges({ keys }: { keys: StatusKey[] }) {
  if (keys.length === 0) return <span className="text-secondary">—</span>;
  return (
    <div className="d-flex flex-wrap gap-1 mt-1">
      {keys.map((key) => {
        const cfg = STATUS_CFG[key];
        const bg = SEVERITY_COLOR[cfg.severity];
        const title = `${cfg.severity}: ${cfg.description}`;
        return (
          <span
            key={key}
            title={title}
            className={`${s.statusBadge}`}
            style={{ background: bg }}
          >
            <span className={s.statusBadgeSymbol}>{cfg.symbol}</span>
            <span>{key}</span>
          </span>
        );
      })}
    </div>
  );
}

function PolicyRow({ p }: { p: PolicyEdge }) {
  const isDeny = p.action === 1;
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-2">
        <div className="fw-semibold text-light text-break">{p.policyName}</div>
        <div className="d-flex gap-1 flex-shrink-0">
          {isDeny && <span className="badge bg-danger">deny</span>}
          <span className={`badge ${p.level === 'namespace' ? 'bg-warning text-dark' : 'bg-secondary'}`}>
            {p.level}
          </span>
        </div>
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Engine:</div>
        <div>{p.policySource}</div>
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Namespace:</div>
        <div>{p.namespace}</div>
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Direction:</div>
        <DirectionBadge direction={p.direction} />
      </div>
      <div>
        <div className="text-secondary">Ports</div>
        <div className="d-flex flex-column align-items-start gap-1 mt-1">
          {realPorts(p.ports)?.map((pt, i) => (
            <span key={i} className="badge bg-info text-dark">{formatPort(pt)}/{pt.protocol}</span>
          )) ?? <div>all ports</div>}
        </div>
      </div>
      {p.l7Matches && <L7Block blocks={p.l7Matches} />}
    </div>
  );
}

function RuleRow({ rule }: { rule: NodeRule }) {
  const isDeny = rule.action === 1;
  const ports = realPorts([rule.port]);
  const dstLabel = rule.dstLabel || rule.dstId; // fall back to raw id for CIDR / unresolved
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className="fw-semibold text-light text-break">
          Workload: → {dstLabel}
          {rule.dstNamespace && <span className="text-secondary"> / {rule.dstNamespace}</span>}
        </div>
        {isDeny && <span className="badge bg-danger flex-shrink-0">deny</span>}
      </div>
      <div className={`${s.fieldRow} mb-1`}>
        <div className={s.fieldLabel}>Direction:</div>
        <DirectionBadge direction={rule.direction} />
      </div>
      <div>
        <div className="text-secondary">Ports</div>
        <div className="d-flex flex-column align-items-start gap-1 mt-1">
          {ports?.map((pt, i) => (
            <span key={i} className="badge bg-info text-dark">{formatPort(pt)}/{pt.protocol}</span>
          )) ?? <div>all ports</div>}
        </div>
      </div>
      {rule.l7Match && <L7Block blocks={[rule.l7Match]} />}
      {rule.contributors && rule.contributors.length > 0 && (
        <div className="mt-2">
          <div className="text-secondary">From policy</div>
          {rule.contributors.map((policyRef, i) => (
            <div key={i} className="text-light">
              {policyRef.name}<span className="text-secondary"> / {policyRef.namespace}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function PolicyRefRow({ policyRef }: { policyRef: PolicyRef }) {
  return (
    <div className={`border border-secondary rounded p-2 mb-2 ${s.smallText}`}>
      <div className="d-flex justify-content-between align-items-start gap-2 mb-1">
        <div className="fw-semibold text-light text-break">{policyRef.name}</div>
        <div className="d-flex gap-1 flex-shrink-0">
          {policyRef.action === 'deny' && <span className="badge bg-danger">deny</span>}
          {policyRef.action === 'allow' && <span className="badge bg-success">allow</span>}
          {policyRef.direction && <DirectionBadge direction={policyRef.direction} />}
        </div>
      </div>
      <div className={s.fieldRow}>
        <div className={s.fieldLabel}>Namespace:</div>
        <div>{policyRef.namespace}</div>
      </div>
    </div>
  );
}

function verdictColor(verdict: string): string {
  if (verdict === 'allow') return SEVERITY_COLOR.secure;
  if (verdict === 'deny')  return SEVERITY_COLOR.high;
  return SEVERITY_COLOR.caution;
}

function DirectionBlock({ label, dir, direction }: {
  label:     string;
  dir:       DirectionVerdict;
  direction: 'egress' | 'ingress';
}) {
  const { tint, arrow } = DIR_COLOR[direction];
  const allowCount = dir.allowMatches?.length ?? 0;
  const denyCount  = dir.denyMatches?.length  ?? 0;
  return (
    <div
      className="rounded p-2 mb-2 border border-secondary"
      style={{ borderLeftColor: tint, borderLeftWidth: 4 }}
    >
      <div className="d-flex justify-content-between align-items-center mb-1 flex-wrap gap-1">
        <span className="fw-semibold" style={{ color: tint }}>{arrow} {label}</span>
        <div className="d-flex gap-1 flex-wrap">
          {allowCount > 0 && (
            <span className="badge" style={{ background: SEVERITY_COLOR.secure }}>
              {allowCount} allow
            </span>
          )}
          {denyCount > 0 && (
            <span className="badge bg-danger">{denyCount} deny</span>
          )}
          {/* Surface "locked + no allow" as default-deny so it screams from the chip row
              instead of hiding inside the reason text. */}
          {dir.locked && allowCount === 0 && denyCount === 0
            ? <span className="badge bg-danger">default-deny</span>
            : dir.locked
              ? <span className="badge bg-warning text-dark">locked</span>
              : <span className="badge bg-secondary">open</span>}
        </div>
      </div>
      <div className={`text-secondary ${s.smallText} mb-1`}>{dir.reason}</div>
      {allowCount > 0 && (
        <div className="mt-2">
          <div className="text-success small">Allow rules</div>
          {dir.allowMatches!.map((rule, i) => <RuleRow key={`a-${i}`} rule={rule} />)}
        </div>
      )}
      {denyCount > 0 && (
        <div className="mt-2">
          <div className="text-danger small">Deny rules</div>
          {dir.denyMatches!.map((rule, i) => <RuleRow key={`d-${i}`} rule={rule} />)}
        </div>
      )}
    </div>
  );
}

function EngineCard({ name, ev }: { name: string; ev: EngineVerdict }) {
  const color = ev.status === 'allow' ? SEVERITY_COLOR.secure
              : ev.status === 'deny'  ? SEVERITY_COLOR.high
              : '#6c757d';
  return (
    <div
      className="border border-secondary rounded p-2 mb-2"
      style={{ borderLeftColor: color, borderLeftWidth: 5 }}
    >
      <div className="d-flex justify-content-between align-items-center mb-2">
        <span className="fw-semibold">{name}</span>
        <span className="badge text-light" style={{ background: color }}>{ev.status}</span>
      </div>
      <DirectionBlock label="Egress (src side)"  dir={ev.egress}  direction="egress" />
      <DirectionBlock label="Ingress (dst side)" dir={ev.ingress} direction="ingress" />
    </div>
  );
}

function PolicyRefList({ items }: { items?: PolicyRef[] }) {
  if (!items || items.length === 0) {
    return <div className={`text-secondary ${s.smallText}`}>none</div>;
  }
  return <div>{items.map((p, i) => <PolicyRefRow key={i} policyRef={p} />)}</div>;
}

function WorkloadColumn({ node, role, engines, policiesKey }: {
  node:        WorkloadNode;
  role:        'SRC' | 'DST';
  engines:     [string, EngineVerdict][];
  policiesKey: 'srcPolicies' | 'dstPolicies';
}) {
  const roleColor = role === 'SRC' ? 'bg-info' : 'bg-warning';
  return (
    <div className={s.reachCol}>
      <div className={s.reachColHeader}>
        <span className={`badge ${roleColor} text-dark me-2`}>{role}</span>
        <span className="text-break">{node.label}</span>
        <div className={`text-secondary ${s.smallText}`}>
          {node.namespace || '—'} · {node.type}
        </div>
      </div>
      <div className="d-flex flex-wrap gap-1 mb-3">
        {Object.entries(node.labels).map(([k, v]) => (
          <span key={k} className={`badge bg-secondary ${s.badgeSm}`}>{k}={v}</span>
        ))}
        {Object.keys(node.labels).length === 0 && (
          <span className={`text-secondary ${s.smallText}`}>no labels</span>
        )}
      </div>
      {engines.map(([name, ev]) => (
        <div key={name} className="border border-secondary rounded p-2 mb-2">
          <div className="fw-semibold mb-1">{name}</div>
          <div className={`text-secondary ${s.smallText} mb-1`}>Selecting policies</div>
          <PolicyRefList items={ev[policiesKey]} />
        </div>
      ))}
    </div>
  );
}

function ResultColumn({ result, engines }: {
  result:  ReachabilityResult;
  engines: [string, EngineVerdict][];
}) {
  return (
    <div className={s.reachCol}>
      <div className={s.reachColHeader}>
        <span
          className="badge text-light text-uppercase me-2"
          style={{ background: verdictColor(result.verdict) }}
        >
          {result.verdict}
        </span>
        verdict
      </div>
      <div className={`text-secondary mb-3 ${s.smallText}`}>{result.reason}</div>
      {engines.map(([name, ev]) => (
        <EngineCard key={name} name={name} ev={ev} />
      ))}
    </div>
  );
}

function ReachabilityPane({ src, dst, result }: {
  src:    WorkloadNode;
  dst:    WorkloadNode;
  result: ReachabilityResult;
}) {
  const engines = Object.entries(result.engines);
  return (
    <div className={s.reachGrid}>
      <WorkloadColumn node={src} role="SRC" engines={engines} policiesKey="srcPolicies" />
      <WorkloadColumn node={dst} role="DST" engines={engines} policiesKey="dstPolicies" />
      <ResultColumn result={result} engines={engines} />
    </div>
  );
}

export default function DetailPanel() {
  const {
    selectedNode, selectedEdges, nodeInfo, nodeInfoLoading,
    reachabilitySource, reachability, reachabilityLoading, reachabilityTarget,
    setSelectedNode, setSelectedEdges, pinReachabilitySource, clearReachability,
  } = useGraphStore();

  if (!selectedNode && selectedEdges.length === 0 && !reachabilitySource) return null;

  const close = () => { setSelectedNode(null); setSelectedEdges([]); clearReachability(); };
  // Wide layout kicks in once we have *something* reachability-related to render
  // (loading or result). Avoids stretching the panel before the user has clicked
  // the target node.
  const reachActive = !!reachabilitySource && (reachabilityLoading || !!reachability);
  const headerLabel = reachActive
    ? 'Reachability'
    : selectedNode
      ? 'Workload'
      : `${selectedEdges.length} polic${selectedEdges.length === 1 ? 'y' : 'ies'}`;

  return (
    <div className={`text-light border border-secondary rounded shadow ${s.panel} ${reachActive ? s.panelWide : ''}`}>
      <div className="d-flex justify-content-between align-items-center px-3 py-2 border-bottom border-secondary">
        <span className="fw-bold small">{headerLabel}</span>
        <button className="btn-close btn-close-white btn-sm" onClick={close} />
      </div>

      <div className="p-3">
        {reachabilitySource && (
          <div className="border border-info rounded p-2 mb-2">
            <div className="d-flex justify-content-between align-items-center">
              <div>
                <span className="badge bg-info text-dark me-2">SRC</span>
                <span className="fw-semibold">{reachabilitySource.label}</span>
                <span className="text-secondary"> / {reachabilitySource.namespace || '—'}</span>
              </div>
              <button className="btn btn-sm btn-outline-light" onClick={clearReachability}>
                Cancel
              </button>
            </div>
            <div className={`text-secondary mt-1 ${s.smallText}`}>
              {reachabilityTarget
                ? <>Target: {reachabilityTarget.label}</>
                : <>Click another node to check reachability</>}
            </div>
          </div>
        )}

        {reachabilityLoading && (
          <div className="text-secondary small mb-2">Computing reachability…</div>
        )}

        {reachability && reachabilitySource && reachabilityTarget && (
          <ReachabilityPane
            src={reachabilitySource}
            dst={reachabilityTarget}
            result={reachability}
          />
        )}

        {!reachActive && selectedNode && (
          <div className="d-flex flex-column gap-2">
            <div className={s.fieldRow}>
              <div className={s.fieldLabel}>Name:</div>
              <div className="fw-semibold text-break">{selectedNode.label}</div>
            </div>
            <div className={s.fieldRow}>
              <div className={s.fieldLabel}>Namespace:</div>
              <div>{selectedNode.namespace || '—'}</div>
            </div>
            <div className={s.fieldRow}>
              <div className={s.fieldLabel}>Type:</div>
              <div>{selectedNode.type}</div>
            </div>
            {!reachabilitySource && (
              <button
                className="btn btn-sm btn-outline-info align-self-start"
                onClick={() => pinReachabilitySource(selectedNode)}
              >
                Pin as reachability source
              </button>
            )}
            <div>
              <div className="text-secondary">Labels</div>
              <div className="d-flex flex-column align-items-start gap-1 mt-1">
                {Object.entries(selectedNode.labels).map(([k, v]) => (
                  <span key={k} className={`badge bg-secondary ${s.badgeSm}`}>{k}={v}</span>
                ))}
                {Object.keys(selectedNode.labels).length === 0 && '—'}
              </div>
            </div>

            <div>
              <div className="text-secondary">Status (effective)</div>
              <StatusBadges keys={selectedNode.statuses ?? []} />
            </div>

            {(() => {
              const engines = new Set<string>([
                ...Object.keys(selectedNode.statusesBySource ?? {}),
                ...Object.keys(nodeInfo ?? {}),
              ]);
              if (engines.size === 0) return null;
              return (
                <div>
                  <div className="text-secondary">By policy engine</div>
                  {nodeInfoLoading && (
                    <div className="text-secondary small mt-1">Loading rules + policies…</div>
                  )}
                  <div className="d-flex flex-column gap-2 mt-1">
                    {[...engines].map((engine) => {
                      const keys = selectedNode.statusesBySource?.[engine] ?? [];
                      const info = nodeInfo?.[engine];
                      return (
                        <div key={engine} className="border border-secondary rounded p-2 mt-4">
                          <div className={`fw-semibold`}>{engine}</div>
                          <StatusBadges keys={keys} />
                          {info?.policies && info.policies.length > 0 && (
                            <div className="mt-2">
                              <div className="mt-4">Selecting policies</div>
                              {info.policies.map((policyRef, i) => <PolicyRefRow key={i} policyRef={policyRef} />)}
                            </div>
                          )}
                          {info?.rules && info.rules.length > 0 && (
                            <div className="mt-2">
                              <div className="mt-4">Outbound rules</div>
                              {info.rules.map((rule, i) => <RuleRow key={i} rule={rule} />)}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              );
            })()}
          </div>
        )}

        {selectedEdges.length > 0 && (
          <>
            {selectedEdges.length > 1 && (
              <div className={`text-secondary mb-2 ${s.smallText}`}>
                {selectedEdges.length} policies on this connection
              </div>
            )}
            <div className={selectedEdges.length > 1 ? s.policyGrid : ''}>
              {selectedEdges.map((p) => <PolicyRow key={p.id} p={p} />)}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
