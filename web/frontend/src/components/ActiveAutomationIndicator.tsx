/**
 * ActiveAutomationIndicator
 *
 * Shows scheduled routines and agent heartbeats that are running right now, in
 * the same bottom-right sticky stack as terminals and chats. Unlike those two
 * this card is deliberately inert — background automation has no screen to jump
 * to mid-run, so it only answers "is the system working?" and stays out of the
 * way. Hides itself when nothing is running.
 */

import { useEffect, useState } from 'react';
import { CalendarClock, HeartPulse } from 'lucide-react';
import { api } from '../lib/api';

interface RunningAutomation {
  kind: 'schedule' | 'heartbeat';
  id: string;
  label: string;
  detail?: string;
  started_at: string;
}

export function ActiveAutomationIndicator() {
  const [runs, setRuns] = useState<RunningAutomation[]>([]);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const data = await api.get<RunningAutomation[]>('/automation/active');
        if (!cancelled) setRuns(Array.isArray(data) ? data : []);
      } catch {
        if (!cancelled) setRuns([]);
      }
      if (!cancelled) timer = setTimeout(poll, 5000);
    };
    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, []);

  if (runs.length === 0) return null;

  // Positioned by the shared stack in Layout, so no fixed offsets here.
  return (
    <div className="w-64 max-w-[calc(100vw-2rem)]">
      <div className="pointer-events-auto rounded-xl border border-border-1 bg-surface-1/95 backdrop-blur-md shadow-xl shadow-black/20 overflow-hidden">
        <div className="px-3 py-2 border-b border-border-0 flex items-center gap-2">
          <CalendarClock className="w-3.5 h-3.5 text-accent-primary flex-shrink-0" aria-hidden="true" />
          <span className="text-xs font-semibold text-text-1">
            {runs.length} automation{runs.length > 1 ? 's' : ''} running
          </span>
        </div>
        <div className="max-h-56 overflow-y-auto">
          {runs.map((run) => {
            const Icon = run.kind === 'heartbeat' ? HeartPulse : CalendarClock;
            return (
              <div key={run.id} className="w-full flex items-center gap-2 px-3 py-2">
                <Icon className="w-3 h-3 text-accent-primary animate-pulse flex-shrink-0" aria-hidden="true" />
                <span className="flex-1 min-w-0 text-xs text-text-1 truncate" title={run.label}>
                  {run.label}
                </span>
                {run.detail && (
                  <span className="text-[10px] text-text-3 truncate max-w-[5.5rem]" title={run.detail}>
                    {run.detail}
                  </span>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
