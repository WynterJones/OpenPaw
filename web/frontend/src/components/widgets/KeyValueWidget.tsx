import type { WidgetProps } from './WidgetRegistry';

export function KeyValueWidget({ data }: WidgetProps) {
  let entries: [string, unknown][];

  if (Array.isArray(data.entries)) {
    entries = data.entries as [string, unknown][];
  } else {
    entries = Object.entries(data).filter(([k]) => k !== 'entries');
  }

  if (!entries.length) return null;

  return (
    <div className="rounded-lg border border-border-1 overflow-hidden">
      <div className="divide-y divide-border-1">
        {entries.map(([key, value], i) => (
          <div key={i} className="flex flex-col sm:flex-row sm:items-center px-3 py-2 hover:bg-surface-2/50 transition-colors gap-0.5 sm:gap-0">
            <span className="text-xs font-medium text-text-3 sm:text-text-2 sm:w-32 flex-shrink-0">{key}</span>
            <span className="text-sm text-text-1 font-mono break-all">{String(value)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
