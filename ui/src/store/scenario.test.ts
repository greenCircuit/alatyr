import { describe, it, expect } from 'vitest';
import { findIssue, findWorkload, scenarioIdFromSearch, initialScenarioId } from './scenario';
import { SCENARIOS, DEFAULT_SCENARIO, scenarioById } from '../data/scenarios';
import type { Issue, WorkloadNode } from '../data/policies';

const node = (namespace: string, label: string): WorkloadNode =>
  ({ id: `uid-${namespace}-${label}`, label, namespace, type: 'service', labels: {} });

const nodes = [node('analytics', 'enrich'), node('analytics', 'ingest'), node('payments', 'fraud-check')];

const issue = (type: Issue['type'], src?: WorkloadNode, dst?: WorkloadNode): Issue =>
  ({ type, message: type, src, dst });

describe('scenario target resolution', () => {
  it('prefers the named workload, falls back to type, reports a dead type', () => {
    const onIngest = issue('no dns', nodes[1]);
    const onEnrich = issue('no dns', nodes[0]);
    const issues = [onIngest, onEnrich];

    expect(findIssue(issues, { issueType: 'no dns', workload: { namespace: 'analytics', label: 'enrich' } }))
      .toBe(onEnrich);
    // named workload absent → first of the type still lands the visitor on a
    // real finding rather than nothing
    expect(findIssue(issues, { issueType: 'no dns', workload: { namespace: 'analytics', label: 'gone' } }))
      .toBe(onIngest);
    expect(findIssue(issues, { issueType: 'policy conflict' })).toBeNull();
    expect(findIssue(issues, undefined)).toBeNull();
  });

  it('resolves workloads by namespace + label, not by id', () => {
    expect(findWorkload(nodes, { namespace: 'payments', label: 'fraud-check' })?.id).toBe('uid-payments-fraud-check');
    expect(findWorkload(nodes, { namespace: 'storefront', label: 'fraud-check' })).toBeNull();
  });
});

describe('scenario deep links', () => {
  it('reads ?s= and falls back to the differentiated landing scenario', () => {
    expect(scenarioIdFromSearch('?s=dns-blackhole')).toBe('dns-blackhole');
    expect(scenarioIdFromSearch('?ns=payments')).toBeNull();
    expect(initialScenarioId('')).toBe(DEFAULT_SCENARIO);
    expect(initialScenarioId('?s=exposure')).toBe('exposure');
  });

  it('keeps the registry self-consistent — unique ids, one caption + cta each', () => {
    const ids = SCENARIOS.map((scenario) => scenario.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(scenarioById(DEFAULT_SCENARIO)).not.toBeNull();
    expect(scenarioById('nope')).toBeNull();
    for (const scenario of SCENARIOS) {
      expect(scenario.caption.length).toBeGreaterThan(0);
      expect(scenario.cta.length).toBeGreaterThan(0);
      // tables-only settings must not ride along on a graph/status landing
      if (scenario.view !== 'tables') expect(scenario.tablesTab).toBeUndefined();
    }
  });
});
