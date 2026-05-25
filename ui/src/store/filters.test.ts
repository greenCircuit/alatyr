import { describe, it, expect } from 'vitest';
import { filteredNodes, filteredEdges, type FilterState } from './filters';
import type { WorkloadNode, PolicyEdge } from '../data/policies';

// ── Fixtures ──────────────────────────────────────────────────────────────────

const node = (
  id: string,
  namespace: string,
  type: WorkloadNode['type'] = 'service',
  label = id,
): WorkloadNode => ({ id, label, namespace, type, labels: {} });

const edge = (
  id: string,
  source: string,
  target: string,
  level: PolicyEdge['level'] = 'workload',
): PolicyEdge => ({ id, source, target, level, direction: 'egress', policyName: id, namespace: 'test', policySource: 'k8s' });

const ALL_TYPES = new Set(['service', 'deployment', 'headless', 'external']);

function state(overrides: Partial<FilterState>): FilterState {
  return {
    allNodes: [],
    allEdges: [],
    selectedNamespaces: new Set(),
    selectedNodeTypes: ALL_TYPES,
    selectedPolicySources: new Set(['k8s', 'istio']),
    selectedActions: new Set([0, 1]),
    showNamespaceEdges: true,
    showConnectedNamespaces: false,
    searchQuery: '',
    ...overrides,
  };
}

// Nodes used across tests
const nodeA1 = node('a1', 'ns-a');
const nodeA2 = node('a2', 'ns-a', 'deployment');
const nodeB1 = node('b1', 'ns-b');
const nodeC1 = node('c1', 'ns-c');
const nodeExt = node('internet', '', 'external');

const allNodes = [nodeA1, nodeA2, nodeB1, nodeC1, nodeExt];

// Edges used across tests
const edgeA1B1 = edge('e-a1-b1', 'a1', 'b1');           // workload: ns-a → ns-b
const edgeA2C1 = edge('e-a2-c1', 'a2', 'c1');           // workload: ns-a → ns-c
const edgeNsANsB = edge('e-ns-a-ns-b', 'ns-ns-a', 'ns-ns-b', 'namespace');
const edgeExtA1 = edge('e-ext-a1', 'internet', 'a1');   // workload: external → ns-a

// ── filteredNodes ─────────────────────────────────────────────────────────────

