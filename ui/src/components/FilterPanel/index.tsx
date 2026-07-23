// Two-row toolbar.
//   Row 1 (chrome + reset): view mode, search, display options, "Reset
//     filters" btn, refresh.
//   Row 2 (data filters): Issue first (loudest signal / most common debug
//     entry point), then Namespace, Status, Source, Action, Direction.
// No active-filter chip strip — each filter button already reflects its own
// narrowed state via warning/danger tint, so a separate summary strip was
// redundant.

import { useGraphStore } from '../../store/graphStore';
import { DisplayDropdown } from './parts/display-dropdown';
import { useFilterReset } from './parts/use-filter-reset';
import {
  NamespaceDropdown,
  StatusDropdown,
  PolicySourceDropdown,
  ActionDropdown,
  DirectionDropdown,
  IssueTypeDropdown,
  MeshDropdown,
} from './parts/filter-dropdowns';

export default function FilterPanel() {
  const {
    view, setView,
    searchQuery, setSearchQuery,
    loadGraph, loading,
  } = useGraphStore();

  const { activeCount, resetAll } = useFilterReset();
  const isGraph = view === 'graph';

  return (
    <div className="d-flex flex-column border-bottom bg-dark text-light flex-shrink-0 fs-13">
      {/* Row 1 — workspace chrome + reset filters */}
      <div className="d-flex flex-wrap align-items-center gap-1 px-3 py-2 border-bottom border-secondary min-h-44">
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
          <button
            type="button"
            className={`btn ${view === 'status' ? 'btn-warning' : 'btn-outline-secondary'}`}
            onClick={() => setView('status')}
            title="Cluster status — posture and counts at a glance"
          >
            ▦ Status
          </button>
        </div>

        <input
          type="text"
          className="form-control form-control-sm bg-dark text-light border-secondary flex-shrink-0"
          style={{ width: 180 }}
          placeholder="Search by node name…"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />

        {isGraph && <DisplayDropdown />}

        <button
          type="button"
          className={`btn btn-sm ${activeCount > 0 ? 'btn-outline-danger' : 'btn-outline-secondary'} text-nowrap ms-auto`}
          onClick={resetAll}
          disabled={activeCount === 0}
          title={activeCount === 0
            ? 'No filters narrowed'
            : `Reset ${activeCount} narrowed filter${activeCount === 1 ? '' : 's'} to show everything`}
        >
          ✕ Reset filters{activeCount > 0 ? ` (${activeCount})` : ''}
        </button>

        <button
          className="btn btn-sm btn-outline-secondary text-nowrap"
          onClick={() => loadGraph()}
          disabled={loading}
          title="Refresh data"
        >
          ↻&nbsp;{loading ? 'Loading…' : 'Refresh'}
        </button>
      </div>

      {/* Row 2 — data filters (Issue first: loudest signal + most common entry point) */}
      <div className="d-flex flex-wrap align-items-center gap-1 px-3 py-2 min-h-44">
        <IssueTypeDropdown />
        <NamespaceDropdown />
        <StatusDropdown />
        <PolicySourceDropdown />
        <ActionDropdown />
        <DirectionDropdown />
        <MeshDropdown />
      </div>
    </div>
  );
}
