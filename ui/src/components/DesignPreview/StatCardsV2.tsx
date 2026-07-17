// V2 stat display: two real KPI cards (Issues, Exposed) carrying severity via
// left border stripe; context counts (Workloads/Policies/Namespaces) demoted
// to a single inline strip. Hierarchy comes from contrast, not size alone.

import { SEVERITY_COLOR } from '../../data/policies';

interface Props {
  workloads:          number;
  policies:           number;
  namespacesSelected: number;
  namespacesTotal:    number;
  issues:             number;
  issuesBlocking:     number;
  exposedCount:       number;
  exposedNamespaces:  number;
}

function KpiCard({ label, value, sub, stripeColor, tone }: {
  label: string; value: number; sub: string; stripeColor: string; tone: 'danger' | 'warning' | 'secure';
}) {
  const valueClass = tone === 'danger' ? 'text-danger' : tone === 'warning' ? 'text-warning' : 'text-success';
  return (
    <div
      className="card bg-dark border-secondary border-l-5 h-100"
      style={{ borderLeftColor: stripeColor }}
    >
      <div className="card-body py-2 px-3">
        <div className="text-secondary text-uppercase fs-11 tracking-wide">{label}</div>
        <div className={`fs-3 fw-bold tnum ${valueClass}`}>{value}</div>
        <div className="text-secondary fs-11">{sub}</div>
      </div>
    </div>
  );
}

export default function StatCardsV2(props: Props) {
  const nsLabel = props.namespacesSelected === props.namespacesTotal
    ? String(props.namespacesTotal)
    : `${props.namespacesSelected}/${props.namespacesTotal}`;

  return (
    <div className="d-flex flex-column gap-2">
      <div className="row g-3">
        <div className="col-6">
          <KpiCard
            label="Issues"
            value={props.issues}
            sub={props.issuesBlocking > 0 ? `${props.issuesBlocking} blocking` : 'none blocking'}
            stripeColor={props.issuesBlocking > 0 ? SEVERITY_COLOR.high : SEVERITY_COLOR.secure}
            tone={props.issuesBlocking > 0 ? 'danger' : 'secure'}
          />
        </div>
        <div className="col-6">
          <KpiCard
            label="Exposed & unpoliced"
            value={props.exposedCount}
            sub={props.exposedCount > 0 ? `in ${props.exposedNamespaces} namespace${props.exposedNamespaces === 1 ? '' : 's'}` : 'none'}
            stripeColor={props.exposedCount > 0 ? SEVERITY_COLOR.high : SEVERITY_COLOR.secure}
            tone={props.exposedCount > 0 ? 'danger' : 'secure'}
          />
        </div>
      </div>
      <div className="d-flex align-items-baseline gap-3 fs-12 text-secondary px-1 pt-1">
        <span>Workloads <span className="text-light fw-semibold tnum">{props.workloads}</span></span>
        <span className="opacity-50">·</span>
        <span>Policies <span className="text-light fw-semibold tnum">{props.policies}</span></span>
        <span className="opacity-50">·</span>
        <span>Namespaces <span className="text-light fw-semibold tnum">{nsLabel}</span></span>
      </div>
    </div>
  );
}
