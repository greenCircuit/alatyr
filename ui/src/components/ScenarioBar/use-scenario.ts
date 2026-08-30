// Owns scenario selection state for App: which preset is active, whether its
// target resolved, and whether the caption card is still up.

import { useCallback, useEffect, useRef, useState } from 'react';
import { DEMO_MODE } from '../../api/demo';
import { scenarioById, type Scenario } from '../../data/scenarios';
import {
  applyScenarioById, initialScenarioId, scenarioIdFromSearch, type ScenarioResolution,
} from '../../store/scenario';
import { useGraphStore } from '../../store/graphStore';

export function useScenario(enabled: boolean) {
  const nodeCount     = useGraphStore((s) => s.allNodes.length);
  const issuesLoading = useGraphStore((s) => s.issuesLoading);

  const [active, setActive]         = useState<Scenario | null>(null);
  const [resolution, setResolution] = useState<ScenarioResolution | null>(null);
  const [captionOpen, setCaptionOpen] = useState(false);
  const landed = useRef(false);

  const select = useCallback((id: string) => {
    const outcome = applyScenarioById(id);
    if (outcome === null) return;
    setActive(scenarioById(id));
    setResolution(outcome);
    setCaptionOpen(true);
  }, []);

  // The landing scenario applies once, after both the graph and the issue scan
  // have landed — targets resolve through /api/issues, so applying earlier
  // would report every scenario as unresolved. Outside demo mode only an
  // explicit `?s=` link lands; a live cluster gets no auto-scenario.
  useEffect(() => {
    if (!enabled || landed.current) return;
    if (nodeCount === 0 || issuesLoading) return;
    landed.current = true;
    const search = window.location.search;
    const id = DEMO_MODE ? initialScenarioId(search) : scenarioIdFromSearch(search);
    if (id) select(id);
  }, [enabled, nodeCount, issuesLoading, select]);

  const dismiss = useCallback(() => setCaptionOpen(false), []);

  return { active, resolution, captionOpen, select, dismiss };
}
