// "Filter dropdowns" feature: the four multi-select dropdowns that decide
// which data lands in the graph — namespaces, status keys, policy engines,
// and rule actions (allow/deny). All share the same shape (anchor button +
// dropdown panel with all/none links + checkbox list) so they live together.

import { useRef, useState } from 'react';
import { useGraphStore } from '../../../store/graphStore';
import type { StatusKey } from '../../../data/policies';
import { STATUS_CFG, SEVERITY_COLOR } from '../../../data/policies';
import { useOutsideClick } from './useOutsideClick';
import { STATUS_LABELS, ACTION_LABEL, DIRECTION_LABEL, ISSUE_TYPE_LABEL, ALL_ISSUE_TYPES } from './constants';
import { countIssuesByType } from '../../../store/issueIndex';
import styles from '../FilterPanel.module.css';

export function NamespaceDropdown() {
  const {
    availableNamespaces: namespaces,
    selectedNamespaces,
    toggleNamespace,
  } = useGraphStore();

  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const ref = useRef<HTMLDivElement>(null);
  useOutsideClick(ref, open, () => setOpen(false));

  const query = search.trim().toLowerCase();
  const visibleNamespaces = query === ''
    ? namespaces
    : namespaces.filter((ns) => ns.toLowerCase().includes(query));

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
        <div className="dropdown-shell dropdown-panel dropdown-panel-lg">
          <input
            type="text"
            autoFocus
            className="form-control form-control-sm bg-dark text-light border-secondary mb-2"
            placeholder="Search namespaces…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />

          <div className="d-flex gap-2 mb-2 pb-1 border-bottom border-secondary">
            <button
              className="btn btn-link btn-sm p-0 text-secondary fs-11"
              onClick={() => visibleNamespaces.forEach((ns) => { if (!selectedNamespaces.has(ns)) toggleNamespace(ns); })}
            >
              all
            </button>
            <button
              className="btn btn-link btn-sm p-0 text-secondary fs-11"
              onClick={() => visibleNamespaces.forEach((ns) => { if (selectedNamespaces.has(ns)) toggleNamespace(ns); })}
            >
              none
            </button>
          </div>

          {visibleNamespaces.length === 0 && (
            <div className={`text-secondary ${styles.noMatches}`}>No matches</div>
          )}

          {visibleNamespaces.map((ns) => (
            <div key={ns} className="form-check mb-1">
              <input
                className="form-check-input"
                type="checkbox"
                id={`ns-dd-${ns}`}
                checked={selectedNamespaces.has(ns)}
                onChange={() => toggleNamespace(ns)}
              />
              <label className="form-check-label text-light fs-13" htmlFor={`ns-dd-${ns}`}>
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

  // Match the Legend's badge order (STATUS_CFG insertion order), keeping only
  // keys the backend actually emits.
  const available = new Set(availableStatusKeys);
  const orderedStatusKeys = (Object.keys(STATUS_CFG) as StatusKey[]).filter((key) => available.has(key));

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
        <div className="dropdown-shell dropdown-panel dropdown-panel-lg">
          <div className="d-flex gap-2 mb-2 pb-1 border-bottom border-secondary">
            <button
              className="btn btn-link btn-sm p-0 text-secondary fs-11"
              onClick={() => availableStatusKeys.forEach((k) => { if (!selectedStatuses.has(k)) toggleStatus(k); })}
            >
              all
            </button>
            <button
              className="btn btn-link btn-sm p-0 text-secondary fs-11"
              onClick={() => availableStatusKeys.forEach((k) => { if (selectedStatuses.has(k)) toggleStatus(k); })}
            >
              none
            </button>
          </div>

          {orderedStatusKeys.map((key: StatusKey) => {
            const cfg = STATUS_CFG[key];
            return (
              <div key={key} className="form-check mb-1 d-flex align-items-center gap-2">
                <input
                  className="form-check-input m-0"
                  type="checkbox"
                  id={`status-dd-${key}`}
                  checked={selectedStatuses.has(key)}
                  onChange={() => toggleStatus(key)}
                />
                <label
                  className={`form-check-label text-light d-flex align-items-center gap-2 ${styles.statusLabel}`}
                  htmlFor={`status-dd-${key}`}
                >
                  <span
                    title={cfg.description}
                    className={styles.statusBadge}
                    style={{ background: SEVERITY_COLOR[cfg.severity] }}
                  >
                    {cfg.symbol}
                  </span>
                  {STATUS_LABELS[key]}
                </label>
              </div>
            );
          })}
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
        <div className="dropdown-shell dropdown-panel dropdown-panel-md">
          <div className="d-flex gap-2 mb-2 pb-1 border-bottom border-secondary">
            <button
              className="btn btn-link btn-sm p-0 text-secondary fs-11"
              onClick={() => availablePolicySources.forEach((src) => { if (!selectedPolicySources.has(src)) togglePolicySource(src); })}
            >
              all
            </button>
            <button
              className="btn btn-link btn-sm p-0 text-secondary fs-11"
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
              <label className="form-check-label text-light fs-13" htmlFor={`src-dd-${src}`}>
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
        <div className="dropdown-shell dropdown-panel dropdown-panel-sm">
          {[0, 1].map((action) => (
            <div key={action} className="form-check mb-1">
              <input
                className="form-check-input"
                type="checkbox"
                id={`action-dd-${action}`}
                checked={selectedActions.has(action)}
                onChange={() => toggleAction(action)}
              />
              <label className="form-check-label text-light fs-13" htmlFor={`action-dd-${action}`}>
                {ACTION_LABEL[action]}
              </label>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// Sole conflict entry point on the toolbar. Anchor surfaces "any conflicts?"
// at a glance; the dropdown is dual-purpose — a "Browse full list" link opens
// the drawer for detailed reading, and the type checkboxes narrow the tables
// view. Merged from a separate toolbar Issues button so operators aren't
// hunting for two affordances that answer the same question.
export function IssueTypeDropdown() {
  const {
    issues, issuesLoading,
    selectedIssueTypes, toggleIssueType,
    setIssuesDrawerOpen,
  } = useGraphStore();

  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useOutsideClick(ref, open, () => setOpen(false));

  const counts = countIssuesByType(issues);
  const total = issues.length;
  const selected = selectedIssueTypes.size;

  const anchorLabel = total === 0
    ? 'No conflicts'
    : selected === 0
      ? `${total} conflict${total === 1 ? '' : 's'}`
      : `${selected} conflict type${selected === 1 ? '' : 's'}`;

  const anchorClass = total === 0
    ? 'btn-outline-secondary'
    : selected > 0
      ? 'btn-outline-warning'
      : 'btn-outline-danger';

  return (
    <div className="position-relative" ref={ref}>
      <button
        className={`btn btn-sm ${anchorClass} dropdown-toggle`}
        type="button"
        onClick={() => setOpen((o) => !o)}
      >
        ⚠ {anchorLabel}
        {issuesLoading && (
          <span className="spinner-border spinner-border-sm ms-1 spinner-xs" role="status" />
        )}
      </button>

      {open && (
        <div className="dropdown-shell dropdown-panel dropdown-panel-xl">
          {/* Browse action is a separate line up top so the dropdown reads:
              "open reader" vs "narrow tables view" — two different tasks. */}
          <button
            type="button"
            className="btn btn-sm btn-outline-secondary w-100 mb-2 d-flex justify-content-between align-items-center fs-12"
            disabled={total === 0}
            onClick={() => { setIssuesDrawerOpen(true); setOpen(false); }}
          >
            <span>Browse full list</span>
            <span>↗</span>
          </button>

          <div className="text-secondary text-uppercase mb-1 fs-10 tracking-wide">
            Filter tables by type
          </div>

          <div className="d-flex gap-2 mb-2 pb-1 border-bottom border-secondary">
            <button
              className="btn btn-link btn-sm p-0 text-secondary fs-11"
              onClick={() => ALL_ISSUE_TYPES.forEach((t) => { if (!selectedIssueTypes.has(t)) toggleIssueType(t); })}
            >
              all
            </button>
            <button
              className="btn btn-link btn-sm p-0 text-secondary fs-11"
              onClick={() => ALL_ISSUE_TYPES.forEach((t) => { if (selectedIssueTypes.has(t)) toggleIssueType(t); })}
            >
              none
            </button>
          </div>

          {ALL_ISSUE_TYPES.map((type) => {
            const count = counts[type] ?? 0;
            return (
              <div key={type} className="form-check mb-1 d-flex align-items-center gap-2">
                <input
                  className="form-check-input m-0"
                  type="checkbox"
                  id={`issue-dd-${type}`}
                  checked={selectedIssueTypes.has(type)}
                  disabled={count === 0}
                  onChange={() => toggleIssueType(type)}
                />
                <label
                  className={`form-check-label d-flex align-items-center justify-content-between gap-2 flex-grow-1 fs-13 ${count === 0 ? 'text-secondary' : 'text-light'}`}
                  htmlFor={`issue-dd-${type}`}
                >
                  <span>{ISSUE_TYPE_LABEL[type]}</span>
                  <span className={`badge ${count === 0 ? 'bg-secondary' : 'bg-danger'}`}>{count}</span>
                </label>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function DirectionDropdown() {
  const { selectedDirections, toggleDirection } = useGraphStore();

  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useOutsideClick(ref, open, () => setOpen(false));

  const label = selectedDirections.size === 2
    ? 'Ingress + Egress'
    : selectedDirections.has('ingress')
      ? 'Ingress only'
      : selectedDirections.has('egress')
        ? 'Egress only'
        : 'No direction';

  return (
    <div className="position-relative" ref={ref}>
      <button
        className={`btn btn-sm ${selectedDirections.size < 2 ? 'btn-outline-warning' : 'btn-outline-secondary'} dropdown-toggle`}
        type="button"
        onClick={() => setOpen((o) => !o)}
      >
        {label}
      </button>

      {open && (
        <div className="dropdown-shell dropdown-panel dropdown-panel-sm">
          {['ingress', 'egress'].map((direction) => (
            <div key={direction} className="form-check mb-1">
              <input
                className="form-check-input"
                type="checkbox"
                id={`direction-dd-${direction}`}
                checked={selectedDirections.has(direction)}
                onChange={() => toggleDirection(direction)}
              />
              <label className="form-check-label text-light fs-13" htmlFor={`direction-dd-${direction}`}>
                {DIRECTION_LABEL[direction]}
              </label>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
