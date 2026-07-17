import 'bootstrap/dist/css/bootstrap.min.css';
import { useEffect, useState } from 'react';
import FilterPanel from './components/FilterPanel';
import PolicyGraph from './components/PolicyGraph';
import TablesView from './components/TablesView';
import ClusterStatusView from './components/ClusterStatusView';
import DetailPanel from './components/DetailPanel';
import IssuesDrawer from './components/IssuesDrawer';
import DesignPreview from './components/DesignPreview';
import DesignPreviewPanels from './components/DetailPanel/DesignPreview';
import { useGraphStore } from './store/graphStore';

type PreviewMode = 'status' | 'panels' | null;

function currentPreview(): PreviewMode {
  if (typeof window === 'undefined') return null;
  if (window.location.hash === '#design-preview') return 'status';
  if (window.location.hash === '#design-preview-panels') return 'panels';
  return null;
}

export default function App() {
  const view = useGraphStore((s) => s.view);
  const loadClusterState = useGraphStore((s) => s.loadClusterState);
  const loadGraph = useGraphStore((s) => s.loadGraph);

  // Throwaway design-preview routes toggled by URL hash. Bypasses store/API so
  // mock pages render without a cluster. Remove when redesign lands.
  const [preview, setPreview] = useState<PreviewMode>(() => currentPreview());
  useEffect(() => {
    const onHashChange = () => setPreview(currentPreview());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  // Initial data load lives here, not in PolicyGraph. PolicyGraph remounts on
  // every view switch; loading there re-runs loadClusterState and wipes the
  // user's filter selections. App mounts once, so filters survive the switch.
  useEffect(() => {
    if (preview) return;
    loadClusterState();
    loadGraph();
  }, [loadClusterState, loadGraph, preview]);

  if (preview === 'status') {
    return (
      <div className="d-flex flex-column" style={{ height: '100vh', overflow: 'hidden' }}>
        <DesignPreview />
      </div>
    );
  }
  if (preview === 'panels') {
    return (
      <div className="d-flex flex-column" style={{ height: '100vh', overflow: 'hidden' }}>
        <DesignPreviewPanels />
      </div>
    );
  }

  return (
    <div className="d-flex flex-column" style={{ height: '100vh', overflow: 'hidden' }}>
      <FilterPanel />
      <div className="position-relative flex-grow-1 d-flex flex-column overflow-hidden">
        {view === 'graph' && <PolicyGraph />}
        {view === 'tables' && <TablesView />}
        {view === 'status' && <ClusterStatusView />}
        <IssuesDrawer />
        <DetailPanel />
      </div>
    </div>
  );
}
