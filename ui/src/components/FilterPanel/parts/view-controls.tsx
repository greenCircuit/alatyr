// "View controls" feature: the left half of the toolbar — title chip, search
// box, layout selector, refresh button, plus the two view-mode toggles
// (aggregate by namespace, show connected namespaces). These affect *how* the
// graph renders rather than *which* data lands in it.

import { useGraphStore } from '../../../store/graphStore';
import { LAYOUTS } from './constants';

export function ViewControls() {
  const {
    searchQuery, layoutAlgorithm,
    showConnectedNamespaces, toggleConnectedNamespaces,
    aggregateByNamespace, toggleAggregateByNamespace,
    showEngineIcons, toggleEngineIcons,
    setSearchQuery, setLayoutAlgorithm,
    loadGraph, loading,
  } = useGraphStore();

  return (
    <>
      <span
        className="fw-bold text-secondary me-1"
        style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.08em', whiteSpace: 'nowrap' }}
      >
        NetPol Visualizer
      </span>

      <input
        type="text"
        className="form-control form-control-sm bg-dark text-light border-secondary"
        placeholder="Search by node name…"
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
        style={{ width: 160 }}
      />

      <select
        className="form-select form-select-sm bg-dark text-light border-secondary"
        style={{ width: 170 }}
        value={layoutAlgorithm}
        onChange={(e) => setLayoutAlgorithm(e.target.value)}
      >
        {LAYOUTS.map(({ value, label }) => (
          <option key={value} value={value}>{label}</option>
        ))}
      </select>

      <button
        className="btn btn-sm btn-outline-secondary"
        onClick={() => loadGraph()}
        disabled={loading}
        title="Refresh data"
        style={{ whiteSpace: 'nowrap' }}
      >
        ↻&nbsp;{loading ? 'Loading…' : 'Refresh'}
      </button>

      <button
        className={`btn btn-sm ${aggregateByNamespace ? 'btn-warning' : 'btn-outline-secondary'}`}
        onClick={toggleAggregateByNamespace}
        title="Collapse all workloads into namespace nodes to see NS-to-NS policy flow"
        style={{ whiteSpace: 'nowrap' }}
      >
        ▣ NS view
      </button>

      <button
        className={`btn btn-sm ${showConnectedNamespaces ? 'btn-warning' : 'btn-outline-secondary'}`}
        onClick={toggleConnectedNamespaces}
        title="Also show namespaces connected to the selected ones"
        style={{ whiteSpace: 'nowrap' }}
      >
        ⇄ Connections
      </button>

      <button
        className={`btn btn-sm ${showEngineIcons ? 'btn-warning' : 'btn-outline-secondary'}`}
        onClick={toggleEngineIcons}
        title="Show which policy engine produced each arrow"
        style={{ whiteSpace: 'nowrap' }}
      >
        ☸ Engine icons
      </button>
    </>
  );
}
