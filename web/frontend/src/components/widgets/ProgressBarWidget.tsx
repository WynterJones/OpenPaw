import type { WidgetProps } from './WidgetRegistry';

export function ProgressBarWidget({ data }: WidgetProps) {
  const label = (data.label as string) || '';
  const value = Number(data.value) || 0;
  const max = Number(data.max) || 100;
  const pct = Math.min(100, Math.max(0, (value / max) * 100));

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-xs">
        <span className="text-text-2">{label}</span>
        <span className="text-text-1 font-medium">{Math.round(pct)}%</span>
      </div>
      <div className="h-2.5 rounded-full bg-surface-2 overflow-hidden">
        <div
          className="h-full rounded-full transition-all duration-500"
          style={{ width: `${pct}%`, background: 'var(--op-accent)' }}
        />
      </div>
    </div>
  );
}
