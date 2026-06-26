import { describe, it, expect } from 'vitest';
import { nextSort, type SortState } from './SortHeader';

type Col = 'name' | 'namespace';

describe('nextSort', () => {
  it("starts a new sort at asc when clicking a different column", () => {
    const current: SortState<Col> = { col: 'name', dir: 'desc' };
    expect(nextSort(current, 'namespace')).toEqual({ col: 'namespace', dir: 'asc' });
  });

  it('cycles asc → desc on the same column', () => {
    const current: SortState<Col> = { col: 'name', dir: 'asc' };
    expect(nextSort(current, 'name')).toEqual({ col: 'name', dir: 'desc' });
  });

  it('cycles desc → off (null) on the same column', () => {
    // Without this third state, users would have no way to clear the sort
    // and return rows to their original (insertion) order.
    const current: SortState<Col> = { col: 'name', dir: 'desc' };
    expect(nextSort(current, 'name')).toEqual({ col: null, dir: 'asc' });
  });

  it('starts at asc when there is no current sort', () => {
    const current: SortState<Col> = { col: null, dir: 'asc' };
    expect(nextSort(current, 'name')).toEqual({ col: 'name', dir: 'asc' });
  });
});
