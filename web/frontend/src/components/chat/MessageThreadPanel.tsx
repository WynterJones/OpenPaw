import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { ArrowUp, CornerDownRight, Loader2, MessageSquare, OctagonX, Square, UserRound, X } from 'lucide-react';
import { api, type AgentRole, type ChatMessage, type ChatThread } from '../../lib/api';
import { MessageBubble } from './MessageBubbles';
import { mentionComponents } from './MentionSystem';
import { resolveMessageRole } from '../../lib/chatUtils';
import { localFileSrc, splitPastedImages } from './messageDisplay';

interface MessageThreadLookup {
  thread: ChatThread | null;
  reply_count: number;
}

interface MessageThreadPanelProps {
  rootMessage: ChatMessage;
  roles: AgentRole[];
  userAvatarPath?: string;
  onClose: () => void;
  onReplyCountChange: (messageId: string, count: number, childThreadId?: string) => void;
  onError: (message: string) => void;
}

const POLL_INTERVAL_MS = 1200;

function sameMessages(current: ChatMessage[], next: ChatMessage[]): boolean {
  if (current.length !== next.length) return false;
  return current.every((message, index) => {
    const candidate = next[index];
    return message.id === candidate.id
      && message.content === candidate.content
      && message.stopped === candidate.stopped
      && message.widget_data === candidate.widget_data
      && message.image_url === candidate.image_url
      && JSON.stringify(message.tool_calls) === JSON.stringify(candidate.tool_calls)
      && JSON.stringify(message.reactions) === JSON.stringify(candidate.reactions);
  });
}

function ThreadOriginalMessage({
  message,
  roles,
  userAvatarPath,
}: {
  message: ChatMessage;
  roles: AgentRole[];
  userAvatarPath?: string;
}) {
  const isUser = message.role === 'user';
  const role = resolveMessageRole(roles, message.agent_role_slug);
  const author = isUser ? 'You' : role?.name || 'Gateway';
  const { text, images } = isUser
    ? splitPastedImages(message.content)
    : { text: message.content, images: [] };
  const timestamp = new Date(message.created_at).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });

  return (
    <section aria-label="Original message" className="mb-5">
      <div className="mb-2 flex items-center gap-2 px-1">
        <CornerDownRight className="h-3.5 w-3.5 text-accent-primary" aria-hidden="true" />
        <span className="text-[11px] font-semibold uppercase tracking-wider text-text-2">Original message</span>
      </div>
      <div className="rounded-r-xl border-l-2 border-accent-primary/50 bg-surface-0/45 px-3 py-3">
        <div className="flex min-w-0 items-center gap-2.5">
          {isUser ? (
            userAvatarPath ? (
              <img
                src={userAvatarPath}
                alt=""
                className="h-7 w-7 flex-shrink-0 rounded-lg object-cover"
              />
            ) : (
              <span className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-lg bg-surface-2 text-text-2">
                <UserRound className="h-3.5 w-3.5" aria-hidden="true" />
              </span>
            )
          ) : (
            <img
              src={role?.avatar_path || '/gateway-avatar.png'}
              alt=""
              className="h-7 w-7 flex-shrink-0 rounded-lg object-cover"
            />
          )}
          <span className="min-w-0 flex-1 truncate text-xs font-semibold text-text-1">{author}</span>
          <time className="flex-shrink-0 whitespace-nowrap text-[10px] text-text-3" dateTime={message.created_at}>
            {timestamp}
          </time>
        </div>

        <div className={`mt-3 min-w-0 text-[15px] font-medium leading-7 text-text-1 ${isUser ? 'prose-chat-user' : ''}`}>
          <div className="prose-chat max-w-none break-words">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={mentionComponents(roles)}>
              {text}
            </ReactMarkdown>
          </div>
          {images.length > 0 && (
            <div className={`flex flex-wrap gap-2 ${text ? 'mt-3' : ''}`}>
              {images.map((image) => (
                <img
                  key={image.path}
                  src={localFileSrc(image.path)}
                  alt={image.name}
                  title={image.name}
                  loading="lazy"
                  className="max-h-52 max-w-full rounded-lg border border-border-1 object-contain"
                />
              ))}
            </div>
          )}
          {message.image_url && (
            <img
              src={message.image_url}
              alt="Generated image"
              className="mt-3 max-h-60 max-w-full rounded-lg border border-border-1 object-contain"
            />
          )}
        </div>

        {message.stopped && (
          <div className="mt-3 flex items-center gap-1.5 border-t border-border-0 pt-2.5 text-[11px] font-medium text-text-3">
            <OctagonX className="h-3 w-3 flex-shrink-0" aria-hidden="true" />
            <span>Stopped by you — this reply is incomplete</span>
          </div>
        )}
      </div>
    </section>
  );
}

