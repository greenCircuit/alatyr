import { describe, it, expect } from 'vitest';
import { policyTableEdges } from './policyTableEdges';
import type { FilterState, MeshFilterValue } from './filters';
import type { WorkloadNode, PolicyEdge } from '../data/policies';

// Regression cover for the two ways the Policies table must diverge from the
// graph's edge filter. Both were live bugs: a Calico GlobalNetworkPolicy
// disappeared from the audit table when a namespace was selected, and a policy
// whose only surviving rule was a blanket allow-all vanished entirely because
// blanket rules carry no peer endpoint.

const node = (id: string, namespace: string): WorkloadNode =>
  ({ id, label: id, namespace, type: 'deployment', labels: {} });

const edge = (overrides: Partial<PolicyEdge> & { id: string }): PolicyEdge => ({
  source: '', target: '', level: 'workload', direction: 'egress',
  policyName: overrides.id, namespace: '', policySource: 'calico', action: 0,
  ...overrides,
});

const ALL_NAMESPACES = new Set(['gitlab', 'services']);

function state(overrides: Partial<FilterState>): FilterState {
  return {
    allNodes: [], allEdges: [],
    selectedNamespaces: new Set(ALL_NAMESPACES),
    selectedNodeTypes: new Set(['deployment']),
    selectedPolicySources: new Set(['calico', 'k8s']),
    selectedActions: new Set([0, 1]),
    selectedDirections: new Set(['ingress', 'egress']),
    selectedMeshFilters: new Set<MeshFilterValue>(),
    meshStatus: {},
    showNamespaceEdges: true,
    showConnectedNamespaces: false,
    searchQuery: '',
    ...overrides,
  };
}

const gitlabPod   = node('gitlab/runner', 'gitlab');
const servicesPod = node('services/api', 'services');
const podCidr     = node('cidr:10.42.0.0/16', '');

// The real cluster shape: one GNP allows the cluster ranges, another is a
// catch-all allow whose only surviving rule is the direction-wide marker.
const namedNetEdge = edge({
  id: 'gnp-named', policyName: 'egress-default-deny',
  source: 'services/api', target: 'cidr:10.42.0.0/16',
});
const blanketAllow = edge({
  id: 'gnp-blanket', policyName: 'egress-enroll-namespaces',
  source: 'gitlab/runner', target: '',
});
const blanketDeny = edge({
  id: 'gnp-deny', policyName: 'egress-default-deny',
  source: 'services/api', target: '', action: 1,
});
const namespacedNetpol = edge({
  id: 'netpol', policyName: 'services-network-policies', policySource: 'k8s',
  namespace: 'services', source: 'services/api', target: '',
});

// collapseUniformRules rewrites a cluster-wide policy's endpoint to the
// namespace node once every workload in that namespace resolved the same rule.
// Namespace nodes are deliberately absent from filteredNodes, so this endpoint
// needs its own admission path.
const collapsedBlanket = edge({
  id: 'gnp-collapsed', policyName: 'egress-enroll-collapsed',
  source: 'ns-gitlab', target: '',
});

const allNodes = [gitlabPod, servicesPod, podCidr];
const allEdges = [namedNetEdge, blanketAllow, blanketDeny, namespacedNetpol, collapsedBlanket];

const policyNames = (edges: PolicyEdge[]) => [...new Set(edges.map((e) => e.policyName))].sort();

describe('policyTableEdges', () => {
  it('keeps blanket rules that the graph filter drops for having no peer', () => {
    const result = policyTableEdges(state({ allNodes, allEdges }), ALL_NAMESPACES);
    expect(policyNames(result)).toEqual([
      'egress-default-deny', 'egress-enroll-collapsed', 'egress-enroll-namespaces',
      'services-network-policies',
    ]);
  });

  it('keeps blanket rules collapsed onto a namespace node', () => {
    const result = policyTableEdges(state({ allNodes, allEdges }), ALL_NAMESPACES);
    expect(policyNames(result)).toContain('egress-enroll-collapsed');
  });

  it('keeps cluster-scoped policies when a foreign namespace is selected', () => {
    // Only gitlab selected: the GNP rule pointing at a services workload is
    // cluster-scoped, so it must survive; the namespaced netpol must not.
    const result = policyTableEdges(
      state({ allNodes, allEdges, selectedNamespaces: new Set(['gitlab']) }),
      ALL_NAMESPACES,
    );
    expect(policyNames(result)).toEqual([
      'egress-default-deny', 'egress-enroll-collapsed', 'egress-enroll-namespaces',
    ]);
  });

  it('still scopes namespaced blanket rules to the namespace selection', () => {
    const result = policyTableEdges(
      state({ allNodes, allEdges, selectedNamespaces: new Set(['services']) }),
      ALL_NAMESPACES,
    );
    expect(policyNames(result)).toContain('services-network-policies');

    const other = policyTableEdges(
      state({ allNodes, allEdges, selectedNamespaces: new Set(['gitlab']) }),
      ALL_NAMESPACES,
    );
    expect(policyNames(other)).not.toContain('services-network-policies');
  });

  it('honours engine, action and direction chips on re-admitted rules', () => {
    const noCalico = policyTableEdges(
      state({ allNodes, allEdges, selectedPolicySources: new Set(['k8s']) }),
      ALL_NAMESPACES,
    );
    expect(policyNames(noCalico)).toEqual(['services-network-policies']);

    const allowOnly = policyTableEdges(
      state({ allNodes, allEdges, selectedActions: new Set([0]) }),
      ALL_NAMESPACES,
    );
    expect(allowOnly.some((e) => e.action === 1)).toBe(false);
    expect(policyNames(allowOnly)).toContain('egress-enroll-namespaces');

    const ingressOnly = policyTableEdges(
      state({ allNodes, allEdges, selectedDirections: new Set(['ingress']) }),
      ALL_NAMESPACES,
    );
    expect(ingressOnly).toHaveLength(0);
  });

  it('emits each edge once when it passes both the scoped and re-admitted paths', () => {
    const result = policyTableEdges(state({ allNodes, allEdges }), ALL_NAMESPACES);
    expect(result).toHaveLength(new Set(result.map((e) => e.id)).size);
  });
});
