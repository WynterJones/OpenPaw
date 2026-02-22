import { ResponsiveContainer, LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts';
import type { WidgetProps } from './WidgetRegistry';

const COLORS = ['var(--op-accent)', '#f472b6', '#60a5fa', '#34d399', '#fbbf24', '#a78bfa'];

export function LineChartWidget({ data, config }: WidgetProps) {
  const chartData = Array.isArray(data) ? data : (data.points as Record<string, unknown>[]) || [];
  const xKey = (config?.xKey as string) || 'name';
  const yKeys = (config?.yKeys as string[]) || Object.keys(chartData[0] || {}).filter(k => k !== xKey);
  const showGrid = config?.showGrid !== false;

  if (chartData.length === 0) {
    return <div className="h-48 flex items-center justify-center text-xs text-text-3">No data</div>;
  }

  return (
    <ResponsiveContainer width="100%" height={200}>
      <LineChart data={chartData as Record<string, unknown>[]}>
        {showGrid && <CartesianGrid strokeDasharray="3 3" stroke="var(--op-border-0)" />}
        <XAxis dataKey={xKey} tick={{ fontSize: 11, fill: 'var(--op-text-3)' }} stroke="var(--op-border-0)" />
        <YAxis tick={{ fontSize: 11, fill: 'var(--op-text-3)' }} stroke="var(--op-border-0)" />
        <Tooltip
          contentStyle={{ background: 'var(--op-surface-2)', border: '1px solid var(--op-border-1)', borderRadius: 8, fontSize: 12, color: 'var(--op-text-1)' }}
        />
        {yKeys.map((key, i) => (
          <Line key={key} type="monotone" dataKey={key} stroke={COLORS[i % COLORS.length]} strokeWidth={2} dot={false} />
        ))}
      </LineChart>
    </ResponsiveContainer>
  );
}
