import { create } from 'zustand';

// Identity of the policy object whose YAML is shown in the (single) manifest
// drawer. One drawer total — opening a new manifest replaces the current one
// instead of stacking, so closing always dismisses cleanly.
export interface ManifestTarget {
  kind:       string;    // "k8s" | "istio" | "pa"
  namespace:  string;
  name:       string;
  // "key: value" lines to highlight in the YAML (the selectors relevant to this
  // trigger). `highlight` = this-node side (amber); `highlightPeer` = the other
  // endpoint's side (teal). When both omitted, the drawer falls back to the
  // selected node's labels on the amber class.
  highlight?:     string[];
  highlightPeer?: string[];
}

interface ManifestState {
  target: ManifestTarget | null;
  open:   (target: ManifestTarget) => void;
  close:  () => void;
}

export const useManifestStore = create<ManifestState>((set) => ({
  target: null,
  open:   (target) => set({ target }),
  close:  () => set({ target: null }),
}));
