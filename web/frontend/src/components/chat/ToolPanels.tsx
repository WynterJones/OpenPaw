import { useState } from 'react';
import { ChevronDown, Loader2, Check, AlertCircle, Activity } from 'lucide-react';
import type { ToolCallResult } from '../../lib/api';
import { getToolDetail, groupBy, toolDisplayName, toolGroupKey, timeAgo, type StreamingTool } from '../../lib/chatUtils';

export function ToolActivityPanel({ tools, isStreaming, defaultExpanded }: {
  tools: ToolCallResult[];
  isStreaming?: boolean;
  defaultExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded ?? false);
  if (!tools.length) return null;

  const groups = groupBy(tools, t => toolGroupKey(t));
  const errorCount = tools.filter(t => t.status === 'error').length;
  const total = tools.length;

  return (
    <div className={`tool-panel-entrance rounded-lg border border-border-1 bg-transparent overflow-hidden my-1.5 ${expanded ? 'w-full' : 'w-fit max-w-full'}`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-1.5 text-left cursor-pointer hover:bg-surface-2/40 transition-colors"
      >
        {isStreaming ? (
          <Loader2 className="w-3.5 h-3.5 text-accent-primary animate-spin flex-shrink-0" />
        ) : errorCount > 0 ? (
          <AlertCircle className="w-3.5 h-3.5 text-red-400 flex-shrink-0" />
        ) : (
          <Check className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
        )}
        <span className="text-xs font-medium text-text-2 whitespace-nowrap">
          {total} tool call{total !== 1 ? 's' : ''}
        </span>
        {errorCount > 0 && (
          <span className="text-[11px] text-red-400 flex-shrink-0 whitespace-nowrap">
            {errorCount} error{errorCount !== 1 ? 's' : ''}
          </span>
        )}
        <ChevronDown className={`w-3 h-3 text-text-3 transition-transform flex-shrink-0 ${expanded ? 'rotate-180' : ''}`} />
      </button>
      {expanded && (
        <div className="border-t border-border-1 px-3 py-2 space-y-2">
          {groups.map(g => {
            const sample = g.items[0];
            const display = toolDisplayName(sample.tool_name, sample.endpoint);
            return (
              <div key={g.name}>
                <p className="text-[11px] font-medium text-text-3 uppercase tracking-wide mb-1">
                  {display} &times;{g.items.length}
                </p>
                <div className="space-y-0.5">
                  {g.items.map((call, i) => {
                    const detail = call.detail || getToolDetail(call.tool_name, call.input || {});
                    return (
                      <div key={i} className={`flex items-center gap-2 text-xs px-2 py-1 rounded ${call.status === 'error' ? 'bg-red-500/5 text-red-400' : 'text-text-2'}`}>
                        {call.status === 'error' ? (
                          <AlertCircle className="w-3 h-3 flex-shrink-0" />
                        ) : (
                          <Check className="w-3 h-3 text-text-3 flex-shrink-0" />
                        )}
                        <span className="flex-shrink-0 max-w-[45%] truncate text-text-2">{toolDisplayName(call.tool_name, call.endpoint)}</span>
                        {detail && <span className="truncate flex-1 min-w-0 font-mono text-text-3">{detail}</span>}
                        {call.timestamp && <span className="text-text-3 text-[10px] flex-shrink-0">{timeAgo(call.timestamp)}</span>}
                      </div>
                    );
                  })}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function StreamingToolPanel({ tools }: { tools: StreamingTool[] }) {
  const [expanded, setExpanded] = useState(false);
  if (!tools.length) return null;

  const groups = groupBy(tools, t => toolGroupKey(t));
  const running = tools.filter(t => !t.done).length;
  const allDone = running === 0;
  const total = tools.length;

  // The step to name on the second line: whatever is running now, or the last
  // thing that ran once everything is done. Collapsed, the badge otherwise
  // says only "1 running", which tells you nothing about what it is doing.
  const current = tools.filter(t => !t.done).at(-1) ?? tools.at(-1);
  const currentLabel = current ? toolDisplayName(current.name, current.endpoint) : '';

  return (
    <div className={`tool-panel-entrance rounded-lg border border-border-1 bg-transparent overflow-hidden my-1.5 ${expanded ? 'w-full' : 'w-fit max-w-full'}`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex flex-col items-start gap-0.5 px-3 py-1.5 text-left cursor-pointer hover:bg-surface-2/40 transition-colors"
      >
        <span className="flex items-center gap-2 w-full">
          {allDone ? (
            <Check className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
          ) : (
            <Activity className="w-3.5 h-3.5 text-accent-primary flex-shrink-0" />
          )}
          <span className="text-xs font-medium text-text-2 whitespace-nowrap">
            {allDone ? `${total} tool call${total !== 1 ? 's' : ''}` : 'Activity'}
          </span>
          {running > 0 && (
            <span className="inline-flex items-center gap-1 text-[11px] text-accent-primary flex-shrink-0 whitespace-nowrap">
              <Loader2 className="w-2.5 h-2.5 animate-spin" />
              {running} running
            </span>
          )}
          <ChevronDown className={`w-3 h-3 text-text-3 transition-transform flex-shrink-0 ml-auto ${expanded ? 'rotate-180' : ''}`} />
        </span>

        {currentLabel && (
          // Keyed on the tool id so React remounts it on every change, which is
          // what replays the entrance animation.
          <span
            key={current?.id ?? currentLabel}
            className={`op-tool-swap block max-w-full truncate pl-[22px] text-[11px] leading-tight ${
              allDone ? 'text-text-3' : 'op-shimmer-text'
            }`}
            title={current?.detail || currentLabel}
          >
            {currentLabel}
            {current?.detail ? <span className="font-mono"> · {current.detail}</span> : null}
          </span>
        )}
      </button>
      {expanded && (
        <div className="border-t border-border-1 px-3 py-2 space-y-2">
          {groups.map(g => {
            const sample = g.items[0];
            const display = toolDisplayName(sample.name, sample.endpoint);
            return (
              <div key={g.name}>
                <p className="text-[11px] font-medium text-text-3 uppercase tracking-wide mb-1">
                  {display} &times;{g.items.length}
                </p>
                <div className="space-y-0.5">
                  {g.items.map(tool => (
                    <div key={tool.id} className="flex items-center gap-2 text-xs px-2 py-1 rounded text-text-2">
                      {!tool.done ? (
                        <Loader2 className="w-3 h-3 text-accent-primary animate-spin flex-shrink-0" />
                      ) : (
                        <Check className="w-3 h-3 text-text-3 flex-shrink-0" />
                      )}
                      <span className="flex-shrink-0 max-w-[45%] truncate text-text-2">{toolDisplayName(tool.name, tool.endpoint)}</span>
                      {tool.detail && (
                        <span className="truncate flex-1 min-w-0 font-mono text-text-3">{tool.detail}</span>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
