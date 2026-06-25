// Single source of truth for policy-engine presentation (provenance).
// Used by: the engine-icon overlay on graph arrows, the edge detail panel,
// and the per-engine cards in the workload panel. Adding an engine = one entry
// here; everything downstream reads from engineMeta so glyphs/colors stay
// consistent. Glyphs are unicode (no served assets) so the bundle stays
// self-contained when the UI is embedded into the Go binary.

export interface EngineMeta {
  label: string;  // human name for tooltips / panel headers
  color: string;  // brand color for the logo + chip border
}

const ENGINE_META: Record<string, EngineMeta> = {
  k8s:   { label: 'Kubernetes NetworkPolicy', color: '#326ce5' },
  istio: { label: 'Istio AuthorizationPolicy', color: '#466bb0' },
};

// engineMeta resolves a PolicySource.Name() to its presentation. Unknown
// engines fall back to a neutral brand color so a newly-wired backend engine
// still renders (with an initials logo) without a frontend change.
export function engineMeta(name: string): EngineMeta {
  return ENGINE_META[name] ?? { label: name, color: '#6c757d' };
}
