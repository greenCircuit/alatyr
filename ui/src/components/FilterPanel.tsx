import { useEffect, useRef, useState } from 'react';
import { useGraphStore } from '../store/graphStore';

const LAYOUTS = [
  { value: 'dagre',       label: 'Dagre (hierarchical)' },
  { value: 'fcose',       label: 'fCoSE (compound)' },
  { value: 'cola',        label: 'Cola (force)' },
  { value: 'cose',        label: 'CoSE (force)' },
  { value: 'breadthfirst', label: 'Breadth-first' },
  { value: 'grid',        label: 'Grid' },
  { value: 'circle',      label: 'Circle' },
];

export default function FilterPanel() {
  const {
    availableNamespaces: namespaces,
    selectedNamespaces, searchQuery, layoutAlgorithm,
    toggleNamespace, setSearchQuery, setLayoutAlgorithm,
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

      {/* Layout selector */}
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

    </div>
  );
}
