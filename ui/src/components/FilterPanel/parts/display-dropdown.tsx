// Display options dropdown — groups the four render-time graph controls
// (layout algorithm, aggregate by namespace, show connected namespaces,
// engine icons) behind one anchor so the toolbar stops sprouting a new
// button per toggle. Anchor counts how many toggles are on so operators can
// see "2 display options on" without opening it.

import { useRef, useState } from 'react';
import { useGraphStore } from '../../../store/graphStore';
import { useOutsideClick } from './useOutsideClick';
import { LAYOUTS } from './constants';

export function DisplayDropdown() {
  const {
    layoutAlgorithm, setLayoutAlgorithm,
    aggregateByNamespace, toggleAggregateByNamespace,
    showConnectedNamespaces, toggleConnectedNamespaces,
    showEngineIcons, toggleEngineIcons,
    showMeshOverlay, toggleMeshOverlay,
  } = useGraphStore();

  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useOutsideClick(ref, open, () => setOpen(false));

  const activeCount =
    (aggregateByNamespace ? 1 : 0) +
    (showConnectedNamespaces ? 1 : 0) +
    (showEngineIcons ? 1 : 0) +
    (showMeshOverlay ? 1 : 0);

  const label = activeCount === 0
    ? 'Display'
    : `Display (${activeCount})`;

  return (
    <div className="position-relative" ref={ref}>
      <button
        className={`btn btn-sm ${activeCount > 0 ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle text-nowrap`}
        type="button"
        onClick={() => setOpen((o) => !o)}
        title="Graph display options"
      >
        ⚙ {label}
      </button>

      {open && (
        <div className="dropdown-shell dropdown-panel dropdown-panel-lg">
          <div className="mb-2">
            <label className="text-secondary text-uppercase fs-10 tracking-wide d-block mb-1" htmlFor="display-layout">
              Layout
            </label>
            <select
              id="display-layout"
              className="form-select form-select-sm bg-dark text-light border-secondary"
              value={layoutAlgorithm}
              onChange={(e) => setLayoutAlgorithm(e.target.value)}
            >
              {LAYOUTS.map(({ value, label }) => (
                <option key={value} value={value}>{label}</option>
              ))}
            </select>
          </div>

          <div className="form-check mb-1">
            <input
              className="form-check-input"
              type="checkbox"
              id="display-ns-view"
              checked={aggregateByNamespace}
              onChange={toggleAggregateByNamespace}
            />
            <label className="form-check-label text-light fs-13" htmlFor="display-ns-view">
              ▣ NS view
              <div className="text-secondary fs-11">Collapse workloads into namespace nodes</div>
            </label>
          </div>

          <div className="form-check mb-1">
            <input
              className="form-check-input"
              type="checkbox"
              id="display-connections"
              checked={showConnectedNamespaces}
              onChange={toggleConnectedNamespaces}
            />
            <label className="form-check-label text-light fs-13" htmlFor="display-connections">
              ⇄ Connections
              <div className="text-secondary fs-11">Show namespaces connected to selected ones</div>
            </label>
          </div>

          <div className="form-check mb-1">
            <input
              className="form-check-input"
              type="checkbox"
              id="display-engine-icons"
              checked={showEngineIcons}
              onChange={toggleEngineIcons}
            />
            <label className="form-check-label text-light fs-13" htmlFor="display-engine-icons">
              ☸ Engine icons
              <div className="text-secondary fs-11">Show which policy engine produced each arrow</div>
            </label>
          </div>

          <div className="form-check">
            <input
              className="form-check-input"
              type="checkbox"
              id="display-mesh-overlay"
              checked={showMeshOverlay}
              onChange={toggleMeshOverlay}
            />
            <label className="form-check-label text-light fs-13" htmlFor="display-mesh-overlay">
              ⛨ Mesh overlay
              <div className="text-secondary fs-11">Color per-workload mesh + mTLS state on the graph</div>
            </label>
          </div>
        </div>
      )}
    </div>
  );
}