describe('filteredNodes', () => {
  it('returns all namespaced nodes when all namespaces selected', () => {
    const result = filteredNodes(state({
      allNodes,
      selectedNamespaces: new Set(['ns-a', 'ns-b', 'ns-c']),
    }));
    expect(result.map(n => n.id).sort()).toEqual(['a1', 'a2', 'b1', 'c1', 'internet']);
  });

  it('filters to only selected namespace', () => {
    const result = filteredNodes(state({
      allNodes,
      selectedNamespaces: new Set(['ns-a']),
    }));
    expect(result.map(n => n.id).sort()).toEqual(['a1', 'a2', 'internet']);
  });

  it('always includes external nodes (namespace = "") regardless of selection', () => {
    const result = filteredNodes(state({
      allNodes,
      selectedNamespaces: new Set(), // nothing selected
    }));
    expect(result.map(n => n.id)).toEqual(['internet']);
  });

  it('excludes nodes whose type is deselected', () => {
    const result = filteredNodes(state({
      allNodes,
      selectedNamespaces: new Set(['ns-a']),
      selectedNodeTypes: new Set(['service', 'external']), // deployment excluded
    }));
    expect(result.map(n => n.id).sort()).toEqual(['a1', 'internet']);
  });

  it('excludes external node when external type is deselected', () => {
    const result = filteredNodes(state({
      allNodes,
      selectedNamespaces: new Set(['ns-a']),
      selectedNodeTypes: new Set(['service']), // external excluded
    }));
    expect(result.map(n => n.id)).not.toContain('internet');
  });

  it('filters by search query matching label', () => {
    const nodes = [
      node('alpha', 'ns-a', 'service', 'alpha-svc'),
      node('beta', 'ns-a', 'service', 'beta-svc'),
    ];
    const result = filteredNodes(state({
      allNodes: nodes,
      selectedNamespaces: new Set(['ns-a']),
      searchQuery: 'alpha',
    }));
    expect(result.map(n => n.id)).toEqual(['alpha']);
  });

  it('filters by search query matching namespace', () => {
    const result = filteredNodes(state({
      allNodes,
      selectedNamespaces: new Set(['ns-a', 'ns-b', 'ns-c']),
      searchQuery: 'ns-b',
    }));
    expect(result.map(n => n.id)).toEqual(['b1']);
  });

  it('empty search query returns all nodes matching other filters', () => {
    const result = filteredNodes(state({
      allNodes,
      selectedNamespaces: new Set(['ns-a']),
      searchQuery: '',
    }));
    expect(result.map(n => n.id).sort()).toEqual(['a1', 'a2', 'internet']);
  });

  describe('showConnectedNamespaces', () => {
    it('adds nodes from namespaces reachable via outbound edges', () => {
      const result = filteredNodes(state({
        allNodes,
        allEdges: [edgeA1B1],
        selectedNamespaces: new Set(['ns-a']),
        showConnectedNamespaces: true,
      }));
      expect(result.map(n => n.id)).toContain('b1');
    });

    it('adds nodes reachable via inbound edges (bidirectional)', () => {
      const result = filteredNodes(state({
        allNodes,
        allEdges: [edgeA1B1],
        selectedNamespaces: new Set(['ns-b']), // selected b, edge points a1→b1
        showConnectedNamespaces: true,
      }));
      expect(result.map(n => n.id)).toContain('a1');
    });

    it('does not add nodes already in base set', () => {
      const result = filteredNodes(state({
        allNodes,
        allEdges: [edgeA1B1],
        selectedNamespaces: new Set(['ns-a', 'ns-b']),
        showConnectedNamespaces: true,
      }));
      const ids = result.map(n => n.id);
      // b1 is already selected, should appear exactly once
      expect(ids.filter(id => id === 'b1')).toHaveLength(1);
    });

    it('respects selectedNodeTypes for extra nodes', () => {
      const deploymentInB = node('b-deploy', 'ns-b', 'deployment');
      const serviceInB    = node('b-svc',    'ns-b', 'service');
      const edgeToB = edge('e', 'a1', 'b-svc');
      const result = filteredNodes(state({
        allNodes: [...allNodes, deploymentInB, serviceInB],
        allEdges: [edgeToB],
        selectedNamespaces: new Set(['ns-a']),
        selectedNodeTypes: new Set(['service', 'external']), // deployments excluded
        showConnectedNamespaces: true,
      }));
      expect(result.map(n => n.id)).toContain('b-svc');
      expect(result.map(n => n.id)).not.toContain('b-deploy');
    });

    it('ignores namespace-level edges when finding connected nodes', () => {
      const result = filteredNodes(state({
        allNodes,
        allEdges: [edgeNsANsB], // namespace-level, should be ignored
        selectedNamespaces: new Set(['ns-a']),
        showConnectedNamespaces: true,
      }));
      expect(result.map(n => n.id)).not.toContain('b1');
    });

    it('follows multiple edges to reach multiple namespaces', () => {
      const result = filteredNodes(state({
        allNodes,
        allEdges: [edgeA1B1, edgeA2C1],
        selectedNamespaces: new Set(['ns-a']),
        showConnectedNamespaces: true,
      }));
      const ids = result.map(n => n.id);
      expect(ids).toContain('b1');
      expect(ids).toContain('c1');
    });

    it('off by default: does not add connected nodes', () => {
      const result = filteredNodes(state({
        allNodes,
        allEdges: [edgeA1B1],
        selectedNamespaces: new Set(['ns-a']),
        showConnectedNamespaces: false,
      }));
      expect(result.map(n => n.id)).not.toContain('b1');
    });
  });
});

// ── filteredEdges ─────────────────────────────────────────────────────────────

