import { DataTableWidget } from './DataTableWidget';
import { KeyValueWidget } from './KeyValueWidget';
import { JsonViewerWidget } from './JsonViewerWidget';
import { MetricCardWidget } from './MetricCardWidget';
import { StatusCardWidget } from './StatusCardWidget';
import { CustomWidget } from './CustomWidget';
import { LineChartWidget } from './LineChartWidget';
import { BarChartWidget } from './BarChartWidget';
import { AreaChartWidget } from './AreaChartWidget';
import { PieChartWidget } from './PieChartWidget';
import { ProgressBarWidget } from './ProgressBarWidget';
import { TextBlockWidget } from './TextBlockWidget';
import { ImageWidget } from './ImageWidget';
import { detectBestWidget } from './detectWidget';
import type { WidgetPayload } from '../../lib/api';

function WidgetTitle({ title }: { title?: string }) {
  if (!title) return null;
  return <p className="text-xs font-semibold text-text-2 mb-1 px-1">{title}</p>;
}

function renderWidgetByType(type: string, widget: WidgetPayload): React.ReactElement | null {
  const { data, config, tool_id } = widget;

  switch (type) {
    case 'data-table':
      if (!data.columns || !data.rows) return null;
      return <DataTableWidget data={data} />;
    case 'key-value':
      return <KeyValueWidget data={data} />;
    case 'metric-card':
      if (!data.label && !data.value) return null;
      return <MetricCardWidget data={data} />;
    case 'status-card':
      if (!data.label && !data.status) return null;
      return <StatusCardWidget data={data} />;
    case 'image':
      return <ImageWidget data={data} />;
    case 'json-viewer':
      return <JsonViewerWidget data={data} />;
    case 'line-chart':
      return <LineChartWidget data={data} config={config} />;
    case 'bar-chart':
      return <BarChartWidget data={data} config={config} />;
    case 'area-chart':
      return <AreaChartWidget data={data} config={config} />;
    case 'pie-chart':
      return <PieChartWidget data={data} config={config} />;
    case 'progress-bar':
      return <ProgressBarWidget data={data} />;
    case 'text-block':
      return <TextBlockWidget data={data} />;
    case 'custom':
      if (tool_id) return <CustomWidget toolId={tool_id} data={data} />;
      return null;
    default:
      return null;
  }
}

function BuiltInWidget({ widget }: { widget: WidgetPayload }) {
  // Try declared type first
  const declared = renderWidgetByType(widget.type, widget);
  if (declared) return declared;

  // Fallback: auto-detect from data shape
  const detected = detectBestWidget(widget.data);
  if (detected !== widget.type) {
    const fallback = renderWidgetByType(detected, widget);
    if (fallback) return fallback;
  }

  // Ultimate fallback
  return <JsonViewerWidget data={widget.data} />;
}

export function WidgetRenderer({ widget }: { widget: WidgetPayload }) {
  return (
    <div className="my-2">
      <WidgetTitle title={widget.title} />
      <BuiltInWidget widget={widget} />
    </div>
  );
}
