// Headline numbers for the cluster status page. Pure props — all counting
// happens in store/clusterStats.ts so this stays a dumb renderer.
//
// V2 layout: two KPI tiles carry severity via left stripe (semantic surfaces
// per STYLEGUIDE §3). Workloads/Policies/Namespaces render as an inline
// context strip — derivable from the nav scope and tables below.

import s from '../DetailPanel/DetailPanel.module.css';

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

function KpiCard({ label, value, sub, tone }: {
  label: string; value: number; sub: string; tone: 'deny' | 'allow';
}) {
  const shellClass = tone === 'deny' ? s.semanticDeny : s.semanticAllow;
  const valueClass = tone === 'deny' ? s.verdictTextDeny : s.verdictTextAllow;
  return (
    <div className={`${shellClass} h-100 d-flex flex-column gap-1`}>
      <div className={s.eyebrow}>{label}</div>
      <div className={`${s.hero} tnum ${valueClass}`}>{value}</div>
      <div className={`${s.dim} ${s.smallText}`}>{sub}</div>
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
    <div className="d-flex flex-column gap-2">
      <div className="row g-3">
        <div className="col-6">
          <KpiCard
            label="Issues"
            value={props.issues}
            sub={issuesDanger ? `${props.issuesBlocking} blocking` : 'none blocking'}
            tone={issuesDanger ? 'deny' : 'allow'}
          />
        </div>
        <div className="col-6">
          <KpiCard
            label="Exposed & unpoliced"
            value={props.exposedCount}
            sub={exposedDanger
              ? `in ${props.exposedNamespaces} namespace${props.exposedNamespaces === 1 ? '' : 's'}`
              : 'none'}
            tone={exposedDanger ? 'deny' : 'allow'}
          />
        </div>
      </div>
      <div className={`d-flex align-items-baseline gap-3 px-1 pt-1 ${s.body} ${s.dim}`}>
        <span>Workloads <span className={`${s.section} tnum`}>{props.workloads}</span></span>
        <span className={s.dim}>·</span>
        <span>Policies <span className={`${s.section} tnum`}>{props.policies}</span></span>
        <span className={s.dim}>·</span>
        <span>Namespaces <span className={`${s.section} tnum`}>{nsLabel}</span></span>
      </div>
    </div>
  );
}
