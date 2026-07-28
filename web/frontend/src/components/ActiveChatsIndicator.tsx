/**
 * ActiveChatsIndicator
 *
 * A global floating sticky that lists every chat currently thinking/working
 * — across all workspaces — so you can see and jump to active chats from any
 * screen. Polls the backend (source of truth = per-thread cancel funcs) and
 * hides itself when nothing is active. Mounted once in Layout.
 */

import { useEffect, useState } from 'react';
import { Loader2, Square } from 'lucide-react';
import { api } from '../lib/api';
import { jumpToWorkspace } from '../lib/jumpToWorkspace';
import { useDockCount } from '../contexts/activityDock';

interface ActiveChat {
  thread_id: string;
  title: string;
  workspace_id?: string;
  agent_slug?: string;
  streaming: boolean;
}

export function ActiveChatsIndicator() {
  const [chats, setChats] = useState<ActiveChat[]>([]);

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

  useDockCount('chats', chats.length);

  if (chats.length === 0) return null;

  return (
    <div className="w-64 max-w-[calc(100vw-1.5rem)]">
      <div className="pointer-events-auto rounded-xl border border-border-1 bg-surface-1/95 backdrop-blur-md shadow-xl shadow-black/20 overflow-hidden">
        <div className="px-3 py-2 border-b border-border-0 flex items-center gap-2">
          <Loader2 className="w-3.5 h-3.5 text-accent-primary animate-spin flex-shrink-0" aria-hidden="true" />
          <span className="text-xs font-semibold text-text-1">
            {chats.length} chat{chats.length > 1 ? 's' : ''} working
          </span>
        </div>
        <div className="max-h-56 overflow-y-auto">
          {chats.map((c) => (
            <div
              key={c.thread_id}
              className="group w-full flex items-center gap-2 px-3 py-2 hover:bg-surface-2 transition-colors"
            >
              <button
                onClick={() => jumpToWorkspace(c.workspace_id, `/chat/${c.thread_id}`)}
                className="flex items-center gap-2 flex-1 min-w-0 text-left cursor-pointer"
                title={`Open "${c.title}"`}
              >
                <span className="w-1.5 h-1.5 rounded-full bg-accent-primary animate-pulse flex-shrink-0" aria-hidden="true" />
                <span className="flex-1 min-w-0 text-xs text-text-1 truncate">{c.title}</span>
              </button>
              <button
                onClick={() => {
                  api.post(`/chat/threads/${c.thread_id}/stop`).catch(() => {});
                  setChats((prev) => prev.filter((x) => x.thread_id !== c.thread_id));
                }}
                className="p-1 rounded flex-shrink-0 text-text-3 hover:text-danger hover:bg-danger/10 transition-colors cursor-pointer"
                title="Stop this chat"
                aria-label="Stop chat"
              >
                <Square className="w-3 h-3 fill-current" aria-hidden="true" />
              </button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
