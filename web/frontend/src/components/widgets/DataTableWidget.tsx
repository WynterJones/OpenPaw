import type { WidgetProps } from './WidgetRegistry';

export function DataTableWidget({ data }: WidgetProps) {
  const columns = (data.columns as string[]) || [];
  const rows = (data.rows as unknown[][]) || [];

  if (!columns.length) return null;

  return (
    <div className="rounded-lg border border-border-1 overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="bg-surface-2">
              {columns.map((col, i) => (
                <th key={i} className="px-3 py-2 text-left text-xs font-semibold text-text-2 whitespace-nowrap">
                  {col}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, ri) => (
              <tr key={ri} className="border-t border-border-1 hover:bg-surface-2/50 transition-colors">
                {row.map((cell, ci) => (
                  <td key={ci} className="px-3 py-2 text-text-1 whitespace-nowrap">
                    {String(cell)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
