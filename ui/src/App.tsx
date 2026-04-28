import 'bootstrap/dist/css/bootstrap.min.css';
import FilterPanel from './components/FilterPanel';
import PolicyGraph from './components/PolicyGraph';

export default function App() {
  return (
    <div className="d-flex flex-column" style={{ height: '100vh', overflow: 'hidden' }}>
      <FilterPanel />
      <PolicyGraph />
    </div>
  );
}
