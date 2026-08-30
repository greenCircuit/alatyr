// Small floating popover anchored at an issue chip. Shows the 2-3 conflicts
// affecting a single row inline, without opening the full-cluster drawer.
// Edge-scoped issues route the operator into the reachability panel; node
// issues select the workload. Portalled to <body> so table overflow doesn't
// clip it; fixed positioning + clamping keeps it in the viewport.

import { useEffect, useRef, type CSSProperties } from 'react';
import { createPortal } from 'react-dom';
import { SEVERITY_COLOR } from '../../data/policies';
import type { Issue, WorkloadNode } from '../../data/policies';
import { useGraphStore } from '../../store/graphStore';
import { CulpritActions } from './CulpritActions';
import { issueSeverity, ISSUE_TYPE_LABEL } from '../FilterPanel/parts/constants';
import { EngineBadge } from '../../data/engineIcons';
import styles from './IssuesPopover.module.css';

const endpointLabel = (node?: WorkloadNode) => node?.label ?? node?.id ?? '';

const POPOVER_WIDTH  = 360;
const POPOVER_MAX_HEIGHT = 340;
const POPOVER_MIN_HEIGHT = 120;
const MARGIN = 12;
const CARET_GAP = 8;

// Position + size the popover based on remaining viewport space. If there's
// less than ~200px below the anchor and more above, flip to above-anchor so a
// row near the fold gets a full-height popover instead of a clipped one. Height
// is capped by whichever side we placed on so the popover never spills off
// screen.
function computePosition(anchor: DOMRect) {
  const spaceBelow = window.innerHeight - anchor.bottom - MARGIN;
  const spaceAbove = anchor.top - MARGIN;
  const flipAbove = spaceBelow < 200 && spaceAbove > spaceBelow;
  const roomOnPlacedSide = flipAbove ? spaceAbove : spaceBelow;
  const maxHeight = Math.max(POPOVER_MIN_HEIGHT, Math.min(POPOVER_MAX_HEIGHT, roomOnPlacedSide - CARET_GAP));
  const top = flipAbove
    ? Math.max(MARGIN, anchor.top - CARET_GAP - maxHeight)
    : Math.min(anchor.bottom + CARET_GAP, window.innerHeight - MARGIN - maxHeight);
  const left = Math.max(MARGIN, Math.min(anchor.left, window.innerWidth - MARGIN - POPOVER_WIDTH));
  // Caret centred on the anchor chip so the visual link to the row survives
  // both a horizontal clamp (chip against the viewport edge) and a flip.
  const caretLeft = Math.max(8, Math.min(POPOVER_WIDTH - 14, anchor.left + anchor.width / 2 - left - 6));
  return { top, left, maxHeight, flipAbove, caretLeft };
}

export function IssuesPopover({ anchor, issues, onClose }: {
  anchor:  DOMRect;
  issues:  Issue[];
  onClose: () => void;
}) {
  const setSelectedNode   = useGraphStore((s) => s.setSelectedNode);
  const showReachability  = useGraphStore((s) => s.showReachability);
  const selectPolicyByRef = useGraphStore((s) => s.selectPolicyByRef);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onDown = (event: MouseEvent) => {
      if (!ref.current) return;
      if (event.target instanceof Node && ref.current.contains(event.target)) return;
      onClose();
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    // Any scroll — table body, page, or a nested scroller — detaches the
    // popover from its anchor chip (fixed position doesn't follow the moving
    // row). Close so we never render a floater pointing at a chip that isn't
    // where it used to be. Capture-phase catches nested scrollers (Bootstrap's
    // overflow:auto containers don't bubble scroll events).
    const onScroll = (event: Event) => {
      if (ref.current && event.target instanceof Node && ref.current.contains(event.target)) return;
      onClose();
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    document.addEventListener('scroll', onScroll, true);
    window.addEventListener('resize', onClose);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('scroll', onScroll, true);
      window.removeEventListener('resize', onClose);
    };
  }, [onClose]);

  const { top, left, maxHeight, flipAbove, caretLeft } = computePosition(anchor);
  const style: CSSProperties = { position: 'fixed', top, left, width: POPOVER_WIDTH, maxHeight };

  const openIssue = (issue: Issue) => {
    if (issue.src && issue.dst) {
      showReachability(issue.src, issue.dst);
    } else if (issue.node) {
      setSelectedNode(issue.node);
    }
    onClose();
  };

  const openPolicy = (source: string, namespace: string, name: string) => {
    const selected = selectPolicyByRef(source, namespace, name);
    onClose();
    return selected;
  };

  return createPortal(
    <div ref={ref} className={styles.popover} style={style}>
      <span
        className={`${styles.caret} ${flipAbove ? styles.caretDown : styles.caretUp}`}
        style={{ left: caretLeft }}
        aria-hidden="true"
      />
      <div className={styles.header}>
        <span className={styles.title}>{issues.length} issue{issues.length === 1 ? '' : 's'}</span>
        <button type="button" className="btn-close btn-close-white btn-sm" aria-label="Close" onClick={onClose} />
      </div>
      <div className={styles.list}>
        {issues.map((issue, index) => {
          const severity = issueSeverity(issue);
          const isEdge = !!(issue.src && issue.dst);
          const primaryLabel = isEdge ? 'Open reachability' : 'Open workload';
          return (
            <div
              key={index}
              className={styles.item}
              style={{ ['--accent' as string]: SEVERITY_COLOR[severity] }}
            >
              <div className={styles.itemHead}>
                <span className={styles.typeTag}>{ISSUE_TYPE_LABEL[issue.type] ?? issue.type}</span>
                {(issue.engines?.length ? issue.engines : issue.engine ? [issue.engine] : []).map((engine) => (
                  <EngineBadge key={engine} engine={engine} />
                ))}
              </div>
              {isEdge ? (
                <>
                  <div className={styles.node}>
                    {endpointLabel(issue.src)} <span className="text-secondary">→</span> {endpointLabel(issue.dst)}
                  </div>
                  <div className={styles.message}>{issue.message}</div>
                </>
              ) : (
                <>
                  <div className={styles.node}>{issue.message}</div>
                  <div className={styles.endpointNs}>{endpointLabel(issue.node)}</div>
                </>
              )}
              <div className={styles.actions}>
                <button
                  type="button"
                  className="btn btn-sm btn-outline-light py-0 px-2 fs-11"
                  onClick={() => openIssue(issue)}
                >
                  {primaryLabel}
                </button>
              </div>
              <CulpritActions issue={issue} onOpen={openPolicy} />
            </div>
          );
        })}
      </div>
    </div>,
    document.body,
  );
}
