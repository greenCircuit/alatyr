// Cytoscape element builders for both render modes:
//   - buildElements: per-workload nodes inside namespace boxes
//   - buildAggregatedElements: one node per namespace, edges aggregated
// Each builder leans on coverage.ts for border colors and bundling.ts for
// edge grouping.

import cytoscape from 'cytoscape';
import type { WorkloadNode, PolicyEdge } from '../../../data/policies';
import { COV, computeCoverage } from './coverage';
import { bundleEdges, aggregateDirection, edgeLabel, bundleLabel } from './bundling';

export function buildElements(
  workloads: WorkloadNode[],
  policyEdges: PolicyEdge[],
  visibleNS: Set<string>,
  allNodes: WorkloadNode[],
): cytoscape.ElementDefinition[] {
  const coverage   = computeCoverage(policyEdges, allNodes);
  const bundles    = bundleEdges(policyEdges);
  const els: cytoscape.ElementDefinition[] = [];

  // Only add a namespace box if it has at least one visible workload
  const occupiedNS = new Set(workloads.filter((w) => w.namespace).map((w) => w.namespace));
  const namespaces = [...new Set(allNodes.map((n) => n.namespace).filter(Boolean))];

  // Two-node ns design:
  //   nsbox-<ns>  — invisible compound parent (just the border/box visual)
  //   ns-<ns>     — label-only node, ALSO a child of nsbox-<ns>; carries the
  //                 click handler and serves as the edge endpoint for ns-level
  //                 policies. Because both ns-<ns> and the workloads are
  //                 siblings inside the same compound, an "ingress from this ns"
  //                 edge is a normal sibling-to-sibling line — no loop.
  const nsNodeById = new Map(
    allNodes.filter((n) => n.type === 'namespace').map((n) => [n.id, n]),
  );
  for (const ns of namespaces) {
    if (!visibleNS.has(ns) || !occupiedNS.has(ns)) continue;
    els.push({
      data: {
        id:       `nsbox-${ns}`,
        ntype:    'namespace-box',
        covColor: COV[coverage.ns[ns] ?? 'none'],
      },
    });
    els.push({
      data: {
        id:       `ns-${ns}`,
        parent:   `nsbox-${ns}`,
        label:    ns,
        ntype:    'namespace',
        covColor: COV[coverage.ns[ns] ?? 'none'],
        workload: nsNodeById.get(`ns-${ns}`),
      },
    });
  }

  // Workload nodes
  const workloadNsById = new Map<string, string>();
  for (const w of workloads) {
    workloadNsById.set(w.id, w.namespace);
    const data: Record<string, unknown> = {
      id:       w.id,
      label:    w.label,
      ntype:    'workload',
      wtype:    w.type,
      workload: w,
      covColor: coverage.workload.has(w.id) ? COV.workload : COV.none,
    };
    if (w.namespace && visibleNS.has(w.namespace)) data.parent = `nsbox-${w.namespace}`;
    els.push({ data });
  }

  // Edge bundles. nsToChild flag = source is an NS box AND target lives
  // inside that same NS — Cytoscape gets a direct top-anchored edge instead
  // of routing around the compound parent (avoids loop-shaped visuals).
  for (const b of bundles) {
    const count = b.policies.length;
    const nsToChild =
      b.source.startsWith('ns-') &&
      workloadNsById.get(b.target) === b.source.slice('ns-'.length);
    els.push({
      data: {
        id:        b.id,
        source:    b.source,
        target:    b.target,
        label:     count === 1 ? edgeLabel(b.policies[0]) : bundleLabel(b.policies),
        policies:  b.policies,
        hasNS:     b.hasNS,
        direction: b.direction,
        action:    b.action,
        nsToChild,
      },
    });
  }

  return els;
}

export function buildAggregatedElements(
  workloads: WorkloadNode[],
  allEdges: PolicyEdge[],
  allNodes: WorkloadNode[],
): cytoscape.ElementDefinition[] {
  const coverage = computeCoverage(allEdges, allNodes);
  const els: cytoscape.ElementDefinition[] = [];

  // Look up by all nodes (not just filtered) so cross-NS workload edges are mapped correctly
  const nodeById = new Map(allNodes.map((w) => [w.id, w]));

  // Group ID: ns-{namespace} for namespaced nodes, own id for external
  const groupOf = (id: string): string => {
    const node = nodeById.get(id);
    if (!node) return id;
    return node.namespace ? `ns-${node.namespace}` : node.id;
  };

  // One aggregate node per occupied namespace
  const occupiedNS = new Set(workloads.filter((w) => w.namespace).map((w) => w.namespace));

  // Track which IDs actually exist as elements so we can drop dangling edges
  const validIds = new Set<string>();

  const nsNodeById = new Map(
    allNodes.filter((n) => n.type === 'namespace').map((n) => [n.id, n]),
  );
  for (const ns of occupiedNS) {
    validIds.add(`ns-${ns}`);
    els.push({
      data: {
        id:       `ns-${ns}`,
        label:    ns,
        ntype:    'namespace-agg',
        covColor: COV[coverage.ns[ns] ?? 'none'],
        workload: nsNodeById.get(`ns-${ns}`),
      },
    });
  }

  // External nodes (no namespace) stay as individual nodes
  for (const w of workloads.filter((w) => !w.namespace)) {
    validIds.add(w.id);
    els.push({
      data: {
        id:       w.id,
        label:    w.label,
        ntype:    'workload',
        wtype:    w.type,
        workload: w,
        covColor: coverage.workload.has(w.id) ? COV.workload : COV.none,
      },
    });
  }

  // Use all edges so workload-level cross-NS policies are included, not just namespace-level ones.
  // validIds gates which namespace pairs actually appear.
  // Split aggregated edges by action too so deny doesn't collapse into allow.
  const aggMap = new Map<string, { src: string; tgt: string; action: number; policies: PolicyEdge[] }>();
  for (const e of allEdges) {
    const src = e.level === 'namespace' ? e.source : groupOf(e.source);
    const tgt = e.level === 'namespace' ? e.target : groupOf(e.target);
    if (src === tgt) continue;
    if (!validIds.has(src) || !validIds.has(tgt)) continue;
    const action = e.action ?? 0;
    const key = `${src}\0${tgt}\0${action}`;
    if (!aggMap.has(key)) aggMap.set(key, { src, tgt, action, policies: [] });
    aggMap.get(key)!.policies.push(e);
  }

  for (const { src, tgt, action, policies } of aggMap.values()) {
    const count = policies.length;
    els.push({
      data: {
        id:        `agg-${src}-${tgt}-${action}`,
        source:    src,
        target:    tgt,
        label:     count === 1 ? edgeLabel(policies[0]) : bundleLabel(policies),
        policies,
        hasNS:     policies.some((p) => p.level === 'namespace'),
        direction: aggregateDirection(policies),
        action,
      },
    });
  }

  return els;
}
