// Full-page V2 mockup — cluster status redesign, hardcoded data, no store.
// Access via URL hash `#design-preview`. Compare directly to the real page
// at the `/status` view (same data schema, mocked values).

import { SEVERITY_COLOR } from '../../data/policies';
import StatCardsV2 from './StatCardsV2';
import ExposedCalloutV2 from './ExposedCalloutV2';
import {
  SectionHeader, ZoneDivider, ProportionBarV2, LegendChips,
  RiskyTableV2, NamespaceTableV2, ChipRow,
} from './parts';
import {
  mockStats, mockExposed, mockCoverage, mockSeverity, mockRisky, mockNamespaces,
  mockSingleEngineCount, COVERAGE_CLASSES, COVERAGE_DESCRIPTION,
} from './mockData';

// Placeholder rollups — the real page uses EngineRollup / IssueRollup /
// StatusRollup from TablesView; mocked here as ChipRow so the preview stands
// alone without wiring those into fake data.
const mockEngines = [
  { key: 'k8s NetworkPolicy',    count: 22, color: SEVERITY_COLOR.info,    hint: 'L3/L4 policies' },
  { key: 'Istio AuthorizationPolicy', count: 16, color: SEVERITY_COLOR.secure, hint: 'L7-capable' },
];

const mockIssues = [
  { key: 'orphan-rule',    count: 8, color: SEVERITY_COLOR.high,     hint: 'rule references a peer that no longer exists' },
  { key: 'shadowed-allow', count: 5, color: SEVERITY_COLOR.warning,  hint: 'ALLOW covered by a broader DENY' },
  { key: 'blanket-egress', count: 3, color: SEVERITY_COLOR.caution,  hint: 'egress allow-all' },
  { key: 'partial-access', count: 4, color: SEVERITY_COLOR.info,     hint: 'informational — expected layering' },
];

const mockStatuses = [
  { key: 'internet-ingress', count: 6,  color: SEVERITY_COLOR.high },
  { key: 'internet-egress',  count: 12, color: SEVERITY_COLOR.high },
  { key: 'ns-full-access',   count: 19, color: SEVERITY_COLOR.warning },
  { key: 'l7-applied',       count: 24, color: SEVERITY_COLOR.secure },
  { key: 'air-gapped',       count: 9,  color: SEVERITY_COLOR.info },
];

// Color per coverage class (not severity — coverage isn't severity).
// Green = tight, blue = locked, yellow/red = open, gray = no-op, orange = audit.
const COVERAGE_COLOR: Record<typeof COVERAGE_CLASSES[number], string> = {
  'restricted':   SEVERITY_COLOR.secure,
  'deny all':     SEVERITY_COLOR.info,
  'allow all ns': SEVERITY_COLOR.caution,
  'allow all':    SEVERITY_COLOR.high,
  'unenforced':   '#868e96',
  'audit':        SEVERITY_COLOR.warning,
};

const coverageSegments = COVERAGE_CLASSES.map((cls) => ({
  key: cls, count: mockCoverage[cls], color: COVERAGE_COLOR[cls],
}));

export default function DesignPreview() {
  return (
    <div className="flex-grow-1 overflow-auto bg-dark text-light">
      <div className="d-flex flex-column gap-3 p-3" style={{ maxWidth: 1400, margin: '0 auto' }}>
        {/* Preview chrome */}
        <div className="d-flex align-items-baseline justify-content-between border-bottom border-secondary pb-2">
          <div className="d-flex align-items-baseline gap-2">
            <h5 className="text-light mb-0">Cluster status</h5>
            <span className="text-secondary fs-12">design preview (mock data)</span>
          </div>
          <span className="text-secondary fs-11">
            open <code className="text-info">#design-preview</code>, remove hash for real page
          </span>
        </div>

        {/* ─── POSTURE ────────────────────────────────────────────────── */}
        <ZoneDivider label="Posture" />

        <StatCardsV2 {...mockStats} />
        <ExposedCalloutV2
          nodes={mockExposed}
          onOpenNode={(node) => alert(`[preview] open node details: ${node.label}`)}
          onOpenGraph={(node) => alert(`[preview] open in graph: ${node.label}`)}
          onSeeAll={() => alert('[preview] see all in workloads table')}
        />

        <div className="d-flex flex-column gap-2">
          <SectionHeader title="Issues" hint="what's broken right now" onLink={() => {}} />
          <ChipRow items={mockIssues} />
        </div>

        {/* ─── COVERAGE ───────────────────────────────────────────────── */}
        <ZoneDivider label="Coverage" />

        <div className="d-flex flex-column gap-2">
          <SectionHeader title="Policies by engine" hint={`${mockCoverage.total} policies`} onLink={() => {}} />
          <ChipRow items={mockEngines} />
          {mockSingleEngineCount > 0 && (
            <div
              className="text-secondary fs-12 d-inline-flex align-items-center gap-2"
              title="Workloads carrying status keys from only one of the enabled engines — defense-in-depth gap"
            >
              <span className="swatch-dot" style={{ background: SEVERITY_COLOR.warning }} />
              {mockSingleEngineCount} workload{(mockSingleEngineCount as number) === 1 ? '' : 's'} covered by only one engine
            </div>
          )}
        </div>

        <div className="d-flex flex-column gap-2">
          <SectionHeader title="Rule coverage" hint={`${mockCoverage.restricted}/${mockCoverage.total} restricted`} onLink={() => {}} />
          <ProportionBarV2 segments={coverageSegments} total={mockCoverage.total} />
          <LegendChips segments={coverageSegments} hints={COVERAGE_DESCRIPTION} />
        </div>

        <div className="d-flex flex-column gap-2">
          <SectionHeader title="Node statuses" hint="worst-severity per workload" onLink={() => {}} />
          <ProportionBarV2 segments={mockSeverity} height={8} />
          <ChipRow items={mockStatuses} />
        </div>

        {/* ─── DRILLDOWNS ─────────────────────────────────────────────── */}
        <ZoneDivider label="Drilldowns" />

        <div className="d-flex flex-column gap-2">
          <SectionHeader title="Top risky workloads" hint="ranked worst-first, click row for detail" />
          <RiskyTableV2 rows={mockRisky} />
        </div>

        <div className="d-flex flex-column gap-2">
          <SectionHeader title="Namespaces" hint="click row to drill into graph" />
          <NamespaceTableV2 rows={mockNamespaces} />
        </div>
      </div>
    </div>
  );
}