describe('filteredEdges', () => {
  it('includes workload edge when both endpoints are in filteredNodes', () => {
    const result = filteredEdges(state({
      allNodes,
      allEdges: [edgeA1B1],
      selectedNamespaces: new Set(['ns-a', 'ns-b']),
    }));
    expect(result.map(e => e.id)).toContain('e-a1-b1');
  });

  it('excludes workload edge when target is in unselected namespace', () => {
    const result = filteredEdges(state({
      allNodes,
      allEdges: [edgeA1B1],
      selectedNamespaces: new Set(['ns-a']), // ns-b not selected
    }));
    expect(result.map(e => e.id)).not.toContain('e-a1-b1');
  });

  it('excludes workload edge when source is in unselected namespace', () => {
    const result = filteredEdges(state({
      allNodes,
      allEdges: [edgeA1B1],
      selectedNamespaces: new Set(['ns-b']), // ns-a not selected
    }));
    expect(result.map(e => e.id)).not.toContain('e-a1-b1');
  });

  it('includes edge from external node (namespace="") to selected namespace', () => {
    const result = filteredEdges(state({
      allNodes,
      allEdges: [edgeExtA1],
      selectedNamespaces: new Set(['ns-a']),
    }));
    expect(result.map(e => e.id)).toContain('e-ext-a1');
  });

  describe('namespace-level edges', () => {
    const nsEdge = edge('ns-edge', 'ns-ns-a', 'ns-ns-b', 'namespace');

    it('includes namespace edge when both ns- nodes are visible and showNamespaceEdges=true', () => {
      const result = filteredEdges(state({
        allNodes,
        allEdges: [nsEdge],
        selectedNamespaces: new Set(['ns-a', 'ns-b']),
        showNamespaceEdges: true,
      }));
      expect(result.map(e => e.id)).toContain('ns-edge');
    });

    it('excludes namespace edge when showNamespaceEdges=false', () => {
      const result = filteredEdges(state({
        allNodes,
        allEdges: [nsEdge],
        selectedNamespaces: new Set(['ns-a', 'ns-b']),
        showNamespaceEdges: false,
      }));
      expect(result.map(e => e.id)).not.toContain('ns-edge');
    });

    it('excludes namespace edge when one namespace has no visible workloads', () => {
      // ns-b is selected but has no workloads matching the type filter
      const result = filteredEdges(state({
        allNodes,
        allEdges: [nsEdge],
        selectedNamespaces: new Set(['ns-a', 'ns-b']),
        selectedNodeTypes: new Set(['deployment']), // b1 is a service — excluded
        showNamespaceEdges: true,
      }));
      // ns-ns-b not in visibleIds because no deployment workload occupies ns-b
      expect(result.map(e => e.id)).not.toContain('ns-edge');
    });

    it('excludes namespace edge when one of the namespaces is not selected', () => {
      const result = filteredEdges(state({
        allNodes,
        allEdges: [nsEdge],
        selectedNamespaces: new Set(['ns-a']), // ns-b not selected
        showNamespaceEdges: true,
      }));
      expect(result.map(e => e.id)).not.toContain('ns-edge');
    });
  });

  describe('showConnectedNamespaces', () => {
    it('includes cross-namespace workload edge when target is a connected node', () => {
      const result = filteredEdges(state({
        allNodes,
        allEdges: [edgeA1B1],
        selectedNamespaces: new Set(['ns-a']),
        showConnectedNamespaces: true,
      }));
      expect(result.map(e => e.id)).toContain('e-a1-b1');
    });

    it('includes namespace edge to reached namespace when showConnectedNamespaces=true', () => {
      const nsEdge = edge('ns-edge', 'ns-ns-a', 'ns-ns-b', 'namespace');
      const result = filteredEdges(state({
        allNodes,
        allEdges: [edgeA1B1, nsEdge],
        selectedNamespaces: new Set(['ns-a']),
        showConnectedNamespaces: true,
        showNamespaceEdges: true,
      }));
      // ns-b is reached via a1→b1; ns-ns-b should be in visibleIds
      expect(result.map(e => e.id)).toContain('ns-edge');
    });

    it('excludes namespace edge to reached namespace when showNamespaceEdges=false', () => {
      const nsEdge = edge('ns-edge', 'ns-ns-a', 'ns-ns-b', 'namespace');
      const result = filteredEdges(state({
        allNodes,
        allEdges: [edgeA1B1, nsEdge],
        selectedNamespaces: new Set(['ns-a']),
        showConnectedNamespaces: true,
        showNamespaceEdges: false,
      }));
      expect(result.map(e => e.id)).not.toContain('ns-edge');
    });

    it('does not include ns- for unoccupied namespaces when showConnectedNamespaces=false', () => {
      const nsEdge = edge('ns-edge', 'ns-ns-a', 'ns-ns-b', 'namespace');
      const result = filteredEdges(state({
        allNodes,
        allEdges: [edgeA1B1, nsEdge],
        selectedNamespaces: new Set(['ns-a']), // ns-b not selected
        showConnectedNamespaces: false,
        showNamespaceEdges: true,
      }));
      expect(result.map(e => e.id)).not.toContain('ns-edge');
    });
  });
});
