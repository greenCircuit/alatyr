// Static-demo transport. With VITE_DEMO_MODE=true there is no Go backend —
// the site is a plain GitHub Pages upload — so every /api/* call resolves out
// of the JSON snapshot in public/demo/ produced by scripts/snapshot-api.py.
// Parameterized endpoints are lookup maps keyed by their query params; a key
// miss surfaces as a 404 so callers keep their existing error paths.

export const DEMO_MODE = import.meta.env.VITE_DEMO_MODE === 'true';

export class DemoHttpError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

const snapshots = new Map<string, Promise<unknown>>();

// One in-flight fetch per file, cached forever: the snapshot is immutable for
// the life of the page and several endpoints share a file.
function loadSnapshot<T>(file: string): Promise<T> {
  let pending = snapshots.get(file);
  if (!pending) {
    pending = fetch(`${import.meta.env.BASE_URL}demo/${file}`).then((response) => {
      if (!response.ok) {
        throw new DemoHttpError(response.status, `demo snapshot ${file} unavailable`);
      }
      return response.json();
    });
    snapshots.set(file, pending);
  }
  return pending as Promise<T>;
}

async function lookup<T>(file: string, key: string, missing: string): Promise<T> {
  const table = await loadSnapshot<Record<string, T>>(file);
  const entry = table[key];
  if (entry === undefined) throw new DemoHttpError(404, missing);
  return entry;
}

// Resolves one /api/* URL against the snapshot. The graph snapshot always
// covers every namespace — the UI filters namespaces client-side, so the
// `namespaces` query param is ignored rather than served a narrower graph.
export function demoRequest<T>(url: string): Promise<T> {
  const parsed = new URL(url, window.location.origin);
  const params = parsed.searchParams;

  switch (parsed.pathname) {
    case '/api/graph':
      return loadSnapshot<T>('graph.json');
    case '/api/cluster-state':
      return loadSnapshot<T>('cluster-state.json');
    case '/api/issues':
      return loadSnapshot<T>('issues.json');
    case '/api/mesh-status':
      return loadSnapshot<T>('mesh-status.json');
    case '/api/cluster-metrics':
      return loadSnapshot<T>('cluster-metrics.json');
    case '/api/node-info': {
      const nodeId = params.get('nodeId') ?? '';
      return lookup<T>('node-info.json', nodeId, `no demo data for node ${nodeId}`);
    }
    case '/api/reachable': {
      const key = ['srcId', 'srcNs', 'dstId', 'dstNs']
        .map((name) => params.get(name) ?? '').join('|');
      return lookup<T>('reachable.json', key, 'no demo reachability for this pair');
    }
    case '/api/manifest': {
      const key = ['kind', 'namespace', 'name']
        .map((name) => params.get(name) ?? '').join('|');
      return lookup<T>('manifest.json', key, `manifest ${key} not in demo snapshot`);
    }
    default:
      return Promise.reject(new DemoHttpError(404, `no demo route for ${parsed.pathname}`));
  }
}
