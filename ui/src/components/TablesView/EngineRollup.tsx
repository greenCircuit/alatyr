// Engine rollup strip above the Policies table. Per-engine count of unique
// policies (post other filters, ignoring the engine filter itself). Clicking
// a chip toggles `selectedPolicySources` — same store action the FilterPanel
// dropdown uses, so the two views stay in sync.

import { useMemo } from 'react';
import type { PolicyEdge } from '../../data/policies';
import { engineMeta } from '../../data/engines';
import { EngineLogo } from '../../data/engineIcons';
import { useGraphStore } from '../../store/graphStore';

export default function EngineRollup({ edges }: { edges: PolicyEdge[] }) {
  const availablePolicySources = useGraphStore((s) => s.availablePolicySources);
  const selectedPolicySources  = useGraphStore((s) => s.selectedPolicySources);
  const togglePolicySource     = useGraphStore((s) => s.togglePolicySource);

  // Group by the same key as PoliciesTable rows so the rollup count matches
  // what a user would see in the table (rows, not raw edges).
  const counts = useMemo(() => {
    const seenPolicy = new Set<string>();
    const out = new Map<string, number>();
    for (const e of edges) {
      const policyKey = `${e.policySource}|${e.namespace}|${e.policyName}|${e.action ?? 0}`;
      if (seenPolicy.has(policyKey)) continue;
      seenPolicy.add(policyKey);
      out.set(e.policySource, (out.get(e.policySource) ?? 0) + 1);
    }
    return out;
  }, [edges]);

  // Preserve canonical engine order from cluster-state so the chip row is
  // stable across reloads regardless of which engines have data right now.
  const engines = availablePolicySources.filter((e) => (counts.get(e) ?? 0) > 0);

  if (engines.length === 0) {
    return (
      <div className="px-3 py-2 border-bottom border-secondary text-secondary" style={{ fontSize: 12 }}>
        No policies across the current filter set.
      </div>
    );
  }

  return (
    <div
      className="d-flex align-items-center flex-wrap gap-2 px-3 py-2 border-bottom border-secondary"
      style={{ fontSize: 12 }}
    >
      <span className="text-secondary" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.06em' }}>
        Engine
      </span>
      {engines.map((engine) => {
        const { label, color } = engineMeta(engine);
        const count = counts.get(engine) ?? 0;
        const active = selectedPolicySources.has(engine);
        const dimmed = !active && selectedPolicySources.size > 0;
        return (
          <button
            key={engine}
            type="button"
            className="btn btn-sm d-inline-flex align-items-center gap-1 p-1"
            onClick={() => togglePolicySource(engine)}
            title={`${label} — click to ${active ? 'hide' : 'show only'} ${engine}`}
            style={{
              background: '#11151a',
              border: `1px solid ${color}`,
              color: '#e9ecef',
              opacity: dimmed ? 0.45 : 1,
              outline: active ? `1.5px solid #fff` : 'none',
              outlineOffset: 1,
              fontSize: 12,
              lineHeight: 1,
            }}
          >
            <EngineLogo engine={engine} size={12} />
            <span>{engine}</span>
            <span className="badge bg-secondary ms-1">{count}</span>
          </button>
        );
      })}
    </div>
  );
}
