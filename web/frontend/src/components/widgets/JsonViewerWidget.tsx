import { useState } from 'react';
import { ChevronRight } from 'lucide-react';
import type { WidgetProps } from './WidgetRegistry';

function JsonNode({ label, value, depth = 0 }: { label?: string; value: unknown; depth?: number }) {
  const [open, setOpen] = useState(depth < 2);

  if (value === null || value === undefined) {
    return (
      <div className="flex items-center gap-1" style={{ paddingLeft: depth * 16 }}>
        {label && <span className="text-text-2">{label}:</span>}
        <span className="text-text-3 italic">null</span>
      </div>
    );
  }

  if (typeof value === 'object' && !Array.isArray(value)) {
    const entries = Object.entries(value as Record<string, unknown>);
    return (
      <div style={{ paddingLeft: depth * 16 }}>
        <button onClick={() => setOpen(!open)} className="flex items-center gap-1 cursor-pointer hover:bg-surface-2/50 rounded px-1 -ml-1">
          <ChevronRight className={`w-3 h-3 text-text-3 transition-transform ${open ? 'rotate-90' : ''}`} />
          {label && <span className="text-text-2">{label}</span>}
          <span className="text-text-3 text-xs">{`{${entries.length}}`}</span>
        </button>
        {open && entries.map(([k, v]) => (
          <JsonNode key={k} label={k} value={v} depth={depth + 1} />
        ))}
      </div>
    );
  }

  if (Array.isArray(value)) {
    return (
      <div style={{ paddingLeft: depth * 16 }}>
        <button onClick={() => setOpen(!open)} className="flex items-center gap-1 cursor-pointer hover:bg-surface-2/50 rounded px-1 -ml-1">
          <ChevronRight className={`w-3 h-3 text-text-3 transition-transform ${open ? 'rotate-90' : ''}`} />
          {label && <span className="text-text-2">{label}</span>}
          <span className="text-text-3 text-xs">[{value.length}]</span>
        </button>
        {open && value.map((item, i) => (
          <JsonNode key={i} label={String(i)} value={item} depth={depth + 1} />
        ))}
      </div>
    );
  }

  const color = typeof value === 'string' ? 'text-emerald-400' :
    typeof value === 'number' ? 'text-blue-400' :
    typeof value === 'boolean' ? 'text-amber-400' : 'text-text-1';

  return (
    <div className="flex items-center gap-1" style={{ paddingLeft: depth * 16 }}>
      {label && <span className="text-text-2">{label}:</span>}
      <span className={`font-mono text-xs ${color}`}>
        {typeof value === 'string' ? `"${value}"` : String(value)}
      </span>
    </div>
  );
}

export function JsonViewerWidget({ data }: WidgetProps) {
  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/30 p-3 text-xs font-mono overflow-x-auto max-h-80 overflow-y-auto">
      <JsonNode value={data} />
    </div>
  );
}
