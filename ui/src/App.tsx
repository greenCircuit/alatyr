import 'bootstrap/dist/css/bootstrap.min.css';
import FilterPanel from './components/FilterPanel';
import PolicyGraph from './components/PolicyGraph';
import TablesView from './components/TablesView';
import DetailPanel from './components/DetailPanel';
import { useGraphStore } from './store/graphStore';

export default function App() {
  const view = useGraphStore((s) => s.view);
  return (
    <div className="d-flex flex-column" style={{ height: '100vh', overflow: 'hidden' }}>
      <FilterPanel />
      <div className="position-relative flex-grow-1 d-flex flex-column" style={{ overflow: 'hidden' }}>
        {view === 'graph' ? <PolicyGraph /> : <TablesView />}
        <DetailPanel />
      </div>
    </div>
  );
}
