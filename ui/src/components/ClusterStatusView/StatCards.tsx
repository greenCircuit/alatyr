// Headline numbers for the cluster status page. Pure props — all counting
// happens in store/clusterStats.ts so this stays a dumb renderer.

interface StatCardsProps {
  workloads:          number;
  policies:           number;
  namespacesSelected: number;
  namespacesTotal:    number;
  issues:             number;
  issuesBlocking:     boolean;
}

export default function StatCards(props: StatCardsProps) {
  const cards = [
    { label: 'Workloads',  value: String(props.workloads) },
    { label: 'Policies',   value: String(props.policies) },
    {
      label: 'Namespaces',
      value: props.namespacesSelected === props.namespacesTotal
        ? String(props.namespacesTotal)
        : `${props.namespacesSelected}/${props.namespacesTotal}`,
    },
    { label: 'Issues',     value: String(props.issues), alert: props.issuesBlocking },
  ];
  return (
    <div className="row g-3">
      {cards.map((card) => (
        <div key={card.label} className="col-6 col-md-3">
          <div className="card bg-dark text-light border-secondary h-100">
            <div className="card-body py-2 px-3">
              <div className="text-secondary text-uppercase fs-11 tracking-wide">{card.label}</div>
              <div className="fs-3 fw-bold d-flex align-items-center gap-2">
                {card.value}
                {card.alert && <span className="text-danger fs-5" title="blocking issues present">●</span>}
              </div>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
