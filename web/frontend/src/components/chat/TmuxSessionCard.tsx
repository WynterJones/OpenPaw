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

import { useCallback, useEffect, useId, useState } from 'react';
import { TerminalSquare, Eye, EyeOff, GitBranch, Gauge, Clock, Loader2, X, ChevronDown } from 'lucide-react';
import { api } from '../../lib/api';
import { Modal } from '../Modal';
import { Button } from '../Button';

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
  const [expanded, setExpanded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [killTarget, setKillTarget] = useState<TmuxSession | null>(null);
  const [killing, setKilling] = useState(false);
  const sessionListId = useId();

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

  // Each chat opens with the compact summary so running sessions do not
  // permanently take space above the composer, especially when several agents
  // are active at once.
  useEffect(() => {
    setExpanded(false);
  }, [threadId]);

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

  const killSession = async (session: string) => {
    setKilling(true);
    try {
      await api.delete(`/chat/tmux?session=${encodeURIComponent(session)}`);
      // Drop it locally rather than waiting on the 10s poll, so the bar goes
      // away the moment the session does.
      setData(prev => prev ? { ...prev, sessions: prev.sessions.filter(s => s.name !== session) } : prev);
      setKillTarget(null);
      await load();
    } catch {
      /* the next poll re-syncs whatever actually happened */
    } finally {
      setKilling(false);
    }
  };

  const sessions = (data?.sessions ?? []).filter(s => s.kind !== 'shell');
  if (sessions.length === 0) return null;

  const watchedCount = sessions.filter(s => !!data?.watches?.[s.name]).length;
  const claudeCount = sessions.filter(s => s.kind === 'claude').length;
  const codexCount = sessions.filter(s => s.kind === 'codex').length;
  const providerSummary = [
    claudeCount > 0 ? `${claudeCount} Claude Code` : '',
    codexCount > 0 ? `${codexCount} Codex` : '',
  ].filter(Boolean).join(' · ');

  return (
    <div className="mb-2 max-w-full">
      <button
        type="button"
        onClick={() => setExpanded(value => !value)}
        aria-expanded={expanded}
        aria-controls={sessionListId}
        className="group inline-flex h-9 w-fit max-w-full items-center gap-2 rounded-lg border border-border-1 bg-surface-1/85 px-3 text-left shadow-sm backdrop-blur-sm transition-colors hover:border-accent-primary/40 hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary cursor-pointer"
      >
        <span className="relative flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-md bg-accent-muted text-accent-primary">
          <TerminalSquare className="h-3.5 w-3.5" aria-hidden="true" />
          <span className="absolute -right-0.5 -top-0.5 h-1.5 w-1.5 rounded-full bg-emerald-400 ring-2 ring-surface-1" aria-hidden="true" />
        </span>
        <span className="truncate text-xs font-semibold text-text-1">
          {sessions.length} agent session{sessions.length === 1 ? '' : 's'} running
        </span>
        <span className="hidden truncate text-[10px] text-text-3 sm:inline">
          {providerSummary}
        </span>
        {watchedCount > 0 && (
          <span className="flex flex-shrink-0 items-center gap-1 rounded-md bg-accent-muted px-1.5 py-0.5 text-[10px] font-medium text-accent-primary">
            <Eye className="h-3 w-3" aria-hidden="true" />
            {watchedCount}
            <span className="sr-only">watched</span>
          </span>
        )}
        <ChevronDown
          className={`h-3.5 w-3.5 flex-shrink-0 text-text-3 transition-transform group-hover:text-text-2 ${expanded ? 'rotate-180' : ''}`}
          aria-hidden="true"
        />
      </button>

      <div
        id={sessionListId}
        className={`mt-1.5 space-y-1.5 ${expanded ? 'block' : 'hidden'}`}
      >
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

              <button
                onClick={() => setKillTarget(s)}
                className="p-0.5 rounded-md text-text-3 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer flex-shrink-0"
                title="Close this session — stops whatever is running in it"
                aria-label={`Close tmux session ${s.name}`}
              >
                <X className="w-3.5 h-3.5" aria-hidden="true" />
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

      <Modal open={!!killTarget} onClose={() => !killing && setKillTarget(null)} title="Close this session?" size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-2">
            This kills the tmux session{' '}
            <span className="font-medium text-text-1">
              {killTarget?.status?.project || killTarget?.name}
            </span>{' '}
            and stops whatever is running inside it — unsaved work in that session is lost.
          </p>
          <p className="text-xs text-text-3">
            If it is the last session, tmux shuts its server down too. Any watch on it stops.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setKillTarget(null)} disabled={killing}>Cancel</Button>
            <Button
              variant="danger"
              size="sm"
              loading={killing}
              onClick={() => killTarget && killSession(killTarget.name)}
            >
              Close session
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
