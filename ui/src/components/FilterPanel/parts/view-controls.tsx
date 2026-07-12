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
    view, setView,
  } = useGraphStore();

  return (
    <>

      <span className="fw-bold text-secondary me-1 fs-11 text-uppercase tracking-wider text-nowrap">
        NetPol Visualizer
      </span>

      <div className="btn-group btn-group-sm" role="group" aria-label="View mode">
        <button
          type="button"
          className={`btn ${view === 'graph' ? 'btn-warning' : 'btn-outline-secondary'}`}
          onClick={() => setView('graph')}
          title="Graph view — spatial reachability"
        >
          ◉ Graph
        </button>
        <button
          type="button"
          className={`btn ${view === 'tables' ? 'btn-warning' : 'btn-outline-secondary'}`}
          onClick={() => setView('tables')}
          title="Table view — audit, hygiene, search"
        >
          ▤ Tables
        </button>
      </div>
      <div>
        <input
          type="text"
          className="form-control form-control-sm bg-dark text-light border-secondary w-120"
          placeholder="Search by node name…"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
      </div>
      <div>
        <select
          className="form-select form-select-sm bg-dark text-light border-secondary w-130"
          value={layoutAlgorithm}
          onChange={(e) => setLayoutAlgorithm(e.target.value)}
        >
          {LAYOUTS.map(({ value, label }) => (
            <option key={value} value={value}>{label}</option>
          ))}
        </select>
      </div>
      <button
        className="btn btn-sm btn-outline-secondary text-nowrap"
        onClick={() => loadGraph()}
        disabled={loading}
        title="Refresh data"
      >
        ↻&nbsp;{loading ? 'Loading…' : 'Refresh'}
      </button>

      <button
        className={`btn btn-sm text-nowrap ${aggregateByNamespace ? 'btn-warning' : 'btn-outline-secondary'}`}
        onClick={toggleAggregateByNamespace}
        title="Collapse all workloads into namespace nodes to see NS-to-NS policy flow"
      >
        ▣ NS view
      </button>

      <button
        className={`btn btn-sm text-nowrap ${showConnectedNamespaces ? 'btn-warning' : 'btn-outline-secondary'}`}
        onClick={toggleConnectedNamespaces}
        title="Also show namespaces connected to the selected ones"
      >
        ⇄ Connections
      </button>

      <button
        className={`btn btn-sm text-nowrap ${showEngineIcons ? 'btn-warning' : 'btn-outline-secondary'}`}
        onClick={toggleEngineIcons}
        title="Show which policy engine produced each arrow"
      >
        ☸ Engine icons
      </button>
    </>
  );
}
