// Raw-manifest drawer: fetches one policy's cluster object on demand and shows
// the cleaned YAML. Debug/trust affordance — answers "is this edge real?" by
// rendering the source of truth next to the graph's interpretation. A 404 is
// framed as a stale-graph signal (edge on canvas, object gone from cluster).
//
// Single instance, driven by manifestStore: every ManifestButton just sets the
// store target, and the one <ManifestDrawer/> mounted at App root renders it.
// Opening a second manifest replaces the first instead of stacking drawers.

import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { fetchManifest, type PolicyManifest } from '../../../api/client';
import { useManifestStore } from '../../../store/manifestStore';
import { useGraphStore } from '../../../store/graphStore';
import s from '../DetailPanel.module.css';

// classifyScalar colors a YAML value by its scalar type so the block reads like
// an editor: quoted/plain strings, numbers, booleans/null get distinct hues.
function classifyScalar(value: string, key: number) {
  if (value === '') return null;
  if (/^(true|false|null|~)$/i.test(value)) return <span key={key} className={s.yamlBool}>{value}</span>;
  if (/^-?\d+(\.\d+)?$/.test(value)) return <span key={key} className={s.yamlNum}>{value}</span>;
  return <span key={key} className={s.yamlStr}>{value}</span>;
}

// splitComment separates a trailing ` # ...` comment from a value, ignoring `#`
// inside quotes so a string like "a#b" isn't truncated.
function splitComment(text: string): [string, string] {
  let quote = '';
  for (let pos = 0; pos < text.length; pos++) {
    const char = text[pos];
    if (quote) { if (char === quote) quote = ''; continue; }
    if (char === '"' || char === "'") { quote = char; continue; }
    if (char === '#' && (pos === 0 || text[pos - 1] === ' ')) {
      return [text.slice(0, pos), text.slice(pos)];
    }
  }
  return [text, ''];
}

// tokenizeLine turns one YAML line into colored spans: indent, optional list
// dash, `key:` punctuation, the typed value, and any trailing comment. Heuristic
// (not a full parser) — enough for the flat k8s/Istio manifests this drawer shows.
function tokenizeLine(line: string) {
  const tokens: ReactNode[] = [];
  let next = 0;
  const push = (node: ReactNode) => { tokens.push(<span key={next++}>{node}</span>); };

  const indent = line.match(/^\s*/)![0];
  let rest = line.slice(indent.length);
  if (indent) push(indent);

  if (rest === '---' || rest === '...') { push(<span className={s.yamlPunct}>{rest}</span>); return tokens; }

  const dash = rest.match(/^-\s+/);
  if (dash) { push(<span className={s.yamlPunct}>{dash[0]}</span>); rest = rest.slice(dash[0].length); }

  if (rest.startsWith('#')) { push(<span className={s.yamlComment}>{rest}</span>); return tokens; }

  // key: value  (value may be empty for mapping headers)
  const kv = rest.match(/^([^:\s][^:]*?):(\s|$)(.*)$/);
  if (kv) {
    const [, name, sep, raw] = kv;
    push(<span className={s.yamlKey}>{name}</span>);
    push(<span className={s.yamlPunct}>:</span>);
    if (sep) push(sep);
    const [value, comment] = splitComment(raw);
    push(classifyScalar(value, next));
    if (comment) push(<span className={s.yamlComment}>{comment}</span>);
    return tokens;
  }

  const [value, comment] = splitComment(rest);
  push(classifyScalar(value, next));
  if (comment) push(<span className={s.yamlComment}>{comment}</span>);
  return tokens;
}

// renderYaml tokenizes each line for editor-style syntax coloring and highlights
// any line that is exactly one of the selected workload's labels (`key: value`),
// so the operator sees which selector entries matched. Line-exact (trimmed) match
// avoids false positives — e.g. "loki" inside "name: loki-ingress" won't light.
function renderYaml(yaml: string, highlight: Set<string>) {
  return yaml.split('\n').map((line, index) => {
    const prefix = index === 0 ? '' : '\n';
    const className = highlight.has(line.trim()) ? s.yamlHighlight : undefined;
    return <span key={index} className={className}>{prefix}{tokenizeLine(line)}</span>;
  });
}

type LoadState =
  | { phase: 'loading' }
  | { phase: 'error'; message: string; notFound: boolean }
  | { phase: 'ready'; manifest: PolicyManifest };

