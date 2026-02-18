import { useState } from 'react';
import { ChevronDown, Loader2, Check, AlertCircle, Activity } from 'lucide-react';
import type { ToolCallResult } from '../../lib/api';
import { getToolDetail, groupBy, type StreamingTool } from '../../lib/chatUtils';

export function ToolActivityPanel({ tools, isStreaming }: {
  tools: ToolCallResult[];
  isStreaming?: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  if (!tools.length) return null;

  const groups = groupBy(tools, t => t.tool_name);
  const errorCount = tools.filter(t => t.status === 'error').length;
  const total = tools.length;

  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/50 overflow-hidden my-2">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left cursor-pointer hover:bg-surface-2 transition-colors"
      >
        {isStreaming ? (
          <Loader2 className="w-3.5 h-3.5 text-accent-primary animate-spin flex-shrink-0" />
        ) : errorCount > 0 ? (
          <AlertCircle className="w-3.5 h-3.5 text-red-400 flex-shrink-0" />
        ) : (
          <Check className="w-3.5 h-3.5 text-emerald-400 flex-shrink-0" />
        )}
        <span className="text-xs font-medium text-text-2">
          {total} tool call{total !== 1 ? 's' : ''}
        </span>
        <div className="flex items-center gap-1.5 flex-1 flex-wrap">
          {groups.map(g => (
            <span key={g.name} className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-surface-3 text-[11px] text-text-2">
              {g.name}
              {g.items.length > 1 && <span className="text-text-3">&times;{g.items.length}</span>}
            </span>
          ))}
          {errorCount > 0 && (
            <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-red-500/10 text-[11px] text-red-400">
              {errorCount} error{errorCount !== 1 ? 's' : ''}
            </span>
          )}
        </div>
        <ChevronDown className={`w-3 h-3 text-text-3 transition-transform flex-shrink-0 ${expanded ? 'rotate-180' : ''}`} />
      </button>
      {expanded && (
        <div className="border-t border-border-1 px-3 py-2 space-y-2">
          {groups.map(g => (
            <div key={g.name}>
              <p className="text-[11px] font-medium text-text-3 uppercase tracking-wide mb-1">
                {g.name} &times;{g.items.length}
              </p>
              <div className="space-y-0.5">
                {g.items.map((call, i) => {
                  const detail = getToolDetail(call.tool_name, call.input);
                  return (
                    <div key={i} className={`flex items-center gap-2 text-xs px-2 py-1 rounded ${call.status === 'error' ? 'bg-red-500/5 text-red-400' : 'text-text-2'}`}>
                      {call.status === 'error' ? (
                        <AlertCircle className="w-3 h-3 flex-shrink-0" />
                      ) : (
                        <Check className="w-3 h-3 text-emerald-400 flex-shrink-0" />
                      )}
                      <span className="flex-shrink-0 w-12 text-text-3">{call.tool_name}</span>
                      {detail && <span className="truncate flex-1 font-mono">{detail}</span>}
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function StreamingToolPanel({ tools }: { tools: StreamingTool[] }) {
  const [expanded, setExpanded] = useState(false);
  if (!tools.length) return null;

  const groups = groupBy(tools, t => t.name);
  const running = tools.filter(t => !t.done).length;
  const allDone = running === 0;
  const total = tools.length;

  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/50 overflow-hidden my-2">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left cursor-pointer hover:bg-surface-2 transition-colors"
      >
        {allDone ? (
          <Check className="w-3.5 h-3.5 text-emerald-400 flex-shrink-0" />
        ) : (
          <Activity className="w-3.5 h-3.5 text-accent-primary flex-shrink-0" />
        )}
        <span className="text-xs font-medium text-text-2">
          {allDone ? `${total} tool call${total !== 1 ? 's' : ''}` : 'Activity'}
        </span>
        <div className="flex items-center gap-1.5 flex-1 flex-wrap">
          {groups.map(g => (
            <span key={g.name} className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-surface-3 text-[11px] text-text-2">
              {g.name}
              {g.items.length > 1 && <span className="text-text-3">&times;{g.items.length}</span>}
            </span>
          ))}
          {running > 0 && (
            <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-accent-primary/10 text-[11px] text-accent-primary">
              <Loader2 className="w-2.5 h-2.5 animate-spin" />
              {running} running
            </span>
          )}
        </div>
        <ChevronDown className={`w-3 h-3 text-text-3 transition-transform flex-shrink-0 ${expanded ? 'rotate-180' : ''}`} />
      </button>
      {expanded && (
        <div className="border-t border-border-1 px-3 py-2 space-y-2">
          {groups.map(g => (
            <div key={g.name}>
              <p className="text-[11px] font-medium text-text-3 uppercase tracking-wide mb-1">
                {g.name} &times;{g.items.length}
              </p>
              <div className="space-y-0.5">
                {g.items.map(tool => (
                  <div key={tool.id} className="flex items-center gap-2 text-xs px-2 py-1 rounded text-text-2">
                    {!tool.done ? (
                      <Loader2 className="w-3 h-3 text-accent-primary animate-spin flex-shrink-0" />
                    ) : (
                      <Check className="w-3 h-3 text-emerald-400 flex-shrink-0" />
                    )}
                    <span className="flex-shrink-0 w-12 text-text-3">{tool.name}</span>
                    {tool.detail && (
                      <span className="truncate flex-1 font-mono text-text-2">{tool.detail}</span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
