// Shared sortable column header. Click cycles asc → desc → off (back to insertion order).

import type { ReactNode } from 'react';

export type SortState<K extends string> = { col: K | null; dir: 'asc' | 'desc' };

export function nextSort<K extends string>(current: SortState<K>, col: K): SortState<K> {
  if (current.col !== col) return { col, dir: 'asc' };
  if (current.dir === 'asc') return { col, dir: 'desc' };
  return { col: null, dir: 'asc' };
}

export function SortHeader<K extends string>({
  col, label, sort, onSort, align = 'start',
}: {
  col: K;
  label: ReactNode;
  sort: SortState<K>;
  onSort: (col: K) => void;
  align?: 'start' | 'end' | 'center';
}) {
  const active = sort.col === col;
  const arrow = !active ? '' : sort.dir === 'asc' ? '▲' : '▼';
  return (
    <th
      className={`text-${align} user-select-none`}
      role="button"
      onClick={() => onSort(col)}
      style={{ cursor: 'pointer', whiteSpace: 'nowrap' }}
    >
      {label}
      <span className="text-secondary ms-1" style={{ fontSize: 10 }}>{arrow}</span>
    </th>
  );
}
