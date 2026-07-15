import { describe, it, expect } from 'vitest';
import { aggregateAllowPorts } from './reachability';
import type { DirectionVerdict, NodeRule } from '../../../data/policies';

function allowRule(overrides: Partial<NodeRule>): NodeRule {
  return { direction: 'ingress', ports: [], action: 0, dstId: 'dst', ...overrides };
}

function verdict(overrides: Partial<DirectionVerdict>): DirectionVerdict {
  return { reason: 'permitted', ...overrides };
}

describe('aggregateAllowPorts', () => {
  it('unions and dedupes restricted ports across allow rules', () => {
    const side = aggregateAllowPorts(verdict({
      allowMatches: [
        allowRule({ ports: [{ port: 8080, protocol: 'TCP' }, { port: 443, protocol: 'TCP' }] }),
        allowRule({ ports: [{ port: 8080, protocol: 'TCP' }, { port: 53, protocol: 'UDP' }] }),
      ],
    }));
    expect(side.blocked).toBe(false);
    expect(side.allPorts).toBe(false);
    expect(side.ports.map((port) => `${port.port}/${port.protocol}`))
      .toEqual(['8080/TCP', '443/TCP', '53/UDP']);
  });

  it('collapses to all-ports on allPorts flag, port-0 sentinel, no-opinion, or unattributed permit', () => {
    const flagged = aggregateAllowPorts(verdict({
      allowMatches: [
        allowRule({ ports: [{ port: 8080, protocol: 'TCP' }] }),
        allowRule({ allPorts: true }),
      ],
    }));
    expect(flagged.allPorts).toBe(true);
    expect(flagged.ports).toEqual([]);

    const sentinel = aggregateAllowPorts(verdict({
      allowMatches: [allowRule({ ports: [{ port: 0, protocol: 'TCP' }] })],
    }));
    expect(sentinel.allPorts).toBe(true);

    expect(aggregateAllowPorts(verdict({ reason: 'no-opinion' })).allPorts).toBe(true);
    expect(aggregateAllowPorts(verdict({ allowMatches: [] })).allPorts).toBe(true);
  });

  it('marks blocking directions instead of reporting ports', () => {
    const side = aggregateAllowPorts(verdict({
      reason: 'default-deny',
      allowMatches: [allowRule({ ports: [{ port: 8080, protocol: 'TCP' }] })],
    }));
    expect(side.blocked).toBe(true);
    expect(side.allPorts).toBe(false);
    expect(side.ports).toEqual([]);
  });
});
