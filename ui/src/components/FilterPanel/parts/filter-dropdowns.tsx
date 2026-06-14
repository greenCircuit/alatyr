// "Filter dropdowns" feature: the four multi-select dropdowns that decide
// which data lands in the graph — namespaces, status keys, policy engines,
// and rule actions (allow/deny). All share the same shape (anchor button +
// dropdown panel with all/none links + checkbox list) so they live together.

import { useRef, useState } from 'react';
import { useGraphStore } from '../../../store/graphStore';
import type { StatusKey } from '../../../data/policies';
import { useOutsideClick } from './useOutsideClick';
import { STATUS_LABELS, ACTION_LABEL } from './constants';

export function NamespaceDropdown() {
  const {
    availableNamespaces: namespaces,
    selectedNamespaces,
    toggleNamespace,
  } = useGraphStore();

  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useOutsideClick(ref, open, () => setOpen(false));

  const nsCount = selectedNamespaces.size;
  const label = nsCount === namespaces.length
    ? 'All namespaces'
    : nsCount === 0
      ? 'No namespaces'
      : `${nsCount} / ${namespaces.length} namespaces`;

  return (
    <div className="position-relative" ref={ref}>
      <button
        className={`btn btn-sm ${nsCount < namespaces.length ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
        type="button"
        onClick={() => setOpen((o) => !o)}
      >
        {label}
      </button>

      {open && (
        <div
          className="position-absolute bg-dark border border-secondary rounded shadow p-2"
          style={{ top: '100%', left: 0, marginTop: 4, zIndex: 100, minWidth: 200 }}
        >
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
  );
}

export function StatusDropdown() {
  const {
    availableStatusKeys,
    selectedStatuses, toggleStatus,
  } = useGraphStore();

  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useOutsideClick(ref, open, () => setOpen(false));

  return (
    <div className="position-relative" ref={ref}>
      <button
        className={`btn btn-sm ${selectedStatuses.size > 0 ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
        type="button"
        onClick={() => setOpen((o) => !o)}
      >
        {selectedStatuses.size === 0 ? 'Filter by status' : `${selectedStatuses.size} status filter${selectedStatuses.size > 1 ? 's' : ''}`}
      </button>

      {open && (
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

          {availableStatusKeys.map((key: StatusKey) => (
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
  );
}

export function PolicySourceDropdown() {
  const {
    availablePolicySources,
    selectedPolicySources, togglePolicySource,
  } = useGraphStore();

  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useOutsideClick(ref, open, () => setOpen(false));

  return (
    <div className="position-relative" ref={ref}>
      <button
        className={`btn btn-sm ${selectedPolicySources.size < availablePolicySources.length ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
        type="button"
        onClick={() => setOpen((o) => !o)}
      >
        {selectedPolicySources.size === availablePolicySources.length
          ? 'All engines'
          : selectedPolicySources.size === 0
            ? 'No engines'
            : `${selectedPolicySources.size} / ${availablePolicySources.length} engines`}
      </button>

      {open && (
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
  );
}

export function ActionDropdown() {
  const {
    selectedActions, toggleAction,
  } = useGraphStore();

  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useOutsideClick(ref, open, () => setOpen(false));

  return (
    <div className="position-relative" ref={ref}>
      <button
        className={`btn btn-sm ${selectedActions.size < 2 ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
        type="button"
        onClick={() => setOpen((o) => !o)}
      >
        {selectedActions.size === 2
          ? 'Allow + Deny'
          : selectedActions.has(0)
            ? 'Allow only'
            : selectedActions.has(1)
              ? 'Deny only'
              : 'None'}
      </button>

      {open && (
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
  );
}
