import { TrendingUp, TrendingDown, Minus } from 'lucide-react';
import type { WidgetProps } from './WidgetRegistry';

export function MetricCardWidget({ data }: WidgetProps) {
  const label = (data.label as string) || '';
  const value = data.value;
  const unit = (data.unit as string) || '';
  const trend = (data.trend as string) || '';

  const TrendIcon = trend === 'up' ? TrendingUp : trend === 'down' ? TrendingDown : Minus;
  const trendColor = trend === 'up' ? 'text-emerald-400' : trend === 'down' ? 'text-red-400' : 'text-text-3';

  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/30 px-4 py-3 inline-flex flex-col gap-1">
      <span className="text-xs text-text-2">{label}</span>
      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-bold text-text-0">{String(value)}</span>
        {unit && <span className="text-sm text-text-2">{unit}</span>}
        {trend && <TrendIcon className={`w-4 h-4 ${trendColor}`} />}
      </div>
    </div>
  );
}
