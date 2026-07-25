/**
 * Inbox
 *
 * Where the agents' unattended work reports back. A scheduled run no longer
 * spawns a chat thread on every tick — it files its report here, presented as
 * mail from the agent: sender, subject, the request that was made, and the
 * response in full. A chat thread is created only when you choose to reply,
 * which keeps the chat list to conversations you actually started.
 *
 * Failures file a report too. Previously a failed scheduled run notified
 * nothing at all, so a broken prompt failed silently.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  Inbox as InboxIcon,
  Archive,
  ArchiveRestore,
  MessageSquare,
  Mail,
  MailOpen,
  AlertTriangle,
  Clock,
  Heart,
  RefreshCw,
  ArrowLeft,
  CheckCheck,
} from 'lucide-react';
import { Header } from '../components/Header';
import { Button } from '../components/Button';
import { EmptyState } from '../components/EmptyState';
import { LoadingSpinner } from '../components/LoadingSpinner';
import { notificationsApi, api, type AppNotification, type AgentRole, type WSMessage } from '../lib/api';
import { useWebSocket } from '../lib/useWebSocket';

type Folder = 'all' | 'unread' | 'archived';

const FOLDERS: { key: Folder; label: string; icon: typeof InboxIcon }[] = [
  { key: 'all', label: 'All', icon: InboxIcon },
  { key: 'unread', label: 'Unread', icon: Mail },
  { key: 'archived', label: 'Archived', icon: Archive },
];

const SOURCES: { key: string; label: string; icon: typeof Clock | null }[] = [
  { key: '', label: 'Everything', icon: null },
  { key: 'schedule', label: 'Scheduled', icon: Clock },
  { key: 'heartbeat', label: 'Heartbeat', icon: Heart },
];

function timeAgo(dateStr: string): string {
  const diff = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`;
  return new Date(dateStr).toLocaleDateString();
}

function fullTime(dateStr: string): string {
  return new Date(dateStr).toLocaleString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

/** Sender identity, resolved from the agent roster with a graceful fallback. */
function useSender(roles: AgentRole[]) {
  return useCallback(
    (slug: string) => {
      const role = roles.find(r => r.slug === slug);
      return {
        name: role?.name || slug || 'OpenPaw',
        avatar: role?.avatar_path || '',
        initial: (role?.name || slug || 'O').charAt(0).toUpperCase(),
      };
    },
    [roles],
  );
}

function Avatar({ src, initial, size }: { src: string; initial: string; size: string }) {
  if (src) {
    return <img src={src} alt="" className={`${size} rounded-full object-cover flex-shrink-0`} />;
  }
  return (
    <span
      className={`${size} rounded-full bg-accent-primary/15 text-accent-text font-semibold flex items-center justify-center flex-shrink-0`}
      aria-hidden="true"
    >
      {initial}
    </span>
  );
}

