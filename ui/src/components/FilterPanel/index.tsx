// FilterPanel orchestrator. Toolbar across the top of the page composed of
// view controls (search/layout/refresh/view toggles) and five filter
// dropdowns (namespace/status/engine/action/direction). Each sub-component reads its
// own slice of the graph store directly.

import { ViewControls } from './parts/view-controls';
import {
  NamespaceDropdown,
  StatusDropdown,
  PolicySourceDropdown,
  ActionDropdown,
  DirectionDropdown,
  IssueTypeDropdown,
} from './parts/filter-dropdowns';

export default function FilterPanel() {
  // No overflow-x on this bar: an overflow container clips the absolutely
  // positioned filter dropdowns that hang below it, whatever their z-index.
  // flex-wrap already handles narrow widths.
  return (
    <div className="d-flex flex-wrap align-items-center gap-1 px-3 py-2 border-bottom bg-dark text-light flex-shrink-0 min-h-44 fs-13">
      <ViewControls />
      <NamespaceDropdown />
      <StatusDropdown />
      <PolicySourceDropdown />
      <ActionDropdown />
      <DirectionDropdown />
      <IssueTypeDropdown />
    </div>
  );
}
