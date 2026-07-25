/**
 * ActiveChatsIndicator
 *
 * A global bottom-right sticky that lists every chat currently thinking/working
 * — across all workspaces — so you can see and jump to active chats from any
 * screen. Polls the backend (source of truth = per-thread cancel funcs) and
 * hides itself when nothing is active. Mounted once in Layout.
 */

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import { Loader2 } from 'lucide-react';
import { api } from '../lib/api';

interface ActiveChat {
  thread_id: string;
  title: string;
  workspace_id?: string;
  agent_slug?: string;
  streaming: boolean;
}

export function ActiveChatsIndicator() {
  const [chats, setChats] = useState<ActiveChat[]>([]);
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const data = await api.get<ActiveChat[]>('/chat/active');
        if (!cancelled) setChats(Array.isArray(data) ? data : []);
      } catch {
        if (!cancelled) setChats([]);
      }
      // Poll faster while chats are active, slower when idle.
      if (!cancelled) timer = setTimeout(poll, chats.length > 0 ? 2000 : 4000);
    };
    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (chats.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-30 w-64 max-w-[calc(100vw-2rem)] pointer-events-none">
      <div className="pointer-events-auto rounded-xl border border-border-1 bg-surface-1/95 backdrop-blur-md shadow-xl shadow-black/20 overflow-hidden">
        <div className="px-3 py-2 border-b border-border-0 flex items-center gap-2">
          <Loader2 className="w-3.5 h-3.5 text-accent-primary animate-spin flex-shrink-0" aria-hidden="true" />
          <span className="text-xs font-semibold text-text-1">
            {chats.length} chat{chats.length > 1 ? 's' : ''} working
          </span>
        </div>
        <div className="max-h-56 overflow-y-auto">
          {chats.map((c) => (
            <button
              key={c.thread_id}
              onClick={() => navigate(`/chat/${c.thread_id}`)}
              className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-surface-2 transition-colors cursor-pointer"
              title={`Open "${c.title}"`}
            >
              <span className="w-1.5 h-1.5 rounded-full bg-accent-primary animate-pulse flex-shrink-0" aria-hidden="true" />
              <span className="flex-1 min-w-0 text-xs text-text-1 truncate">{c.title}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
