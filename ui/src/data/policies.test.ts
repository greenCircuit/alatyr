import { describe, it, expect } from 'vitest';
import { formatPort, realPorts, formatL7Summary } from './policies';
import type { Port, L7Match } from './policies';

const port = (overrides: Partial<Port> = {}): Port => ({
  port:     overrides.port     ?? 8080,
  protocol: overrides.protocol ?? 'TCP',
  endPort:  overrides.endPort,
  name:     overrides.name,
});

describe('formatPort', () => {
  it('named port wins over the number', () => {
    expect(formatPort(port({ name: 'http', port: 0 }))).toBe('http');
  });
  it('renders a range when endPort is set', () => {
    expect(formatPort(port({ port: 8000, endPort: 9000 }))).toBe('8000-9000');
  });
  it('falls back to the bare port number', () => {
    expect(formatPort(port({ port: 443 }))).toBe('443');
  });
});

describe('realPorts', () => {
  it('strips the {port:0} all-ports sentinel', () => {
    expect(realPorts([port({ port: 0 })])).toBeUndefined();
  });
  it('keeps a named port-0 entry (real named port, not sentinel)', () => {
    const named = port({ port: 0, name: 'http' });
    expect(realPorts([named])).toEqual([named]);
  });
  it('keeps real ports and drops sentinels in a mixed list', () => {
    const real = port({ port: 8080 });
    expect(realPorts([real, port({ port: 0 })])).toEqual([real]);
  });
  it('returns undefined for undefined input', () => {
    expect(realPorts(undefined)).toBeUndefined();
  });
});

describe('formatL7Summary', () => {
  it('returns null when there is no L7 data', () => {
    expect(formatL7Summary(undefined)).toBeNull();
    expect(formatL7Summary([])).toBeNull();
  });
  it('dedupes methods and paths across blocks', () => {
    const l7: L7Match[] = [
      { methods: ['GET'], paths: ['/a'] },
      { methods: ['GET', 'POST'], paths: ['/a', '/b'] },
    ];
    expect(formatL7Summary(l7)).toBe('L7: GET,POST /a,/b');
  });
  it('omits hosts when paths are present, shows them otherwise', () => {
    expect(formatL7Summary([{ paths: ['/a'], hosts: ['example.com'] }])).toBe('L7: /a');
    expect(formatL7Summary([{ hosts: ['example.com'] }])).toBe('L7: example.com');
  });
  it('returns bare "L7" when blocks carry no positive matcher', () => {
    expect(formatL7Summary([{ notPaths: ['/x'] }])).toBe('L7');
  });
});
