// V2 panels preview page. Hash-toggled via #design-preview-panels in App.tsx.
// Renders Node / Edge / Compare panels side-by-side w/ mock data so the
// surface system iterates in isolation from live store/API.

import { NodePanelV2, EdgePanelV2, ComparePanelV2 } from './panels';

export default function DesignPreviewPanels() {
  return (
    <div style={{ flex: 1, overflow: 'auto', background: '#0d1013', color: '#e6ecf2' }}>
      <div style={{ maxWidth: 1900, margin: '0 auto', padding: 20 }}>
        <div style={{
          display: 'flex',
          alignItems: 'baseline',
          justifyContent: 'space-between',
          borderBottom: '1px solid #ffffff20',
          paddingBottom: 10,
          marginBottom: 20,
        }}>
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 10 }}>
            <h5 style={{ margin: 0 }}>DetailPanel redesign</h5>
            <span style={{ color: '#7a848e', fontSize: 12 }}>V2 (mock data), tier-1/tier-2 surface system</span>
          </div>
          <span style={{ color: '#7a848e', fontSize: 11 }}>
            open <code style={{ color: '#4dabf7' }}>#design-preview-panels</code>, remove hash for live app
          </span>
        </div>

        <div style={{
          display: 'flex',
          gap: 20,
          alignItems: 'flex-start',
          flexWrap: 'wrap',
        }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ color: '#7a848e', fontSize: 11, letterSpacing: '0.06em', textTransform: 'uppercase' }}>
              Node (click on a workload)
            </div>
            <NodePanelV2 />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ color: '#7a848e', fontSize: 11, letterSpacing: '0.06em', textTransform: 'uppercase' }}>
              Edge (click on a connection)
            </div>
            <EdgePanelV2 />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ color: '#7a848e', fontSize: 11, letterSpacing: '0.06em', textTransform: 'uppercase' }}>
              Compare (pin src → click dst)
            </div>
            <ComparePanelV2 />
          </div>
        </div>
      </div>
    </div>
  );
}
