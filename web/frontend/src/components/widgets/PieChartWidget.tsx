import { ResponsiveContainer, PieChart, Pie, Cell, Tooltip } from 'recharts';
import type { WidgetProps } from './WidgetRegistry';

const COLORS = ['var(--op-accent)', '#f472b6', '#60a5fa', '#34d399', '#fbbf24', '#a78bfa', '#fb923c', '#e879f9'];

export function PieChartWidget({ data, config }: WidgetProps) {
  const chartData = Array.isArray(data) ? data : (data.items as Record<string, unknown>[]) || [];
  const nameKey = (config?.nameKey as string) || 'name';
  const valueKey = (config?.valueKey as string) || 'value';

  if (chartData.length === 0) {
    return <div className="h-48 flex items-center justify-center text-xs text-text-3">No data</div>;
  }

  return (
    <ResponsiveContainer width="100%" height={200}>
      <PieChart>
        <Pie
          data={chartData as Record<string, unknown>[]}
          dataKey={valueKey}
          nameKey={nameKey}
          cx="50%"
          cy="50%"
          outerRadius={80}
          innerRadius={40}
          paddingAngle={2}
        >
          {(chartData as Record<string, unknown>[]).map((_, i) => (
            <Cell key={i} fill={COLORS[i % COLORS.length]} />
          ))}
        </Pie>
        <Tooltip
          contentStyle={{ background: 'var(--op-surface-2)', border: '1px solid var(--op-border-1)', borderRadius: 8, fontSize: 12, color: 'var(--op-text-1)' }}
        />
      </PieChart>
    </ResponsiveContainer>
  );
}
