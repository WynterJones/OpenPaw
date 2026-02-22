import { useState } from 'react';
import { ChevronRight, Wrench } from 'lucide-react';
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
import { AudioPlayerWidget } from './AudioPlayerWidget';
import { VideoPlayerWidget } from './VideoPlayerWidget';
import { FilePreviewWidget } from './FilePreviewWidget';
import { detectBestWidget } from './detectWidget';
import type { WidgetPayload } from '../../lib/api';

const EXPAND_BY_DEFAULT = new Set([
  'audio-player',
  'video-player',
  'image',
  'metric-card',
  'status-card',
  'progress-bar',
  'text-block',
  'file-preview',
  'line-chart',
  'bar-chart',
  'area-chart',
  'pie-chart',
]);

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
      return <ImageWidget data={{ ...data, __resolved_tool_id: tool_id }} />;
    case 'audio-player':
      return <AudioPlayerWidget data={{ ...data, __resolved_tool_id: tool_id }} />;
    case 'video-player':
      return <VideoPlayerWidget data={{ ...data, __resolved_tool_id: tool_id }} />;
    case 'file-preview':
      return <FilePreviewWidget data={{ ...data, __resolved_tool_id: tool_id }} />;
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

function resolveWidget(widget: WidgetPayload): { type: string; element: React.ReactElement } {
  const declared = renderWidgetByType(widget.type, widget);
  if (declared) return { type: widget.type, element: declared };

  const detected = detectBestWidget(widget.data);
  if (detected !== widget.type) {
    const fallback = renderWidgetByType(detected, widget);
    if (fallback) return { type: detected, element: fallback };
  }

  return { type: 'json-viewer', element: <JsonViewerWidget data={widget.data} /> };
}

function dataPreview(data: Record<string, unknown>): string {
  const keys = Object.keys(data).filter(k => !k.startsWith('__'));
  if (keys.length === 0) return '';
  const shown = keys.slice(0, 4).join(', ');
  return keys.length > 4 ? `${shown} + ${keys.length - 4} more` : shown;
}

function CollapsibleWidget({ widget, resolvedType, children }: {
  widget: WidgetPayload;
  resolvedType: string;
  children: React.ReactElement;
}) {
  const expandDefault = EXPAND_BY_DEFAULT.has(resolvedType);
  const [expanded, setExpanded] = useState(expandDefault);

  const label = widget.title || 'Tool Response';
  const endpoint = widget.endpoint;
  const preview = !expanded ? dataPreview(widget.data) : '';

  return (
    <div className="my-2 rounded-lg border border-border-1 bg-surface-1/50 overflow-hidden">
      <button
        onClick={() => setExpanded(v => !v)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-surface-2/50 transition-colors cursor-pointer"
      >
        <ChevronRight
          className={`w-3.5 h-3.5 text-text-3 flex-shrink-0 transition-transform ${expanded ? 'rotate-90' : ''}`}
        />
        <Wrench className="w-3.5 h-3.5 text-accent-primary flex-shrink-0" />
        <span className="text-xs font-medium text-text-1 truncate">{label}</span>
        {endpoint && (
          <code className="text-[10px] font-mono text-accent-primary bg-accent-primary/10 px-1.5 py-0.5 rounded flex-shrink-0">
            {endpoint}
          </code>
        )}
        {preview && (
          <span className="text-[10px] text-text-3 truncate ml-auto">{preview}</span>
        )}
      </button>
      {expanded && (
        <div className="px-3 pb-3 pt-1">
          {children}
        </div>
      )}
    </div>
  );
}

export function WidgetRenderer({ widget }: { widget: WidgetPayload }) {
  const { type: resolvedType, element } = resolveWidget(widget);

  if (EXPAND_BY_DEFAULT.has(resolvedType) && !widget.endpoint) {
    return (
      <div className="my-2">
        {widget.title && <p className="text-xs font-semibold text-text-2 mb-1 px-1">{widget.title}</p>}
        {element}
      </div>
    );
  }

  return (
    <CollapsibleWidget widget={widget} resolvedType={resolvedType}>
      {element}
    </CollapsibleWidget>
  );
}
