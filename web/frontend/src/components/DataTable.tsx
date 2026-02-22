import type { ReactNode } from 'react';

interface Column<T> {
  key: string;
  header: string;
  render: (item: T) => ReactNode;
  className?: string;
  hideOnMobile?: boolean;
}

interface DataTableProps<T> {
  columns: Column<T>[];
  data: T[];
  keyExtractor: (item: T) => string;
  onRowClick?: (item: T) => void;
  emptyState?: ReactNode;
}

export function DataTable<T>({ columns, data, keyExtractor, onRowClick, emptyState }: DataTableProps<T>) {
  if (data.length === 0 && emptyState) {
    return <>{emptyState}</>;
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border-0">
            {columns.map(col => (
              <th
                key={col.key}
                scope="col"
                className={`text-left px-3 md:px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-3 ${col.hideOnMobile ? 'hidden md:table-cell' : ''} ${col.className || ''}`}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map(item => (
            <tr
              key={keyExtractor(item)}
              onClick={() => onRowClick?.(item)}
              onKeyDown={onRowClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onRowClick(item); } } : undefined}
              tabIndex={onRowClick ? 0 : undefined}
              className={`border-b border-border-0/50 transition-colors hover:bg-surface-2/50 ${onRowClick ? 'cursor-pointer focus:bg-surface-2/50 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-primary' : ''}`}
            >
              {columns.map(col => (
                <td key={col.key} className={`px-3 md:px-4 py-3 text-text-1 ${col.hideOnMobile ? 'hidden md:table-cell' : ''} ${col.className || ''}`}>
                  {col.render(item)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
