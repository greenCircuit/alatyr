// Headline metric strip — 5-column grid of bordered mini-cards. Card treatment
// keeps the strip visually consistent with every other section on the page.
// Danger tone lifts the border + number color so a red-tinted card carries
// the "on fire" signal without needing a separate banner.

import { SEVERITY_COLOR } from '../../data/policies';
import p from './Panel.module.css';

interface StatCardsProps {
  workloads:          number;
  policies:           number;
  namespacesSelected: number;
  namespacesTotal:    number;
  issues:             number;
  issuesBlocking:     number;
  exposedCount:       number;
  exposedNamespaces:  number;
}

function MetricCard({ label, value, sub, danger }: {
  label: string; value: string | number; sub: string; danger?: boolean;
}) {
  const borderStyle = danger
    ? { border: `1px solid ${SEVERITY_COLOR.high}` }
    : undefined;
  const valueColor  = danger ? SEVERITY_COLOR.high : '#e9ecef';
  return (
    <div className={p.card} style={borderStyle}>
      <div className="text-secondary fs-11 text-uppercase tracking-wide fw-semibold mb-2">
        {label}
      </div>
      <div className="tnum fw-bold" style={{ fontSize: 30, lineHeight: 1, color: valueColor }}>
        {value}
      </div>
      <div className="text-secondary fs-12 mt-2">{sub}</div>
    </div>
  );
}

export default function StatCards(props: StatCardsProps) {
  const nsLabel = props.namespacesSelected === props.namespacesTotal
    ? String(props.namespacesTotal)
    : `${props.namespacesSelected}/${props.namespacesTotal}`;

  const issuesDanger  = props.issuesBlocking > 0;
  const exposedDanger = props.exposedCount > 0;

  return (
    <div className="d-grid gap-3" style={{ gridTemplateColumns: 'repeat(5, 1fr)' }}>
      <MetricCard
        label="Issues"
        value={props.issues}
        sub={issuesDanger ? `${props.issuesBlocking} blocking` : 'none blocking'}
        danger={issuesDanger}
      />
      <MetricCard
        label="Exposed & unpoliced"
        value={props.exposedCount}
        sub={exposedDanger
          ? `in ${props.exposedNamespaces} namespace${props.exposedNamespaces === 1 ? '' : 's'}`
          : 'none'}
        danger={exposedDanger}
      />
      <MetricCard label="Workloads"  value={props.workloads} sub="in scope" />
      <MetricCard label="Policies"   value={props.policies}  sub="in scope" />
      <MetricCard label="Namespaces" value={nsLabel}         sub="selected of total" />
    </div>
  );
}
