// Scenario preset strip + caption card (spec: DEMO-SCENARIOS.md).
//
// Buttons are symptom-named and lead with the recognized scenario; the
// standalone triage entry point is set apart on the right. Clicking one
// restores its landing state and stamps `?s=<id>` on the URL so the view is
// shareable. The caption card carries one sentence and one instruction, and
// dismisses on the first interaction with the view underneath.

import { useEffect } from 'react';
import { SCENARIOS, type Scenario } from '../../data/scenarios';
import type { ScenarioResolution } from '../../store/scenario';
import styles from './ScenarioBar.module.css';

interface Props {
  active:   Scenario | null;
  onSelect: (id: string) => void;
}

export default function ScenarioBar({ active, onSelect }: Props) {
  const lessons = SCENARIOS.filter((scenario) => !scenario.standalone);
  const entries = SCENARIOS.filter((scenario) => scenario.standalone);

  return (
    <div className="d-flex flex-wrap align-items-center gap-1 px-3 py-2 border-bottom border-secondary bg-dark text-light flex-shrink-0 fs-13">
      <span className="text-secondary text-uppercase me-2" style={{ fontSize: '0.7rem', letterSpacing: '0.05em' }}>
        Scenarios
      </span>
      {lessons.map((scenario) => (
        <ScenarioButton
          key={scenario.id}
          scenario={scenario}
          active={active?.id === scenario.id}
          onSelect={onSelect}
        />
      ))}
      <span className="border-start border-secondary mx-2 align-self-stretch" />
      {entries.map((scenario) => (
        <ScenarioButton
          key={scenario.id}
          scenario={scenario}
          active={active?.id === scenario.id}
          onSelect={onSelect}
        />
      ))}
    </div>
  );
}

function ScenarioButton(
  { scenario, active, onSelect }: { scenario: Scenario; active: boolean; onSelect: (id: string) => void },
) {
  return (
    <button
      type="button"
      className={`btn btn-sm text-nowrap ${active ? 'btn-warning' : 'btn-outline-secondary'}`}
      onClick={() => onSelect(scenario.id)}
    >
      {scenario.label}
    </button>
  );
}

// Rendered inside the view container so it overlays the graph/table rather than
// pushing it down. Dismisses on any pointerdown outside itself — a visitor who
// starts exploring has already read it.
export function ScenarioCaption({ scenario, resolution, onDismiss }: {
  scenario:   Scenario;
  resolution: ScenarioResolution | null;
  onDismiss:  () => void;
}) {
  useEffect(() => {
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.closest(`.${styles.card}`)) return;
      onDismiss();
    };
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [onDismiss]);

  return (
    <div className={`card bg-dark text-light border-secondary shadow ${styles.card}`}>
      <div className="card-body py-2 px-3">
        <div className="d-flex align-items-start gap-2">
          <div className={`fw-semibold flex-grow-1 ${styles.headline}`}>{scenario.label}</div>
          <button
            type="button"
            className="btn-close btn-close-white flex-shrink-0"
            style={{ fontSize: '0.6rem' }}
            aria-label="Dismiss scenario caption"
            onClick={onDismiss}
          />
        </div>
        <div className={`text-secondary mt-1 ${styles.body}`}>{scenario.caption}</div>
        {resolution === 'unresolved' ? (
          <div className={`text-warning fst-italic mt-2 ${styles.cta}`}>
            This finding is not in the current snapshot — the view below may not show it.
          </div>
        ) : (
          <div className={`text-warning fst-italic mt-2 ${styles.cta}`}>{scenario.cta}</div>
        )}
      </div>
    </div>
  );
}
