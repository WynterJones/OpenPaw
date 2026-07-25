/**
 * ActiveTerminalsIndicator
 *
 * Lists every live terminal across all workbenches and workspaces so you can
 * jump back to one from any screen. Rendered inside the bottom-right sticky
 * stack, directly above the active-chats card — and in its place when no chat
 * is working, since each card hides itself when empty.
 */

import { useEffect, useState } from 'react';
import { TerminalSquare } from 'lucide-react';
import { api } from '../lib/api';
import { jumpToWorkspace } from '../lib/jumpToWorkspace';

interface ActiveTerminal {
  session_id: string;
  title: string;
  workbench_id?: string;
  workspace_id?: string;
}

export function ActiveTerminalsIndicator() {
  const [sessions, setSessions] = useState<ActiveTerminal[]>([]);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const data = await api.get<ActiveTerminal[]>('/terminal/active');
        if (!cancelled) setSessions(Array.isArray(data) ? data : []);
      } catch {
        if (!cancelled) setSessions([]);
      }
      if (!cancelled) timer = setTimeout(poll, 5000);
    };
    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, []);

  if (sessions.length === 0) return null;

  // Positioned by the shared stack in Layout, so no fixed offsets here.
  return (
    <div className="w-64 max-w-[calc(100vw-2rem)]">
      <div className="pointer-events-auto rounded-xl border border-border-1 bg-surface-1/95 backdrop-blur-md shadow-xl shadow-black/20 overflow-hidden">
        <div className="px-3 py-2 border-b border-border-0 flex items-center gap-2">
          <TerminalSquare className="w-3.5 h-3.5 text-accent-primary flex-shrink-0" aria-hidden="true" />
          <span className="text-xs font-semibold text-text-1">
            {sessions.length} terminal{sessions.length > 1 ? 's' : ''} open
          </span>
        </div>
        <div className="max-h-56 overflow-y-auto">
          {sessions.map((s) => (
            <button
              key={s.session_id}
              onClick={() => jumpToWorkspace(s.workspace_id, '/workbench')}
              className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-surface-2 transition-colors cursor-pointer"
              title={`Open "${s.title}"`}
            >
              <span className="w-1.5 h-1.5 rounded-full bg-accent-primary flex-shrink-0" aria-hidden="true" />
              <span className="flex-1 min-w-0 text-xs text-text-1 truncate">{s.title}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
