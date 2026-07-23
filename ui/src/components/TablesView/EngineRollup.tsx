// Engine rollup strip above the Policies table. Per-engine count of unique
// policies (post other filters, ignoring the engine filter itself). Clicking
// a chip toggles `selectedPolicySources` — same store action the FilterPanel
// dropdown uses, so the two views stay in sync.

import { useMemo } from 'react';
import type { PolicyEdge } from '../../data/policies';
import { engineMeta } from '../../data/engines';
import { EngineLogo } from '../../data/engineIcons';
import chip from '../../data/engineIcons.module.css';
import r from './Rollup.module.css';
import { useGraphStore } from '../../store/graphStore';

interface EngineRollupProps {
  edges: PolicyEdge[];
  // See StatusRollup — drop strip chrome + inline label when hosted inside a
  // section that already provides them.
  bare?: boolean;
}

export default function EngineRollup({ edges, bare = false }: EngineRollupProps) {
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
      <div className={bare ? 'text-secondary fs-12' : 'px-3 py-2 border-bottom border-secondary text-secondary fs-12'}>
        No policies across the current filter set.
      </div>
    );
  }

  const wrapperClass = bare
    ? 'd-flex align-items-center flex-wrap gap-2 fs-12'
    : 'd-flex align-items-center flex-wrap gap-2 px-3 py-2 border-bottom border-secondary fs-12';

  return (
    <div className={wrapperClass}>
      {!bare && (
        <span className="text-secondary fs-11 text-uppercase tracking-wide">
          Engine
        </span>
      )}
      {engines.map((engine) => {
        const { label, color } = engineMeta(engine);
        const count = counts.get(engine) ?? 0;
        const active = selectedPolicySources.has(engine);
        const dimmed = !active && selectedPolicySources.size > 0;
        return (
          <button
            key={engine}
            type="button"
            className={`btn btn-sm d-inline-flex align-items-center gap-1 p-1 ${chip.engineChip} ${r.chip} ${active ? r.active : ''} ${dimmed ? r.dimmed : ''}`}
            onClick={() => togglePolicySource(engine)}
            title={`${label} — click to ${active ? 'hide' : 'show only'} ${engine}`}
            style={{ border: `1px solid ${color}` }}
          >
            <EngineLogo engine={engine} size={10} />
            <span>{engine}</span>
            <span className="chip-count ms-1">{count}</span>
          </button>
        );
      })}
    </div>
  );
}