// ManifestButton is the entry affordance dropped into a policy card's badge
// cluster: an outline "YAML" button that opens the shared drawer for that policy.
export function ManifestButton({ kind, namespace, name, highlight }: {
  kind:       string;
  namespace:  string;
  name:       string;
  highlight?: string[];
}) {
  const open = useManifestStore((store) => store.open);
  return (
    <button
      type="button"
      className={`btn btn-sm btn-outline-secondary py-0 px-1 ${s.smallText}`}
      onClick={() => open({ kind, namespace, name, highlight })}
    >
      YAML
    </button>
  );
}

// ManifestDrawer is the single drawer instance — mount once at App root. Renders
// nothing until a target is set in the store.
export function ManifestDrawer() {
  const target = useManifestStore((store) => store.target);
  const close = useManifestStore((store) => store.close);
  if (!target) return null;
  // key forces a fresh fetch/state when the target identity changes.
  return (
    <ManifestModal
      key={`${target.kind}/${target.namespace}/${target.name}`}
      kind={target.kind}
      namespace={target.namespace}
      name={target.name}
      highlight={target.highlight}
      onClose={close}
    />
  );
}

function ManifestModal({ kind, namespace, name, highlight, onClose }: {
  kind:       string;   // policySource: "k8s" | "istio" | "pa"
  namespace:  string;
  name:       string;
  highlight?: string[];
  onClose:    () => void;
}) {
  const [state, setState] = useState<LoadState>({ phase: 'loading' });
  const [copied, setCopied] = useState(false);

  // Highlight selector lines in the YAML. Prefer the trigger's explicit
  // selectors (e.g. a rule's policy-selector + this-workload labels); otherwise
  // fall back to the selected workload's own labels. Pod vs ns not distinguished.
  const nodeLabels = useGraphStore((store) => store.selectedNode?.labels);
  const highlightSet = useMemo(() => {
    const set = new Set<string>();
    if (highlight && highlight.length > 0) {
      for (const line of highlight) set.add(line);
    } else {
      for (const [key, value] of Object.entries(nodeLabels ?? {})) set.add(`${key}: ${value}`);
    }
    return set;
  }, [highlight, nodeLabels]);

  function load() {
    setState({ phase: 'loading' });
    fetchManifest(kind, namespace, name)
      .then((manifest) => setState({ phase: 'ready', manifest }))
      .catch((error: Error & { status?: number }) =>
        setState({ phase: 'error', message: error.message, notFound: error.status === 404 }));
  }

  // Fetch on open / identity change; close on Esc.
  useEffect(() => {
    load();
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind, namespace, name]);

  function copy() {
    if (state.phase !== 'ready') return;
    navigator.clipboard.writeText(state.manifest.yaml).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }

  const headerKind = state.phase === 'ready' ? state.manifest.kind : kind;

  // Flex child of .rightDock — renders to the left of the DetailPanel so both
  // read side by side, panel pinned right, YAML to its left.
  return (
    <div className={s.manifestDrawer}>
      <div className="d-flex justify-content-between align-items-start p-3 border-bottom border-secondary">
        <div className="d-flex flex-column gap-1">
          <span className="fw-bold text-light small">{headerKind}</span>
          <span className="text-secondary small">{namespace}/{name}</span>
        </div>
        <div className="d-flex gap-2 align-items-center">
          {state.phase === 'ready' && (
            <button className="btn btn-sm btn-outline-secondary" onClick={copy}>
              {copied ? 'Copied' : 'Copy'}
            </button>
          )}
          <button className="btn-close btn-close-white btn-sm" onClick={onClose} />
        </div>
      </div>

      <div className="p-3 d-flex flex-column" style={{ minHeight: 0 }}>
        {state.phase === 'loading' && (
          <div className="text-secondary small py-4 text-center">Fetching…</div>
        )}

        {state.phase === 'error' && (
          <div className="small">
            {state.notFound ? (
              <div className="text-warning">
                <div className="fw-semibold mb-1">⚠ Not found in cluster</div>
                <div className="text-secondary">
                  Edge is on the graph but the object is gone — graph may be stale.
                </div>
              </div>
            ) : (
              <div className="text-danger">{state.message}</div>
            )}
            <button className="btn btn-sm btn-outline-secondary mt-3" onClick={load}>Retry</button>
          </div>
        )}

        {state.phase === 'ready' && (
          <pre className={s.yamlBlock}>{renderYaml(state.manifest.yaml, highlightSet)}</pre>
        )}
      </div>
    </div>
  );
}
