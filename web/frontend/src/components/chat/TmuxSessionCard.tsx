/**
 * TmuxSessionCard
 *
 * Shows any running tmux session (a Claude Code / Codex build session) above the
 * composer, with whatever the CLI's own status line exposes.
 *
 * It also starts a server-side watch. An agent turn is one request/response, so
 * an agent that promises to "keep checking" cannot — the watch polls on the
 * server, reports into the chat when the session finishes or stalls, and stops
 * itself. The countdown here is the same schedule, shown so it is obvious a
 * check is actually pending.
 */

import { useCallback, useEffect, useState } from 'react';
import { TerminalSquare, Eye, EyeOff, GitBranch, Gauge, Clock, Loader2 } from 'lucide-react';
import { api } from '../../lib/api';

interface TmuxStatus {
  project?: string;
  branch?: string;
  uncommitted?: number;
  framework?: string;
  model?: string;
  context_pct?: number;
  elapsed?: string;
  lines_added?: number;
  lines_removed?: number;
  auto_mode?: boolean;
  agents?: number;
}

interface TmuxSession {
  name: string;
  windows: number;
  created: string;
  attached: boolean;
  kind: 'claude' | 'codex' | 'shell';
  status?: TmuxStatus;
  tail?: string[];
}

interface TmuxWatch {
  session: string;
  interval_seconds: number;
  next_check: string;
  checks: number;
}

interface TmuxResponse {
  available: boolean;
  sessions: TmuxSession[];
  watches: Record<string, TmuxWatch>;
}

const WATCH_INTERVAL_SECONDS = 30;

function Countdown({ target }: { target: string }) {
  const [left, setLeft] = useState(() => Math.max(0, Math.round((new Date(target).getTime() - Date.now()) / 1000)));

  useEffect(() => {
    const tick = () => setLeft(Math.max(0, Math.round((new Date(target).getTime() - Date.now()) / 1000)));
    tick();
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [target]);

  return <span className="tabular-nums">{left}s</span>;
}

export function TmuxSessionCard({ threadId }: { threadId: string | null }) {
  const [data, setData] = useState<TmuxResponse | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    if (!threadId) return;
    try {
      const res = await api.get<TmuxResponse>(`/chat/tmux?thread_id=${encodeURIComponent(threadId)}`);
      setData(res);
    } catch {
      setData(null);
    }
  }, [threadId]);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      await load();
      if (!cancelled) timer = setTimeout(poll, 10000);
    };
    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [load]);

  const toggleWatch = async (session: string, watching: boolean) => {
    if (!threadId) return;
    setBusy(true);
    try {
      if (watching) {
        await api.delete(`/chat/threads/${threadId}/tmux-watch?session=${encodeURIComponent(session)}`);
      } else {
        await api.post(`/chat/threads/${threadId}/tmux-watch`, {
          session,
          interval_seconds: WATCH_INTERVAL_SECONDS,
        });
      }
      await load();
    } catch {
      /* leave the card as-is; the next poll re-syncs */
    } finally {
      setBusy(false);
    }
  };

  const sessions = (data?.sessions ?? []).filter(s => s.kind !== 'shell');
  if (sessions.length === 0) return null;

  return (
    <div className="space-y-1.5 mb-2">
      {sessions.map((s) => {
        const watch = data?.watches?.[s.name];
        const st = s.status;
        return (
          <div
            key={s.name}
            className="rounded-xl border border-border-1 bg-surface-1/80 backdrop-blur-sm px-3 py-2"
          >
            <div className="flex items-center gap-2">
              <TerminalSquare className="w-3.5 h-3.5 text-accent-primary flex-shrink-0" aria-hidden="true" />
              <span className="text-xs font-semibold text-text-1 truncate">
                {st?.project || s.name}
              </span>
              {st?.branch && (
                <span className="flex items-center gap-1 text-[10px] text-text-3 flex-shrink-0">
                  <GitBranch className="w-3 h-3" aria-hidden="true" />
                  {st.branch}
                </span>
              )}
              <span className="text-[10px] text-text-3 flex-shrink-0">
                {s.kind === 'claude' ? 'Claude Code' : 'Codex'}
              </span>

              <button
                onClick={() => toggleWatch(s.name, !!watch)}
                disabled={busy}
                className={`ml-auto flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-medium transition-colors cursor-pointer disabled:opacity-50 ${
                  watch ? 'text-accent-primary bg-accent-muted' : 'text-text-2 hover:text-text-1 hover:bg-surface-2'
                }`}
                title={watch
                  ? 'Stop checking this session'
                  : `Check every ${WATCH_INTERVAL_SECONDS}s and report back when it finishes or stalls`}
              >
                {busy ? (
                  <Loader2 className="w-3 h-3 animate-spin" aria-hidden="true" />
                ) : watch ? (
                  <Eye className="w-3 h-3" aria-hidden="true" />
                ) : (
                  <EyeOff className="w-3 h-3" aria-hidden="true" />
                )}
                {watch ? <>checking in <Countdown target={watch.next_check} /></> : 'Watch'}
              </button>
            </div>

            {st && (
              <div className="flex items-center flex-wrap gap-x-3 gap-y-0.5 mt-1 text-[10px] text-text-3">
                {st.model && <span>{st.model}</span>}
                {typeof st.context_pct === 'number' && st.context_pct > 0 && (
                  <span className="flex items-center gap-1">
                    <Gauge className="w-3 h-3" aria-hidden="true" />
                    {st.context_pct}% context
                  </span>
                )}
                {st.elapsed && (
                  <span className="flex items-center gap-1">
                    <Clock className="w-3 h-3" aria-hidden="true" />
                    {st.elapsed}
                  </span>
                )}
                {(st.lines_added || st.lines_removed) && (
                  <span>
                    <span className="text-emerald-400">+{st.lines_added ?? 0}</span>
                    {' / '}
                    <span className="text-red-400">-{st.lines_removed ?? 0}</span>
                  </span>
                )}
                {st.auto_mode && <span className="text-accent-primary">auto mode</span>}
                {!!st.agents && <span>{st.agents} agent{st.agents > 1 ? 's' : ''}</span>}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
