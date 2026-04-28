import { useEffect, useRef, useState } from 'react';
import { namespaces } from '../data/policies';
import { useGraphStore } from '../store/graphStore';

const NODE_TYPES = [
  { key: 'service',    label: 'Workload + svc' },
  { key: 'deployment', label: 'Deploy (no svc)' },
  { key: 'headless',   label: 'Headless' },
  { key: 'external',   label: 'External' },
];

export default function FilterPanel() {
  const {
    selectedNamespaces, selectedNodeTypes, searchQuery, showNamespaceEdges,
    toggleNamespace, toggleNodeType, setSearchQuery, toggleNamespaceEdges,
  } = useGraphStore();

  const [nsOpen, setNsOpen] = useState(false);
  const nsDropdownRef = useRef<HTMLDivElement>(null);

  // Close namespace dropdown on outside click
  useEffect(() => {
    if (!nsOpen) return;
    const handler = (e: MouseEvent) => {
      if (nsDropdownRef.current && !nsDropdownRef.current.contains(e.target as Node))
        setNsOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [nsOpen]);

  const nsCount   = selectedNamespaces.size;
  const nsLabel   = nsCount === namespaces.length
    ? 'All namespaces'
    : nsCount === 0
      ? 'No namespaces'
      : `${nsCount} / ${namespaces.length} namespaces`;

  return (
    <div
      className="d-flex align-items-center gap-3 px-3 border-bottom bg-dark text-light"
      style={{ flexShrink: 0, flexWrap: 'wrap', minHeight: 44, fontSize: 13 }}
    >
      <span
        className="fw-bold text-secondary me-1"
        style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.08em', whiteSpace: 'nowrap' }}
      >
        NetPol Visualizer
      </span>

      {/* Search */}
      <input
        type="text"
        className="form-control form-control-sm bg-dark text-light border-secondary"
        placeholder="Search nodes…"
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
        style={{ width: 160 }}
      />

      {/* Namespace multiselect dropdown */}
      <div className="position-relative" ref={nsDropdownRef}>
        <button
          className={`btn btn-sm ${nsCount < namespaces.length ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
          type="button"
          onClick={() => setNsOpen((o) => !o)}
        >
          {nsLabel}
        </button>

        {nsOpen && (
          <div
            className="position-absolute bg-dark border border-secondary rounded shadow p-2"
            style={{ top: '100%', left: 0, marginTop: 4, zIndex: 100, minWidth: 200 }}
          >
            {/* Select all / clear row */}
            <div className="d-flex gap-2 mb-2 pb-1 border-bottom border-secondary">
              <button
                className="btn btn-link btn-sm p-0 text-secondary"
                style={{ fontSize: 11 }}
                onClick={() => namespaces.forEach((ns) => { if (!selectedNamespaces.has(ns)) toggleNamespace(ns); })}
              >
                all
              </button>
              <button
                className="btn btn-link btn-sm p-0 text-secondary"
                style={{ fontSize: 11 }}
                onClick={() => namespaces.forEach((ns) => { if (selectedNamespaces.has(ns)) toggleNamespace(ns); })}
              >
                none
              </button>
            </div>

            {namespaces.map((ns) => (
              <div key={ns} className="form-check mb-1">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id={`ns-dd-${ns}`}
                  checked={selectedNamespaces.has(ns)}
                  onChange={() => toggleNamespace(ns)}
                />
                <label className="form-check-label text-light" htmlFor={`ns-dd-${ns}`} style={{ fontSize: 13 }}>
                  {ns}
                </label>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Node type filters */}
      <div className="d-flex align-items-center gap-1 flex-wrap">
        <span className="text-secondary me-1" style={{ fontSize: 11, whiteSpace: 'nowrap' }}>Types:</span>
        {NODE_TYPES.map(({ key, label }) => (
          <div key={key} className="form-check form-check-inline mb-0">
            <input
              className="form-check-input"
              type="checkbox"
              id={`type-${key}`}
              checked={selectedNodeTypes.has(key)}
              onChange={() => toggleNodeType(key)}
            />
            <label className="form-check-label text-light" htmlFor={`type-${key}`} style={{ fontSize: 12 }}>
              {label}
            </label>
          </div>
        ))}
      </div>

      {/* Namespace-level edges toggle */}
      <div className="form-check form-switch mb-0">
        <input
          className="form-check-input"
          type="checkbox"
          id="edge-ns"
          checked={showNamespaceEdges}
          onChange={toggleNamespaceEdges}
        />
        <label className="form-check-label text-light" htmlFor="edge-ns" style={{ fontSize: 12, whiteSpace: 'nowrap' }}>
          <span style={{ color: '#e67e22' }}>╌</span> NS edges
        </label>
      </div>
    </div>
  );
}