export function Inbox() {
  const [items, setItems] = useState<AppNotification[]>([]);
  const [roles, setRoles] = useState<AgentRole[]>([]);
  const [loading, setLoading] = useState(true);
  const [folder, setFolder] = useState<Folder>('all');
  const [source, setSource] = useState('');
  const [busy, setBusy] = useState(false);
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const sender = useSender(roles);

  // The selected report lives in the URL so a notification can deep-link
  // straight to it and the browser's back button behaves.
  const selectedId = params.get('id');
  const selected = useMemo(() => items.find(i => i.id === selectedId) ?? null, [items, selectedId]);

  const load = useCallback(async () => {
    try {
      const list = await notificationsApi.list({
        unread: folder === 'unread',
        archived: folder === 'archived',
        sourceType: source || undefined,
        limit: 200,
      });
      setItems(Array.isArray(list) ? list : []);
    } catch {
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, [folder, source]);

  useEffect(() => {
    setLoading(true);
    load();
  }, [load]);

  useEffect(() => {
    api
      .get<AgentRole[]>('/agent-roles?enabled=true')
      .then(d => setRoles(d || []))
      .catch(() => setRoles([]));
  }, []);

  // New reports arrive while the page is open — a scheduled run can land at any
  // moment, and an inbox that needs a manual refresh isn't an inbox.
  const onWsMessage = useCallback(
    (msg: WSMessage) => {
      if (msg.type === 'notification_created') load();
    },
    [load],
  );
  useWebSocket({ onMessage: onWsMessage });

  const select = (n: AppNotification) => {
    setParams(prev => {
      const next = new URLSearchParams(prev);
      next.set('id', n.id);
      return next;
    });
    if (!n.read) {
      notificationsApi.markRead(n.id).catch(() => {});
      setItems(prev => prev.map(i => (i.id === n.id ? { ...i, read: true } : i)));
    }
  };

  const clearSelection = () => {
    setParams(prev => {
      const next = new URLSearchParams(prev);
      next.delete('id');
      return next;
    });
  };

  const toggleRead = async (n: AppNotification) => {
    const next = !n.read;
    setItems(prev => prev.map(i => (i.id === n.id ? { ...i, read: next } : i)));
    try {
      await (next ? notificationsApi.markRead(n.id) : notificationsApi.markUnread(n.id));
    } catch {
      load();
    }
    // The unread folder is a filtered view — an item that no longer matches has
    // to leave it, or the list contradicts its own heading.
    if (folder === 'unread') load();
  };

  const archive = async (n: AppNotification) => {
    setBusy(true);
    try {
      await notificationsApi.dismiss(n.id);
      clearSelection();
      await load();
    } finally {
      setBusy(false);
    }
  };

  const restore = async (n: AppNotification) => {
    setBusy(true);
    try {
      await notificationsApi.restore(n.id);
      clearSelection();
      await load();
    } finally {
      setBusy(false);
    }
  };

  const openAsChat = async (n: AppNotification) => {
    setBusy(true);
    try {
      // Idempotent server-side: a report that already has a thread returns it
      // rather than forking a duplicate.
      const { thread_id } = await notificationsApi.openAsChat(n.id);
      navigate(`/chat/${thread_id}`);
    } catch {
      setBusy(false);
    }
  };

  const markAllRead = async () => {
    await notificationsApi.markAllRead().catch(() => {});
    load();
  };

  const unreadCount = items.filter(i => !i.read).length;

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Inbox"
        actions={
          <div className="flex items-center gap-2">
            {unreadCount > 0 && folder !== 'archived' && (
              <Button variant="secondary" size="sm" icon={<CheckCheck className="w-4 h-4" />} onClick={markAllRead}>
                Mark all read
              </Button>
            )}
            <Button
              variant="secondary"
              size="sm"
              icon={<RefreshCw className="w-4 h-4" />}
              onClick={load}
            >
              Refresh
            </Button>
          </div>
        }
      />

      <div className="flex-1 min-h-0 flex">
        {/* Message list — hidden on mobile while a report is open, so the
            reading pane gets the full width instead of a cramped split. */}
        <div
          className={`${selected ? 'hidden md:flex' : 'flex'} flex-col w-full md:w-[360px] lg:w-[420px] md:border-r border-border-0 min-h-0`}
        >
          <div className="flex-shrink-0 px-3 pt-3 pb-2 space-y-2 border-b border-border-0">
            <div className="flex gap-1">
              {FOLDERS.map(f => (
                <button
                  key={f.key}
                  onClick={() => {
                    setFolder(f.key);
                    clearSelection();
                  }}
                  className={`flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                    folder === f.key
                      ? 'bg-accent-muted text-accent-text'
                      : 'text-text-2 hover:text-text-1 hover:bg-surface-2'
                  }`}
                >
                  <f.icon className="w-3.5 h-3.5" aria-hidden="true" />
                  {f.label}
                </button>
              ))}
            </div>
            <div className="flex gap-1">
              {SOURCES.map(s => (
                <button
                  key={s.key}
                  onClick={() => {
                    setSource(s.key);
                    clearSelection();
                  }}
                  className={`flex items-center gap-1.5 px-2 py-1 rounded-md text-[11px] transition-colors cursor-pointer ${
                    source === s.key
                      ? 'bg-surface-3 text-text-1'
                      : 'text-text-3 hover:text-text-1 hover:bg-surface-2'
                  }`}
                >
                  {s.icon && <s.icon className="w-3 h-3" aria-hidden="true" />}
                  {s.label}
                </button>
              ))}
            </div>
          </div>

          <div className="flex-1 overflow-y-auto">
            {loading ? (
              <div className="flex justify-center py-12">
                <LoadingSpinner />
              </div>
            ) : items.length === 0 ? (
              <div className="p-6">
                <EmptyState
                  icon={<InboxIcon className="w-10 h-10" />}
                  title={folder === 'archived' ? 'Nothing archived' : 'Inbox zero'}
                  description={
                    folder === 'archived'
                      ? 'Reports you archive are kept here.'
                      : 'Scheduled runs and heartbeat activity report back here.'
                  }
                />
              </div>
            ) : (
              items.map(n => {
                const s = sender(n.source_agent_slug);
                const failed = n.priority === 'high';
                const active = n.id === selectedId;
                return (
                  <button
                    key={n.id}
                    onClick={() => select(n)}
                    className={`w-full text-left flex items-start gap-3 px-3 py-3 border-b border-border-0 transition-colors cursor-pointer ${
                      active ? 'bg-accent-muted' : n.read ? 'hover:bg-surface-2/40' : 'bg-accent-primary/5 hover:bg-accent-primary/10'
                    }`}
                  >
                    <Avatar src={s.avatar} initial={s.initial} size="w-8 h-8 text-xs" />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline gap-2">
                        <span className={`text-xs truncate ${n.read ? 'text-text-2' : 'text-text-0 font-semibold'}`}>
                          {s.name}
                        </span>
                        <span className="ml-auto text-[10px] text-text-3 flex-shrink-0">{timeAgo(n.created_at)}</span>
                      </div>
                      <p
                        className={`text-sm leading-snug truncate mt-0.5 flex items-center gap-1.5 ${
                          n.read ? 'text-text-1' : 'text-text-0 font-medium'
                        }`}
                      >
                        {failed && <AlertTriangle className="w-3.5 h-3.5 text-danger flex-shrink-0" aria-hidden="true" />}
                        <span className="truncate">{n.title}</span>
                      </p>
                      <p className="text-xs text-text-3 mt-0.5 line-clamp-2 leading-relaxed">{n.body}</p>
                    </div>
                  </button>
                );
              })
            )}
          </div>
        </div>

        {/* Reading pane */}
        <div className={`${selected ? 'flex' : 'hidden md:flex'} flex-col flex-1 min-w-0 min-h-0`}>
          {!selected ? (
            <div className="flex-1 flex items-center justify-center p-6">
              <EmptyState
                icon={<MailOpen className="w-10 h-10" />}
                title="No report selected"
                description="Pick a report on the left to read it."
              />
            </div>
          ) : (
            <ReadingPane
              key={selected.id}
              report={selected}
              sender={sender(selected.source_agent_slug)}
              busy={busy}
              onBack={clearSelection}
              onToggleRead={() => toggleRead(selected)}
              onArchive={() => archive(selected)}
              onRestore={() => restore(selected)}
              onOpenChat={() => openAsChat(selected)}
            />
          )}
        </div>
      </div>
    </div>
  );
}

interface ReadingPaneProps {
  report: AppNotification;
  sender: { name: string; avatar: string; initial: string };
  busy: boolean;
  onBack: () => void;
  onToggleRead: () => void;
  onArchive: () => void;
  onRestore: () => void;
  onOpenChat: () => void;
}

function ReadingPane({
  report,
  sender,
  busy,
  onBack,
  onToggleRead,
  onArchive,
  onRestore,
  onOpenChat,
}: ReadingPaneProps) {
  const failed = report.priority === 'high';
  const hasThread = report.link.startsWith('/chat/');

  return (
    <>
      <div className="flex-shrink-0 border-b border-border-0 px-4 py-3">
        <button
          onClick={onBack}
          className="md:hidden flex items-center gap-1.5 text-xs text-text-2 hover:text-text-1 mb-2 cursor-pointer"
        >
          <ArrowLeft className="w-3.5 h-3.5" aria-hidden="true" />
          Back to inbox
        </button>

        <div className="flex items-start gap-3">
          <Avatar src={sender.avatar} initial={sender.initial} size="w-10 h-10 text-sm" />
          <div className="min-w-0 flex-1">
            <h2 className="text-base font-semibold text-text-0 leading-snug flex items-start gap-2">
              {failed && <AlertTriangle className="w-4 h-4 text-danger flex-shrink-0 mt-0.5" aria-hidden="true" />}
              <span className="min-w-0">{report.title}</span>
            </h2>
            <p className="text-xs text-text-2 mt-0.5">
              <span className="text-text-1">{sender.name}</span>
              {report.source_type && <span className="text-text-3"> · {report.source_type}</span>}
              <span className="text-text-3"> · {fullTime(report.created_at)}</span>
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 mt-3">
          <Button
            size="sm"
            icon={<MessageSquare className="w-4 h-4" />}
            onClick={onOpenChat}
            disabled={busy}
          >
            {hasThread ? 'Open chat' : 'Open as chat'}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            icon={report.read ? <Mail className="w-4 h-4" /> : <MailOpen className="w-4 h-4" />}
            onClick={onToggleRead}
            disabled={busy}
          >
            {report.read ? 'Mark unread' : 'Mark read'}
          </Button>
          {report.dismissed ? (
            <Button
              variant="secondary"
              size="sm"
              icon={<ArchiveRestore className="w-4 h-4" />}
              onClick={onRestore}
              disabled={busy}
            >
              Restore
            </Button>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              icon={<Archive className="w-4 h-4" />}
              onClick={onArchive}
              disabled={busy}
            >
              Archive
            </Button>
          )}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-4 py-5 md:px-8 md:py-7">
        <div className="max-w-3xl mx-auto">
          {report.prompt && (
            <div className="mb-6 rounded-xl border border-border-0 bg-surface-1/60 px-4 py-3">
              <p className="text-[10px] font-semibold uppercase tracking-wider text-text-3 mb-1.5">
                The request
              </p>
              <p className="text-sm text-text-2 leading-relaxed whitespace-pre-wrap">{report.prompt}</p>
            </div>
          )}

          <div className="prose-chat">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{report.detail || report.body}</ReactMarkdown>
          </div>

          <p className="text-[11px] text-text-3 mt-8 pt-4 border-t border-border-0 leading-relaxed">
            {hasThread
              ? 'This report has a chat thread — open it to continue the conversation.'
              : 'Opening this as a chat recreates the exchange as a thread you can reply to.'}
          </p>
        </div>
      </div>
    </>
  );
}
