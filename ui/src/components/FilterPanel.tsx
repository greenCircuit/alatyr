import { useEffect, useRef, useState } from 'react';
import { useGraphStore } from '../store/graphStore';
import type { StatusKey } from '../data/policies';

const STATUS_LABELS: Record<StatusKey, string> = {
  'air-gapped':        'Air-gapped (deny-all)',
  'cross-namespace':   'Cross-namespace',
  'internet-egress':   'Internet egress',
  'internet-ingress':  'Internet ingress',
  'internet-full':     'Internet full (bi-directional)',
  'lan-egress':        'LAN egress',
  'lan-ingress':       'LAN ingress',
  'lan-full':          'LAN full (bi-directional)',
  'api-server-egress': 'API server egress',
  'ns-egress-access':  'NS egress access',
  'ns-ingress-access': 'NS ingress access',
  'ns-full-access':    'NS full access',
};

const LAYOUTS = [
  { value: 'dagre',       label: 'Dagre (hierarchical)' },
  { value: 'fcose',       label: 'fCoSE (compound)' },
  { value: 'cola',        label: 'Cola (force)' },
  { value: 'cose',        label: 'CoSE (force)' },
  { value: 'breadthfirst', label: 'Breadth-first' },
  { value: 'grid',        label: 'Grid' },
  { value: 'circle',      label: 'Circle' },
];

const ACTION_LABEL: Record<number, string> = { 0: 'Allow', 1: 'Deny' };

