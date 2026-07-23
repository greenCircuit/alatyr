// Filter-reset hook. Returns how many filters are narrowed from their
// defaults, and one action that resets every narrowed filter back to
// "showing everything". Drives the "Reset filters" toolbar button and its
// disabled state.
//
// Default per filter:
//   Namespace / PolicySource / Action / Direction — all selected
//   Status / IssueType                            — empty selection

import { useGraphStore } from '../../../store/graphStore';

export function useFilterReset(): { activeCount: number; resetAll: () => void } {
  const {
    availableNamespaces, selectedNamespaces, toggleNamespace,
    selectedStatuses, toggleStatus,
    availablePolicySources, selectedPolicySources, togglePolicySource,
    selectedActions, toggleAction,
    selectedDirections, toggleDirection,
    selectedIssueTypes, toggleIssueType,
    selectedMeshFilters, toggleMeshFilter,
  } = useGraphStore();

  const resets: (() => void)[] = [];

  if (selectedNamespaces.size < availableNamespaces.length) {
    resets.push(() => availableNamespaces.forEach((ns) => {
      if (!selectedNamespaces.has(ns)) toggleNamespace(ns);
    }));
  }
  if (selectedStatuses.size > 0) {
    resets.push(() => Array.from(selectedStatuses).forEach(toggleStatus));
  }
  if (selectedPolicySources.size < availablePolicySources.length) {
    resets.push(() => availablePolicySources.forEach((src) => {
      if (!selectedPolicySources.has(src)) togglePolicySource(src);
    }));
  }
  if (selectedActions.size < 2) {
    resets.push(() => [0, 1].forEach((action) => {
      if (!selectedActions.has(action)) toggleAction(action);
    }));
  }
  if (selectedDirections.size < 2) {
    resets.push(() => ['ingress', 'egress'].forEach((dir) => {
      if (!selectedDirections.has(dir)) toggleDirection(dir);
    }));
  }
  if (selectedIssueTypes.size > 0) {
    resets.push(() => Array.from(selectedIssueTypes).forEach(toggleIssueType));
  }
  if (selectedMeshFilters.size > 0) {
    resets.push(() => Array.from(selectedMeshFilters).forEach(toggleMeshFilter));
  }

  return {
    activeCount: resets.length,
    resetAll: () => resets.forEach((fn) => fn()),
  };
}