/**
 * A focused message thread deliberately owns its composer and polling state.
 * That separation is more than a UI detail: typing or waiting here can never
 * overwrite the parent chat's draft/stream, and the backend gives this child
 * chat its own context window.
 */
export function MessageThreadPanel({
  rootMessage,
  roles,
  userAvatarPath,
  onClose,
  onReplyCountChange,
  onError,
}: MessageThreadPanelProps) {
  const [thread, setThread] = useState<ChatThread | null>(null);
  const [replies, setReplies] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [working, setWorking] = useState(false);
  const [mentionOpen, setMentionOpen] = useState(false);
  const [mentionFilter, setMentionFilter] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const pollRef = useRef<number | null>(null);
  const mountedRef = useRef(true);
  const initialLoadRef = useRef(true);
  const stickToBottomRef = useRef(false);
  const previousReplyCountRef = useRef(0);

  const mentionRoles = useMemo(() => {
    const query = mentionFilter.toLowerCase();
    return roles
      .filter((role) => role.slug !== 'builder')
      .filter((role) => role.name.toLowerCase().includes(query) || role.slug.toLowerCase().includes(query))
      .slice(0, 8);
  }, [roles, mentionFilter]);

  const stopPolling = useCallback(() => {
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }, []);

  const publishReplyCount = useCallback((next: ChatMessage[], child?: ChatThread | null) => {
    onReplyCountChange(rootMessage.id, next.length, child?.id);
  }, [onReplyCountChange, rootMessage.id]);

  const storeReplies = useCallback((next: ChatMessage[]) => {
    setReplies((current) => sameMessages(current, next) ? current : next);
  }, []);

  const loadReplies = useCallback(async (activeThread: ChatThread) => {
    const next = await api.get<ChatMessage[]>(`/chat/threads/${activeThread.id}/messages`);
    if (!mountedRef.current) return [];
    const safe = Array.isArray(next) ? next : [];
    storeReplies(safe);
    publishReplyCount(safe, activeThread);
    return safe;
  }, [publishReplyCount, storeReplies]);

  const startPolling = useCallback((activeThread: ChatThread) => {
    stopPolling();
    setWorking(true);
    let ticks = 0;
    pollRef.current = window.setInterval(async () => {
      ticks += 1;
      try {
        const [status, next] = await Promise.all([
          api.get<{ active: boolean }>(`/chat/threads/${activeThread.id}/status`),
          api.get<ChatMessage[]>(`/chat/threads/${activeThread.id}/messages`),
        ]);
        if (!mountedRef.current) return;
        const safe = Array.isArray(next) ? next : [];
        storeReplies(safe);
        publishReplyCount(safe, activeThread);
        const latest = safe[safe.length - 1];
        // Require an assistant reply before treating an initially-idle status
        // as completion; routing starts in a goroutine just after the POST.
        if ((!status.active && latest?.role === 'assistant') || ticks > 300) {
          setWorking(false);
          stopPolling();
        }
      } catch {
        if (ticks > 5) {
          setWorking(false);
          stopPolling();
        }
      }
    }, POLL_INTERVAL_MS);
  }, [publishReplyCount, stopPolling, storeReplies]);

  useEffect(() => {
    mountedRef.current = true;
    initialLoadRef.current = true;
    stickToBottomRef.current = false;
    previousReplyCountRef.current = 0;
    const load = async () => {
      setLoading(true);
      setThread(null);
      setReplies((current) => current.length > 0 ? [] : current);
      try {
        const lookup = await api.get<MessageThreadLookup>(`/chat/messages/${rootMessage.id}/thread`);
        if (!mountedRef.current) return;
        setThread(lookup.thread);
        if (lookup.thread) {
          const next = await loadReplies(lookup.thread);
          const status = await api.get<{ active: boolean }>(`/chat/threads/${lookup.thread.id}/status`);
          if (status.active) startPolling(lookup.thread);
          else publishReplyCount(next, lookup.thread);
        } else {
          publishReplyCount([], null);
        }
      } catch (error) {
        onError(error instanceof Error ? error.message : 'Failed to open thread');
      } finally {
        if (mountedRef.current) {
          initialLoadRef.current = false;
          setLoading(false);
        }
      }
    };
    void load();
    requestAnimationFrame(() => textareaRef.current?.focus());
    return () => {
      mountedRef.current = false;
      stopPolling();
    };
  }, [loadReplies, onError, publishReplyCount, rootMessage.id, startPolling, stopPolling]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  useEffect(() => {
    const addedReplies = replies.length > previousReplyCountRef.current;
    previousReplyCountRef.current = replies.length;
    if (
      initialLoadRef.current
      || !addedReplies
      || !stickToBottomRef.current
    ) {
      return;
    }
    endRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }, [replies.length]);

  const handleInput = (value: string, cursor: number) => {
    setInput(value);
    const match = value.slice(0, cursor).match(/@([\w-]*)$/);
    setMentionOpen(!!match);
    setMentionFilter(match?.[1] || '');
  };

  const insertMention = (role: AgentRole) => {
    const textarea = textareaRef.current;
    const cursor = textarea?.selectionStart ?? input.length;
    const before = input.slice(0, cursor).replace(/@[\w-]*$/, `@${role.name} `);
    const next = before + input.slice(cursor);
    setInput(next);
    setMentionOpen(false);
    requestAnimationFrame(() => {
      if (!textarea) return;
      textarea.focus();
      textarea.selectionStart = textarea.selectionEnd = before.length;
    });
  };

  const send = async () => {
    const content = input.trim();
    if (!content || sending || working) return;
    setSending(true);
    stickToBottomRef.current = true;
    setInput('');
    setMentionOpen(false);
    try {
      let activeThread = thread;
      if (!activeThread) {
        activeThread = await api.post<ChatThread>(`/chat/messages/${rootMessage.id}/thread`, {});
        if (!mountedRef.current) return;
        setThread(activeThread);
      }
      const saved = await api.post<ChatMessage>(`/chat/threads/${activeThread.id}/messages`, {
        content,
        agent_role_slug: '',
      });
      if (!mountedRef.current) return;
      const next = [...replies, saved];
      setReplies(next);
      publishReplyCount(next, activeThread);
      startPolling(activeThread);
    } catch (error) {
      setInput(content);
      onError(error instanceof Error ? error.message : 'Failed to send thread reply');
    } finally {
      if (mountedRef.current) setSending(false);
    }
  };

  const stop = async () => {
    if (!thread) return;
    try {
      await api.post(`/chat/threads/${thread.id}/stop`);
      setWorking(false);
      stopPolling();
      await loadReplies(thread);
    } catch (error) {
      onError(error instanceof Error ? error.message : 'Failed to stop reply');
    }
  };

  const react = async (messageId: string, emoji: string) => {
    try {
      const result = await api.post<{ reactions: ChatMessage['reactions'] }>(
        `/chat/messages/${messageId}/reactions`,
        { emoji },
      );
      setReplies((current) => current.map((message) => (
        message.id === messageId ? { ...message, reactions: result.reactions || [] } : message
      )));
    } catch {
      onError('Failed to add reaction');
    }
  };

  return (
    <aside
      aria-label="Message thread"
      className="absolute inset-0 z-40 flex min-w-0 flex-col border-l border-border-1 bg-surface-1 shadow-2xl shadow-black/30 md:relative md:inset-auto md:w-[420px] md:flex-shrink-0"
    >
      <header className="flex min-h-14 items-center gap-3 border-b border-border-0 px-4 py-2.5">
        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-accent-muted text-accent-primary">
          <MessageSquare className="h-4 w-4" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold leading-tight text-text-0">Thread</h2>
          <p className="mt-0.5 text-xs leading-relaxed text-text-3">Independent context</p>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="flex h-8 w-8 items-center justify-center rounded-lg border border-border-0 text-text-2 transition-colors hover:border-border-1 hover:bg-surface-2 hover:text-text-0 active:translate-y-px"
          aria-label="Close thread"
          title="Close thread"
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </header>

      <div
        ref={scrollAreaRef}
        className="flex-1 overflow-y-auto px-3 py-4"
        onScroll={() => {
          const area = scrollAreaRef.current;
          if (!area) return;
          stickToBottomRef.current = area.scrollHeight - area.scrollTop - area.clientHeight < 80;
        }}
      >
        <ThreadOriginalMessage
          message={rootMessage}
          roles={roles}
          userAvatarPath={userAvatarPath}
        />

        <div className="mb-4 flex items-center gap-3" aria-hidden="true">
          <div className="h-px flex-1 bg-border-0" />
          <span className="text-[10px] font-semibold uppercase tracking-wider text-text-3">
            {replies.length > 0 ? `${replies.length} ${replies.length === 1 ? 'reply' : 'replies'}` : 'Replies'}
          </span>
          <div className="h-px flex-1 bg-border-0" />
        </div>

        {loading ? (
          <div className="flex min-h-28 items-center justify-center">
            <Loader2 className="h-5 w-5 animate-spin text-accent-primary" aria-label="Loading thread" />
          </div>
        ) : replies.length === 0 ? (
          <div className="mx-auto flex min-h-32 max-w-xs flex-col items-center justify-center px-5 text-center">
            <p className="text-sm font-medium text-text-1">Start a focused conversation</p>
            <p className="mt-1.5 text-xs leading-relaxed text-text-3">
              Only the original message and replies here are shared with the agent.
            </p>
          </div>
        ) : (
          <div className="space-y-4">
            {replies.map((message) => (
              <MessageBubble
                key={message.id}
                message={message}
                roles={roles}
                onRefresh={() => thread && void loadReplies(thread)}
                onReact={react}
              />
            ))}
          </div>
        )}
        {working && (
          <div className="mt-4 flex items-center gap-2 rounded-xl border border-border-0 bg-surface-2/70 px-3 py-2.5 text-xs text-text-2" aria-live="polite">
            <Loader2 className="h-3.5 w-3.5 animate-spin text-accent-primary" aria-hidden="true" />
            Agent is replying…
          </div>
        )}
        <div ref={endRef} />
      </div>

      <div className="border-t border-border-0 bg-surface-0/80 p-3 backdrop-blur-xl">
        <div className="relative rounded-xl border border-border-1 bg-surface-1 transition-colors focus-within:border-accent-primary">
          {mentionOpen && mentionRoles.length > 0 && (
            <div
              role="listbox"
              aria-label="Mention an agent"
              className="absolute bottom-full left-0 right-0 z-50 mb-2 max-h-60 overflow-y-auto rounded-xl border border-border-1 bg-surface-1 p-1.5 shadow-xl shadow-black/30"
            >
              {mentionRoles.map((role) => (
                <button
                  type="button"
                  role="option"
                  aria-selected="false"
                  key={role.slug}
                  onClick={() => insertMention(role)}
                  className="flex w-full items-start gap-2.5 rounded-lg px-2.5 py-2 text-left transition-colors hover:bg-surface-2 active:translate-y-px"
                >
                  <img src={role.avatar_path} alt="" className="h-7 w-7 flex-shrink-0 rounded-md object-cover" />
                  <span className="min-w-0">
                    <span className="block truncate text-xs font-medium text-text-0">@{role.name}</span>
                    {role.description && <span className="mt-0.5 block truncate text-[11px] text-text-3">{role.description}</span>}
                  </span>
                </button>
              ))}
            </div>
          )}
          <textarea
            ref={textareaRef}
            value={input}
            rows={2}
            disabled={loading}
            onChange={(event) => {
              handleInput(event.target.value, event.target.selectionStart);
              event.target.style.height = 'auto';
              event.target.style.height = `${Math.min(event.target.scrollHeight, 160)}px`;
            }}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                void send();
              }
            }}
            placeholder="Reply in thread…"
            aria-label="Reply in thread"
            className="min-h-14 w-full resize-none border-0 bg-transparent px-3.5 pb-1 pt-3 text-sm font-medium text-text-0 outline-none shadow-none focus:border-transparent focus:outline-none focus:ring-0 focus-visible:outline-none focus-visible:ring-0 placeholder:font-normal placeholder:text-text-3 disabled:opacity-50"
            style={{ maxHeight: '160px' }}
          />
          <div className="flex items-center justify-between px-2.5 pb-2 pt-1">
            <span className="px-1 text-[10px] text-text-3">
              <span className="font-medium text-text-2">@</span> agents · Enter to send
            </span>
            {working ? (
              <button
                type="button"
                onClick={() => void stop()}
                className="flex h-8 w-8 items-center justify-center rounded-full bg-surface-3 text-text-1 transition-colors hover:bg-danger hover:text-white active:translate-y-px"
                aria-label="Stop thread reply"
                title="Stop"
              >
                <Square className="h-3.5 w-3.5 fill-current" aria-hidden="true" />
              </button>
            ) : (
              <button
                type="button"
                onClick={() => void send()}
                disabled={!input.trim() || sending || loading}
                className="flex h-8 w-8 items-center justify-center rounded-full bg-accent-primary text-white transition-colors hover:bg-accent-hover active:translate-y-px disabled:cursor-not-allowed disabled:opacity-30"
                aria-label="Send thread reply"
              >
                {sending
                  ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                  : <ArrowUp className="h-4 w-4" aria-hidden="true" />}
              </button>
            )}
          </div>
        </div>
      </div>
    </aside>
  );
}