export default function FilterPanel() {
  const {
    availableNamespaces: namespaces,
    availableStatusKeys,
    availablePolicySources,
    selectedNamespaces, searchQuery, layoutAlgorithm,
    selectedStatuses, toggleStatus,
    selectedPolicySources, togglePolicySource,
    selectedActions, toggleAction,
    showConnectedNamespaces, toggleConnectedNamespaces,
    aggregateByNamespace, toggleAggregateByNamespace,
    toggleNamespace, setSearchQuery, setLayoutAlgorithm,
    loadGraph, loading,
  } = useGraphStore();

  const [nsOpen,     setNsOpen]     = useState(false);
  const [statusOpen, setStatusOpen] = useState(false);
  const [sourceOpen, setSourceOpen] = useState(false);
  const [actionOpen, setActionOpen] = useState(false);
  const nsDropdownRef     = useRef<HTMLDivElement>(null);
  const statusDropdownRef = useRef<HTMLDivElement>(null);
  const sourceDropdownRef = useRef<HTMLDivElement>(null);
  const actionDropdownRef = useRef<HTMLDivElement>(null);

  // Close dropdowns on outside click
  useEffect(() => {
    if (!nsOpen) return;
    const handler = (e: MouseEvent) => {
      if (nsDropdownRef.current && !nsDropdownRef.current.contains(e.target as Node))
        setNsOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [nsOpen]);

  useEffect(() => {
    if (!statusOpen) return;
    const handler = (e: MouseEvent) => {
      if (statusDropdownRef.current && !statusDropdownRef.current.contains(e.target as Node))
        setStatusOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [statusOpen]);

  useEffect(() => {
    if (!sourceOpen) return;
    const handler = (e: MouseEvent) => {
      if (sourceDropdownRef.current && !sourceDropdownRef.current.contains(e.target as Node))
        setSourceOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [sourceOpen]);

  useEffect(() => {
    if (!actionOpen) return;
    const handler = (e: MouseEvent) => {
      if (actionDropdownRef.current && !actionDropdownRef.current.contains(e.target as Node))
        setActionOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [actionOpen]);

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

      {/* Refresh */}
      <button
        className="btn btn-sm btn-outline-secondary"
        onClick={() => loadGraph()}
        disabled={loading}
        title="Refresh data"
        style={{ whiteSpace: 'nowrap' }}
      >
        ↻&nbsp;{loading ? 'Loading…' : 'Refresh'}
      </button>

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

      {/* NS aggregate view toggle */}
      <button
        className={`btn btn-sm ${aggregateByNamespace ? 'btn-warning' : 'btn-outline-secondary'}`}
        onClick={toggleAggregateByNamespace}
        title="Collapse all workloads into namespace nodes to see NS-to-NS policy flow"
        style={{ whiteSpace: 'nowrap' }}
      >
        ▣ NS view
      </button>

      {/* Show connected namespaces toggle */}
      <button
        className={`btn btn-sm ${showConnectedNamespaces ? 'btn-warning' : 'btn-outline-secondary'}`}
        onClick={toggleConnectedNamespaces}
        title="Also show namespaces connected to the selected ones"
        style={{ whiteSpace: 'nowrap' }}
      >
        ⇄ Connections
      </button>

      {/* Status key multiselect dropdown */}
      <div className="position-relative" ref={statusDropdownRef}>
        <button
          className={`btn btn-sm ${selectedStatuses.size > 0 ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
          type="button"
          onClick={() => setStatusOpen((o) => !o)}
        >
          {selectedStatuses.size === 0 ? 'Filter by status' : `${selectedStatuses.size} status filter${selectedStatuses.size > 1 ? 's' : ''}`}
        </button>

        {statusOpen && (
          <div
            className="position-absolute bg-dark border border-secondary rounded shadow p-2"
            style={{ top: '100%', left: 0, marginTop: 4, zIndex: 100, minWidth: 200 }}
          >
            <div className="d-flex gap-2 mb-2 pb-1 border-bottom border-secondary">
              <button
                className="btn btn-link btn-sm p-0 text-secondary"
                style={{ fontSize: 11 }}
                onClick={() => availableStatusKeys.forEach((k) => { if (!selectedStatuses.has(k)) toggleStatus(k); })}
              >
                all
              </button>
              <button
                className="btn btn-link btn-sm p-0 text-secondary"
                style={{ fontSize: 11 }}
                onClick={() => availableStatusKeys.forEach((k) => { if (selectedStatuses.has(k)) toggleStatus(k); })}
              >
                none
              </button>
            </div>

            {availableStatusKeys.map((key) => (
              <div key={key} className="form-check mb-1">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id={`status-dd-${key}`}
                  checked={selectedStatuses.has(key)}
                  onChange={() => toggleStatus(key)}
                />
                <label className="form-check-label text-light" htmlFor={`status-dd-${key}`} style={{ fontSize: 13 }}>
                  {STATUS_LABELS[key]}
                </label>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Policy source multiselect (engines: k8s, istio, ...) */}
      <div className="position-relative" ref={sourceDropdownRef}>
        <button
          className={`btn btn-sm ${selectedPolicySources.size < availablePolicySources.length ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
          type="button"
          onClick={() => setSourceOpen((o) => !o)}
        >
          {selectedPolicySources.size === availablePolicySources.length
            ? 'All engines'
            : selectedPolicySources.size === 0
              ? 'No engines'
              : `${selectedPolicySources.size} / ${availablePolicySources.length} engines`}
        </button>

        {sourceOpen && (
          <div
            className="position-absolute bg-dark border border-secondary rounded shadow p-2"
            style={{ top: '100%', left: 0, marginTop: 4, zIndex: 100, minWidth: 160 }}
          >
            <div className="d-flex gap-2 mb-2 pb-1 border-bottom border-secondary">
              <button
                className="btn btn-link btn-sm p-0 text-secondary"
                style={{ fontSize: 11 }}
                onClick={() => availablePolicySources.forEach((src) => { if (!selectedPolicySources.has(src)) togglePolicySource(src); })}
              >
                all
              </button>
              <button
                className="btn btn-link btn-sm p-0 text-secondary"
                style={{ fontSize: 11 }}
                onClick={() => availablePolicySources.forEach((src) => { if (selectedPolicySources.has(src)) togglePolicySource(src); })}
              >
                none
              </button>
            </div>

            {availablePolicySources.map((src) => (
              <div key={src} className="form-check mb-1">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id={`src-dd-${src}`}
                  checked={selectedPolicySources.has(src)}
                  onChange={() => togglePolicySource(src)}
                />
                <label className="form-check-label text-light" htmlFor={`src-dd-${src}`} style={{ fontSize: 13 }}>
                  {src}
                </label>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Action multiselect (allow / deny) */}
      <div className="position-relative" ref={actionDropdownRef}>
        <button
          className={`btn btn-sm ${selectedActions.size < 2 ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
          type="button"
          onClick={() => setActionOpen((o) => !o)}
        >
          {selectedActions.size === 2
            ? 'Allow + Deny'
            : selectedActions.has(0)
              ? 'Allow only'
              : selectedActions.has(1)
                ? 'Deny only'
                : 'None'}
        </button>

        {actionOpen && (
          <div
            className="position-absolute bg-dark border border-secondary rounded shadow p-2"
            style={{ top: '100%', left: 0, marginTop: 4, zIndex: 100, minWidth: 140 }}
          >
            {[0, 1].map((action) => (
              <div key={action} className="form-check mb-1">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id={`action-dd-${action}`}
                  checked={selectedActions.has(action)}
                  onChange={() => toggleAction(action)}
                />
                <label className="form-check-label text-light" htmlFor={`action-dd-${action}`} style={{ fontSize: 13 }}>
                  {ACTION_LABEL[action]}
                </label>
              </div>
            ))}
          </div>
        )}
      </div>

    </div>
  );
}
