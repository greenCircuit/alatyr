import 'bootstrap/dist/css/bootstrap.min.css';
import { useEffect } from 'react';
import FilterPanel from './components/FilterPanel';
import PolicyGraph from './components/PolicyGraph';
import TablesView from './components/TablesView';
import ClusterStatusView from './components/ClusterStatusView';
import DetailPanel from './components/DetailPanel';
import IssuesDrawer from './components/IssuesDrawer';
import { useGraphStore } from './store/graphStore';

export default function App() {
  const view = useGraphStore((s) => s.view);
  const loadClusterState = useGraphStore((s) => s.loadClusterState);
  const loadGraph = useGraphStore((s) => s.loadGraph);

  // Initial data load lives here, not in PolicyGraph. PolicyGraph remounts on
  // every view switch; loading there re-runs loadClusterState and wipes the
  // user's filter selections. App mounts once, so filters survive the switch.
  useEffect(() => { loadClusterState(); loadGraph(); }, [loadClusterState, loadGraph]);
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
