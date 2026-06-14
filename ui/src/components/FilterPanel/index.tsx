// FilterPanel orchestrator. Toolbar across the top of the page composed of
// view controls (search/layout/refresh/view toggles) and four filter
// dropdowns (namespace/status/engine/action). Each sub-component reads its
// own slice of the graph store directly.

import { ViewControls } from './parts/view-controls';
import {
  NamespaceDropdown,
  StatusDropdown,
  PolicySourceDropdown,
  ActionDropdown,
} from './parts/filter-dropdowns';

export default function FilterPanel() {
  return (
    <div
      className="d-flex align-items-center gap-3 px-3 border-bottom bg-dark text-light"
      style={{ flexShrink: 0, flexWrap: 'wrap', minHeight: 44, fontSize: 13 }}
    >
      <ViewControls />
      <NamespaceDropdown />
      <StatusDropdown />
      <PolicySourceDropdown />
      <ActionDropdown />
    </div>
  );
}
