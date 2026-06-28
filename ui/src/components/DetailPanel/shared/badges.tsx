// Leaf presentation primitives shared by every DetailPanel view: engine
// provenance chip, direction badge, and the L7 matcher block. No policy/edge
// domain logic here — just badges keyed off a string or a small struct.

import type { L7Match } from '../../../data/policies';
import { engineMeta } from '../../../data/engines';
import { EngineLogo } from '../../../data/engineIcons';
import { DIR_COLOR } from './presentation';

// Engine provenance chip — brand logo + name. Shared by the edge panel and the
// workload panel's per-engine cards so the same brand-colored mark that rides
// the arrows also labels the panels. Border (not fill) carries the brand color
// so the logo keeps its own color on the dark chip.
export function EngineBadge({ engine }: { engine: string }) {
  const { label, color } = engineMeta(engine);
  return (
    <span
      className="badge d-inline-flex align-items-center gap-1"
      title={label}
      style={{ background: '#11151a', border: `1px solid ${color}`, color: '#e9ecef' }}
    >
      <EngineLogo engine={engine} size={12} /> {engine}
    </span>
  );
}

export function DirectionBadge({ direction }: { direction: string }) {
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
export function L7Block({ blocks }: { blocks: L7Match[] }) {
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
