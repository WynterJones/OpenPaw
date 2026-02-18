import { AlertCircle, Loader2 } from 'lucide-react';
import { Card } from './Card';
import { LineChartWidget } from './widgets/LineChartWidget';
import { BarChartWidget } from './widgets/BarChartWidget';
import { AreaChartWidget } from './widgets/AreaChartWidget';
import { PieChartWidget } from './widgets/PieChartWidget';
import { MetricCardWidget } from './widgets/MetricCardWidget';
import { StatusCardWidget } from './widgets/StatusCardWidget';
import { DataTableWidget } from './widgets/DataTableWidget';
import { KeyValueWidget } from './widgets/KeyValueWidget';
import { TextBlockWidget } from './widgets/TextBlockWidget';
import { ProgressBarWidget } from './widgets/ProgressBarWidget';
import { JsonViewerWidget } from './widgets/JsonViewerWidget';
import type { DashboardLayout, DashboardWidgetConfig } from '../lib/api';

interface DashboardGridProps {
  layout: DashboardLayout;
  widgets: DashboardWidgetConfig[];
  widgetData: Record<string, unknown>;
  loading: boolean;
}

function WidgetContent({ widget, data }: { widget: DashboardWidgetConfig; data: unknown }) {
  const widgetData = (data ?? widget.data ?? {}) as Record<string, unknown>;
  const config = widget.config;

  if ((widgetData as Record<string, unknown>)?.error) {
    return (
      <div className="h-full flex items-center justify-center text-xs text-text-3 gap-1.5">
        <AlertCircle className="w-3.5 h-3.5 text-danger" />
        <span>{String((widgetData as Record<string, unknown>).error)}</span>
      </div>
    );
  }

  switch (widget.type) {
    case 'line-chart':
      return <LineChartWidget data={widgetData} config={config} />;
    case 'bar-chart':
      return <BarChartWidget data={widgetData} config={config} />;
    case 'area-chart':
      return <AreaChartWidget data={widgetData} config={config} />;
    case 'pie-chart':
      return <PieChartWidget data={widgetData} config={config} />;
    case 'metric-card':
      return <MetricCardWidget data={widgetData} />;
    case 'status-card':
      return <StatusCardWidget data={widgetData} />;
    case 'data-table':
      return <DataTableWidget data={widgetData} />;
    case 'key-value':
      return <KeyValueWidget data={widgetData} />;
    case 'text-block':
      return <TextBlockWidget data={widgetData} />;
    case 'progress-bar':
      return <ProgressBarWidget data={widgetData} />;
    default:
      return <JsonViewerWidget data={widgetData} />;
  }
}

export function DashboardGrid({ layout, widgets, widgetData, loading }: DashboardGridProps) {
  const columns = layout?.columns || 3;
  const gap = layout?.gap || 'md';

  const gapClass = gap === 'sm' ? 'gap-2' : gap === 'lg' ? 'gap-6' : 'gap-4';

  const maxRow = widgets.reduce((max, w) => {
    const rowEnd = (w.position?.row || 0) + (w.position?.rowSpan || 1);
    return Math.max(max, rowEnd);
  }, 1);

  return (
    <div
      className={`grid ${gapClass}`}
      style={{
        gridTemplateColumns: `repeat(${columns}, 1fr)`,
        gridTemplateRows: `repeat(${maxRow}, minmax(180px, auto))`,
      }}
    >
      {widgets.map(widget => {
        const col = widget.position?.col ?? 0;
        const row = widget.position?.row ?? 0;
        const colSpan = widget.position?.colSpan ?? 1;
        const rowSpan = widget.position?.rowSpan ?? 1;

        return (
          <Card
            key={widget.id}
            className="overflow-hidden flex flex-col"
            style={{
              gridColumn: `${col + 1} / span ${colSpan}`,
              gridRow: `${row + 1} / span ${rowSpan}`,
            }}
          >
            <div className="flex items-center justify-between px-1 mb-2 flex-shrink-0">
              <h4 className="text-xs font-semibold text-text-1 truncate">{widget.title}</h4>
              {loading && <Loader2 className="w-3 h-3 text-text-3 animate-spin flex-shrink-0" />}
            </div>
            <div className="flex-1 min-h-0">
              <WidgetContent widget={widget} data={widgetData[widget.id]} />
            </div>
          </Card>
        );
      })}
    </div>
  );
}
