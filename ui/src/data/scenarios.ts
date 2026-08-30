// Scenario presets for the static demo (spec: DEMO-SCENARIOS.md).
//
// The registry data lives in scenarios.json, not here, because two consumers
// read it: this module (buttons, captions, landing state) and
// scripts/check-scenarios.py, which asserts in CI that every scenario's finding
// still fires against the real binary. One file, one truth — a caption that
// keeps asserting a finding the panel no longer shows is worse than no demo.
//
// Targets are declared by namespace + workload label, never by node id: node
// ids are k8s UIDs (internal/graph/buildNodes.go:26), opaque and fixture-bound.
// Resolution happens against the loaded graph at runtime (store/scenario.ts).

import type { IssueType } from './policies';
import registry from './scenarios.json';

export type ScenarioId =
  | 'dns-blackhole'
  | 'engine-contradiction'
  | 'mesh-transport'
  | 'strict-mtls'
  | 'exposure';

// Workload reference in fixture terms — what a reader of test-data/*.yaml can
// verify by eye.
export interface WorkloadRef {
  namespace: string;
  label:     string;
}

// What the landing state must resolve to. `issueType` is the primary key —
// scenarios land on a finding, not on a node — and `workload` narrows when a
// type fires on more than one workload. Both optional: the triage entry point
// lands on a dashboard.
export interface ScenarioTarget {
  issueType?: IssueType;
  workload?:  WorkloadRef;
}

export interface Scenario {
  id: ScenarioId;
  // Symptom-named, identical string in the button and the caption headline.
  label:   string;
  caption: string;
  // Exactly one instruction. The card carries no second call to action.
  cta:     string;
  // Namespaces to narrow to. Empty = whole cluster, unfiltered.
  namespaces: string[];
  view:       'graph' | 'tables' | 'status';
  tablesTab?: 'workloads' | 'policies' | 'issues';
  // Issue-type filter applied to the tables view alongside the landing state.
  issueFilter?: IssueType;
  target?:      ScenarioTarget;
  // Set apart in the button strip — a different entry point, not a fifth
  // mechanism lesson.
  standalone?: boolean;
}

// Landing state for a visitor who never clicks a button: the cross-engine
// contradiction, the one finding commodity visualizers structurally cannot
// produce.
export const DEFAULT_SCENARIO: ScenarioId = 'engine-contradiction';

export const SCENARIOS = registry as Scenario[];

export function scenarioById(id: string | null | undefined): Scenario | null {
  if (!id) return null;
  return SCENARIOS.find((scenario) => scenario.id === id) ?? null;
}
