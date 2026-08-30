// IssuesDrawer: left-anchored overlay listing the whole-cluster conflict scan
// (/api/issues). Opened from the toolbar Issues button. Rows are sorted
// danger-first and grouped under sticky Blocking/Warnings headers; a left
// accent bar carries severity pre-attentively. Clicking an edge-scoped issue
// opens the reachability panel (which engine + rule broke src→dst); a
// node-scoped issue selects the node.

import { useGraphStore } from '../../store/graphStore';
import { SEVERITY_COLOR, mergeIssuesByPair } from '../../data/policies';
import type { Issue, WorkloadNode } from '../../data/policies';
import { issueSeverity, issueTierOf, ISSUE_TYPE_LABEL, ALL_ISSUE_TYPES } from '../FilterPanel/parts/constants';
import { EngineBadge } from '../../data/engineIcons';
import styles from './IssuesDrawer.module.css';

const endpointLabel = (node?: WorkloadNode) => node?.label ?? node?.id ?? '';

export default function IssuesDrawer() {
  const {
    issues, issuesLoading, issuesDrawerOpen,
    setIssuesDrawerOpen, setSelectedNode, showReachability,
    availableNamespaces,
  } = useGraphStore();

  if (!issuesDrawerOpen) return null;

  // Merge same-pair rows first so counts here match the Issues tab — raw
  // issues arrive one per blocked direction and would double-count.
  // Three buckets, not two: info-class (partial access) is expected behavior;
  // bucketing it under Warnings would tell the operator their intentional
  // layering is a fault.
  const merged = mergeIssuesByPair(issues);
  // Within-bucket sort: ALL_ISSUE_TYPES defines the canonical danger-first
  // order (blocking classes first). Same-type rows fall back to endpoint
  // identity so repeat sessions don't reshuffle the list under the operator.
  const typeRank = new Map(ALL_ISSUE_TYPES.map((type, index) => [type, index]));
  const endpointKey = (issue: Issue) =>
    issue.src && issue.dst
      ? `${issue.src.namespace}/${issue.src.id}→${issue.dst.namespace}/${issue.dst.id}`
      : `${issue.node?.namespace ?? ''}/${issue.node?.id ?? ''}`;
  const sortBucket = (bucket: Issue[]) =>
    [...bucket].sort((left, right) => {
      const typeDelta = (typeRank.get(left.type) ?? 99) - (typeRank.get(right.type) ?? 99);
      if (typeDelta !== 0) return typeDelta;
      return endpointKey(left).localeCompare(endpointKey(right));
    });
  // Bucket by tier, not by a literal severity pair — 'critical'/'caution' must
  // land in blocking/warning, not fall through to informational.
  const blocking = sortBucket(merged.filter((issue) => issueTierOf(issue) === 'blocking'));
  const warnings = sortBucket(merged.filter((issue) => issueTierOf(issue) === 'warning'));
  const informational = sortBucket(merged.filter((issue) => issueTierOf(issue) === 'info'));

  // Edge-scoped issues (src+dst) open the reachability panel so the operator
  // sees which engine + rule broke the path. Node-scoped issues select the node.
  const openIssue = (issue: Issue) => {
    if (issue.src && issue.dst) {
      showReachability(issue.src, issue.dst);
    } else if (issue.node) {
      setSelectedNode(issue.node);
    }
  };

  const renderIssue = (issue: Issue, index: number) => {
    const severity = issueSeverity(issue);
    const isEdge = !!(issue.src && issue.dst);
    const srcNs = issue.src?.namespace;
    const dstNs = issue.dst?.namespace;
    const nodeNs = issue.node?.namespace;
    const fullEndpoint = isEdge ? `${issue.src?.id} → ${issue.dst?.id}` : issue.node?.id;

    return (
      <button
        key={`${issue.type}-${fullEndpoint}-${index}`}
        type="button"
        className={styles.item}
        style={{ ['--accent' as string]: SEVERITY_COLOR[severity] }}
        onClick={() => openIssue(issue)}
        title={fullEndpoint}
      >
        <div className={styles.itemHead}>
          <span className={styles.typeTag}>{ISSUE_TYPE_LABEL[issue.type] ?? issue.type}</span>
          {/* Merged rows carry engines[]; unmerged fall back to the single engine. */}
          {(issue.engines?.length ? issue.engines : issue.engine ? [issue.engine] : []).map((engine) => (
            <EngineBadge key={engine} engine={engine} />
          ))}
        </div>

        {isEdge ? (
          <>
            <div className={styles.node}>
              {endpointLabel(issue.src)}
              <span className="text-secondary mx-1">→</span>
              {endpointLabel(issue.dst)}
            </div>
            <div className={styles.endpointNs}>
              {srcNs === dstNs ? srcNs : `${srcNs} → ${dstNs}`}
            </div>
            <div className={styles.message}>{issue.message}</div>
          </>
        ) : (
          <>
            {/* node-scoped: the message is the headline, node id is context */}
            <div className={styles.node}>{issue.message}</div>
            <div className={styles.endpointNs}>
              {endpointLabel(issue.node)}{nodeNs ? ` · ${nodeNs}` : ''}
            </div>
          </>
        )}
      </button>
    );
  };

  return (
    <div className={styles.drawer}>
      <div className={styles.header}>
        <span className={styles.title}>⚠ Issues</span>
        <div className="d-flex align-items-center gap-1">
          {(blocking.length > 0 || warnings.length > 0 || informational.length > 0) && (
            <span
              className={`btn btn-sm btn-dark border ${blocking.length > 0 ? 'border-danger' : 'border-secondary'} py-0 px-2 d-inline-flex align-items-center gap-2 fs-11 fw-semibold leading-tight`}
              role="group"
              aria-label="Issue counts by tier"
              title={[
                blocking.length > 0 && `${blocking.length} blocking`,
                warnings.length > 0 && `${warnings.length} warn`,
                informational.length > 0 && `${informational.length} info`,
              ].filter(Boolean).join(' · ')}
            >
              {blocking.length > 0 && (
                <span className="d-inline-flex align-items-center gap-1" style={{ color: SEVERITY_COLOR.high }}>
                  <span aria-hidden="true">●</span>{blocking.length}
                </span>
              )}
              {warnings.length > 0 && (
                <span className="d-inline-flex align-items-center gap-1" style={{ color: SEVERITY_COLOR.warning }}>
                  <span aria-hidden="true">●</span>{warnings.length}
                </span>
              )}
              {informational.length > 0 && (
                <span className="d-inline-flex align-items-center gap-1" style={{ color: SEVERITY_COLOR.info }}>
                  <span aria-hidden="true">●</span>{informational.length}
                </span>
              )}
            </span>
          )}
          <button
            type="button"
            className="btn-close btn-close-white ms-1"
            aria-label="Close"
            onClick={() => setIssuesDrawerOpen(false)}
          />
        </div>
      </div>

      <div className={styles.list}>
        {issuesLoading && issues.length === 0 ? (
          <div className={styles.empty}>
            <span className="spinner-border spinner-border-sm text-secondary d-block mx-auto mb-2" role="status" />
            Scanning cluster policies…
          </div>
        ) : merged.length === 0 ? (
          <div className={styles.empty}>
            <div className="fs-22 mb-1" style={{ color: SEVERITY_COLOR.secure }}>✓</div>
            No conflicts found
            {availableNamespaces.length > 0 && (
              <div className="text-secondary fs-11">
                across {availableNamespaces.length} namespaces
              </div>
            )}
          </div>
        ) : (
          <>
            {blocking.length > 0 && (
              <>
                <div className={styles.sectionHead}>Blocking · {blocking.length}</div>
                {blocking.map(renderIssue)}
              </>
            )}
            {warnings.length > 0 && (
              <>
                <div className={styles.sectionHead}>Warnings · {warnings.length}</div>
                {warnings.map(renderIssue)}
              </>
            )}
            {informational.length > 0 && (
              <>
                <div className={styles.sectionHead} title="Expected behavior — ns-level allows narrowed by pod-level baselines. Nothing to fix.">
                  Informational · {informational.length}
                </div>
                {informational.map(renderIssue)}
              </>
            )}
          </>
        )}
      </div>
    </div>
  );
}
