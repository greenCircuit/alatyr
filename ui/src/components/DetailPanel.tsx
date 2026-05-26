import { useGraphStore } from '../store/graphStore';
import type { PolicyEdge, StatusKey, L7Match } from '../data/policies';
import { formatPort, realPorts, STATUS_CFG, SEVERITY_COLOR } from '../data/policies';
import s from './DetailPanel.module.css';

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
        <div>{p.direction}</div>
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

export default function DetailPanel() {
  const { selectedNode, selectedEdges, setSelectedNode, setSelectedEdges } = useGraphStore();

  if (!selectedNode && selectedEdges.length === 0) return null;

  const close = () => { setSelectedNode(null); setSelectedEdges([]); };

  return (
    <div className={`text-light border border-secondary rounded shadow ${s.panel}`}>
      <div className="d-flex justify-content-between align-items-center px-3 py-2 border-bottom border-secondary">
        <span className="fw-bold small">
          {selectedNode
            ? 'Workload'
            : `${selectedEdges.length} polic${selectedEdges.length === 1 ? 'y' : 'ies'}`}
        </span>
        <button className="btn-close btn-close-white btn-sm" onClick={close} />
      </div>

      <div className="p-3">
        {selectedNode && (
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

            {selectedNode.statusesBySource && Object.keys(selectedNode.statusesBySource).length > 0 && (
              <div>
                <div className="text-secondary">By policy engine</div>
                <div className="d-flex flex-column gap-2 mt-1">
                  {Object.entries(selectedNode.statusesBySource).map(([engine, keys]) => (
                    <div key={engine} className="border border-secondary rounded p-2">
                      <div className={`fw-semibold text-light ${s.smallText}`}>{engine}</div>
                      <StatusBadges keys={keys} />
                    </div>
                  ))}
                </div>
              </div>
            )}
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
