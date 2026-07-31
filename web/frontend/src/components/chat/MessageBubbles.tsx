import { useState, useEffect, useRef, type ReactNode } from 'react';
import { DollarSign, Zap, Download, Wrench, Minimize2, OctagonX, MessageSquare } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { ChatMessage, AgentRole, WidgetPayload, SubAgentTask, Reaction } from '../../lib/api';
import { parseConfirmation, parseToolSummary, parseWidgets } from '../../lib/api';
import { cleanToolColons, resolveMessageRole, type StreamingTool, type CostInfo } from '../../lib/chatUtils';
import { mentionComponents } from './MentionSystem';
import { ToolActivityPanel, StreamingToolPanel } from './ToolPanels';
import { ConfirmationCardUI, ToolSummaryCardUI } from './Cards';
import { WidgetRenderer } from '../widgets/WidgetRenderer';
import { SubAgentPanel } from './SubAgentPanel';
import { EmojiPicker } from './EmojiPicker';
import { CompanionAvatar } from '../companion/CompanionAvatar';
import { localFileSrc, splitPastedImages } from './messageDisplay';

function ReactionBar({ reactions, onReact, trailing }: { reactions?: Reaction[]; onReact: (emoji: string) => void; trailing?: ReactNode }) {
  return (
    <div className="flex items-center gap-1 mt-1 px-1 flex-wrap">
      {reactions && reactions.length > 0 && reactions.map((r) => (
        <button
          key={`${r.emoji}-${r.source}`}
          onClick={() => onReact(r.emoji)}
          title={r.source !== 'user' ? `@${r.source}` : undefined}
          className={`inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded-full text-sm border cursor-pointer transition-colors ${
            r.source === 'user'
              ? 'bg-accent-muted border-accent-primary text-accent-primary'
              : 'bg-surface-2 border-border-1 text-text-2'
          } hover:bg-surface-2`}
        >
          <span>{r.emoji}</span>
          {r.count > 1 && <span className="text-xs">{r.count}</span>}
        </button>
      ))}
      <EmojiPicker onSelect={onReact} />
      {trailing}
    </div>
  );
}

function ThreadAction({ count = 0, onOpen }: { count?: number; onOpen?: () => void }) {
  if (!onOpen) return null;
  const label = count > 0
    ? `Open thread with ${count} ${count === 1 ? 'reply' : 'replies'}`
    : 'Reply in thread';
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label={label}
      title={label}
      className={`inline-flex items-center gap-1 rounded-full border px-1.5 py-0.5 text-xs transition-colors active:translate-y-px ${
        count > 0
          ? 'border-accent-primary/30 bg-accent-muted text-accent-primary hover:bg-accent-primary/20'
          : 'border-transparent text-text-3 hover:border-border-1 hover:bg-surface-2 hover:text-text-1'
      }`}
    >
      <MessageSquare className="h-3.5 w-3.5" aria-hidden="true" />
      {count > 0 && <span className="tabular-nums">{count}</span>}
    </button>
  );
}

/** Minimum time a status label stays on screen before it may be replaced. */
const STATUS_LABEL_HOLD_MS = 600;

/**
 * Holds a label for a short minimum before letting the next one through.
 *
 * The activity line can flip between "Thinking…", "Working…" and tool activity
 * several times a second while a turn runs. Rendering every change reads as a
 * flicker, so a new label waits out the remainder of the hold and only the
 * latest pending value is applied — a burst of changes settles on the final
 * one rather than replaying the whole sequence.
 */
function useHeldLabel(label: string): string {
  const [shown, setShown] = useState(label);
  // 0 until the first swap; set inside the effect so render stays pure.
  const shownAtRef = useRef(0);

  useEffect(() => {
    if (label === shown) return;
    if (shownAtRef.current === 0) shownAtRef.current = Date.now();
    const elapsed = Date.now() - shownAtRef.current;
    const wait = Math.max(0, STATUS_LABEL_HOLD_MS - elapsed);
    const timer = setTimeout(() => {
      shownAtRef.current = Date.now();
      setShown(label);
    }, wait);
    return () => clearTimeout(timer);
  }, [label, shown]);

  return shown;
}

function ThinkingIndicator({ label = 'Thinking…' }: { label?: string }) {
  const shown = useHeldLabel(label);
  return (
    <div className="status-row flex items-center gap-2 px-1 py-1.5" aria-live="polite">
      <div className="flex items-center gap-1">
        {[0, 150, 300].map((d) => (
          <span
            key={d}
            className="w-1.5 h-1.5 rounded-full bg-accent-primary animate-bounce"
            style={{ animationDelay: `${d}ms` }}
          />
        ))}
      </div>
      {/* Keyed on the text so React remounts it and the fade replays. */}
      <span key={shown} className="status-label-swap text-sm text-text-2 font-medium">
        {shown}
      </span>
    </div>
  );
}

export function StreamingMessage({ text, tools, cost, role, roles, widgets, subAgentTasks }: {
  text: string;
  tools: StreamingTool[];
  cost: CostInfo | null;
  role: AgentRole | null;
  roles: AgentRole[];
  widgets?: WidgetPayload[];
  subAgentTasks?: SubAgentTask[];
}) {
  return (
    <div className="streaming-entrance flex flex-col md:flex-row gap-1 md:gap-3">
      <div className="flex items-center gap-2 md:block">
        {/* CompanionAvatar keeps the active reply animated, while showing a
            neutral placeholder until the working agent is known. */}
        <CompanionAvatar role={role} active />
        {role ? (
          <p className="text-sm font-semibold text-accent-primary md:hidden">{role.name}</p>
        ) : (
          <span className="h-3 w-20 rounded bg-surface-2 animate-pulse md:hidden" aria-hidden="true" />
        )}
      </div>
      <div className="max-w-full md:max-w-[75%]">
        {role ? (
          <p className="text-xs font-medium text-accent-primary mb-0.5 px-1 hidden md:block">{role.name}</p>
        ) : (
          <span className="mb-0.5 mx-1 hidden md:block h-3 w-24 rounded bg-surface-2 animate-pulse" aria-hidden="true" />
        )}
        {text ? (
          <div className="chat-bubble rounded-2xl rounded-tl-md px-4 py-2.5 text-base font-medium text-text-1">
            <div className="prose-chat">
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={mentionComponents(roles)}>{cleanToolColons(text, tools.length > 0)}</ReactMarkdown>
              {tools.length > 0 ? (
                // This turn is using tools — keep "Working…" visible through the
                // gaps between tool calls (when all are done but the model is
                // still working) instead of dropping to a lone blinking cursor.
                <ThinkingIndicator label="Working…" />
              ) : (
                <span className="inline-block w-0.5 h-4 bg-accent-primary animate-pulse ml-0.5 align-text-bottom" />
              )}
            </div>
          </div>
        ) : (
          <ThinkingIndicator />
        )}
        {subAgentTasks && subAgentTasks.length > 0 && (
          <SubAgentPanel tasks={subAgentTasks} roles={roles} />
        )}
        {widgets && widgets.length > 0 && widgets.map((w, i) => (
          <WidgetRenderer key={`sw-${i}`} widget={w} />
        ))}
        <StreamingToolPanel tools={tools} />
        {cost && (
          <div className="flex items-center gap-3 mt-2 px-1 text-[10px] text-text-3">
            <span className="flex items-center gap-1"><DollarSign className="w-3 h-3" />${cost.total_cost_usd.toFixed(4)}</span>
            {cost.usage && (
              <span className="flex items-center gap-1"><Zap className="w-3 h-3" />{((cost.usage.input_tokens || 0) + (cost.usage.output_tokens || 0)).toLocaleString()} tokens</span>
            )}
            {cost.num_turns && cost.num_turns > 1 && (
              <span>{cost.num_turns} turns</span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function UserMessageBubble({ message, roles, onReact, onOpenThread }: { message: ChatMessage; roles: AgentRole[]; onReact?: (messageId: string, emoji: string) => void; onOpenThread?: (message: ChatMessage) => void }) {
  const [expanded, setExpanded] = useState(false);
  const [clamped, setClamped] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);

  const { text, images } = splitPastedImages(message.content);

  useEffect(() => {
    const el = contentRef.current;
    if (el) setClamped(el.scrollHeight > el.clientHeight);
  }, [message.content]);

  return (
    <div className="flex flex-col items-end md:flex-row md:justify-end gap-1 md:gap-3">
      <div className="max-w-[90%] md:max-w-[75%]">
        <div
          className="chat-bubble rounded-2xl rounded-tr-md px-4 py-2.5 text-base font-medium text-text-1 cursor-pointer"
          onClick={() => clamped && setExpanded(!expanded)}
        >
          <div
            ref={contentRef}
            className={`prose-chat prose-chat-user ${!expanded ? 'line-clamp-5' : ''}`}
          >
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={mentionComponents(roles)}>{text}</ReactMarkdown>
          </div>
          {images.length > 0 && (
            <div className={`flex flex-wrap gap-2 ${text ? 'mt-2.5' : ''}`}>
              {images.map(img => (
                <img
                  key={img.path}
                  src={localFileSrc(img.path)}
                  alt={img.name}
                  title={img.name}
                  loading="lazy"
                  className="max-h-40 max-w-[220px] rounded-lg border border-border-1 object-cover"
                />
              ))}
            </div>
          )}
          {clamped && (
            <button className="text-xs text-accent-primary mt-1 hover:underline cursor-pointer">
              {expanded ? 'Show less' : 'Show more'}
            </button>
          )}
        </div>
        <ReactionBar
          reactions={message.reactions}
          onReact={(emoji) => onReact?.(message.id, emoji)}
          trailing={<ThreadAction count={message.thread_reply_count} onOpen={onOpenThread ? () => onOpenThread(message) : undefined} />}
        />
        <div className="flex items-center gap-2 mt-1 px-1 text-[10px] text-text-3 justify-end">
          <span>{new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
        </div>
      </div>
    </div>
  );
}

/**
 * The summary left behind by context compaction. It stands in for messages that
 * no longer exist, so it gets a labelled 2px accent border rather than looking
 * like just another reply in the thread.
 */
function ChatSummaryBubble({ message, roles }: { message: ChatMessage; roles: AgentRole[] }) {
  // mb-8 gives a generous bottom gap so the summary reads as a divider between
  // the compacted history and what follows, not as part of the next message.
  return (
    <div className="mt-2 mb-8">
      {/* Dark, near-opaque fill: the page background art shows through a
          transparent card and makes the summary text hard to read. */}
      <div className="rounded-2xl border-2 border-accent-primary bg-surface-0/95 overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-2 border-b-2 border-accent-primary/40 bg-black/30">
          <Minimize2 className="w-3.5 h-3.5 text-accent-primary flex-shrink-0" aria-hidden="true" />
          <span className="text-xs font-semibold uppercase tracking-wider text-accent-primary">
            Chat Summary
          </span>
          <span className="text-[10px] text-text-3 ml-auto">
            Earlier messages were compacted
          </span>
        </div>
        <div className="px-6 py-6 md:px-8 md:py-7 text-base font-medium text-text-1">
          <div className="prose-chat prose-measure">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={mentionComponents(roles)}>{message.content}</ReactMarkdown>
          </div>
        </div>
      </div>
    </div>
  );
}

export function MessageBubble({ message, roles, onRefresh, onReact, onOpenThread }: { message: ChatMessage; roles: AgentRole[]; onRefresh: () => void; userAvatarPath?: string; onReact?: (messageId: string, emoji: string) => void; onOpenThread?: (message: ChatMessage) => void }) {
  const isUser = message.role === 'user';
  const role = resolveMessageRole(roles, message.agent_role_slug);

  const [toolsOpen, setToolsOpen] = useState(false);

  if (isUser) return <UserMessageBubble message={message} roles={roles} onReact={onReact} onOpenThread={onOpenThread} />;
  if (message.role === 'system') return <ChatSummaryBubble message={message} roles={roles} />;

  const confirmation = parseConfirmation(message.content);
  const toolSummary = !confirmation ? parseToolSummary(message.content) : null;
  const widgets = parseWidgets(message.widget_data);
  const toolCalls = message.tool_calls ?? [];
  const hasTools = toolCalls.length > 0;

  return (
    <div className="flex flex-col md:flex-row gap-1 md:gap-3">
      <div className="flex items-center gap-2 md:block">
        <CompanionAvatar role={role} />
        {role && (
          <p className="text-sm font-semibold text-accent-primary md:hidden">{role.name}</p>
        )}
      </div>
      <div className="max-w-full md:max-w-[75%]">
        {role && (
          <p className="text-xs font-medium text-accent-primary mb-0.5 px-1 hidden md:block">{role.name}</p>
        )}
        {confirmation ? (
          <ConfirmationCardUI card={confirmation} threadId={message.thread_id} onUpdate={onRefresh} />
        ) : toolSummary ? (
          <ToolSummaryCardUI card={toolSummary} />
        ) : (
          <>
            <div className="chat-bubble rounded-2xl rounded-tl-md px-5 py-4 md:px-6 md:py-5 text-base font-medium text-text-1">
              <div className="prose-chat">
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={mentionComponents(roles)}>{cleanToolColons(message.content, (message.tool_calls?.length ?? 0) > 0)}</ReactMarkdown>
              </div>
              {/* Inside the bubble, under the text: this reply is partial, and
                  saying so next to the words is the only way to tell. */}
              {message.stopped && (
                <div className="mt-3 pt-2.5 border-t border-border-0 flex items-center gap-1.5 text-[11px] font-semibold text-text-3">
                  <OctagonX className="w-3 h-3 flex-shrink-0" aria-hidden="true" />
                  <span>Stopped by you — this reply is incomplete</span>
                </div>
              )}
            </div>
            {message.image_url && (
              <div className="mt-2 px-1">
                <div className="relative group inline-block rounded-xl overflow-hidden border border-border-1">
                  <img
                    src={message.image_url}
                    alt="Generated image"
                    className="max-w-full max-h-[400px] rounded-xl object-contain"
                  />
                  <a
                    href={message.image_url}
                    download
                    className="absolute top-2 right-2 p-1.5 rounded-lg bg-black/50 text-white opacity-0 group-hover:opacity-100 transition-opacity hover:bg-black/70"
                    title="Download image"
                  >
                    <Download className="w-4 h-4" />
                  </a>
                </div>
              </div>
            )}
            {widgets && widgets.map((w, i) => (
              <WidgetRenderer key={`w-${message.id}-${i}`} widget={w} />
            ))}
          </>
        )}
        {hasTools && toolsOpen && (
          <ToolActivityPanel tools={toolCalls} defaultExpanded />
        )}
        <ReactionBar
          reactions={message.reactions}
          onReact={(emoji) => onReact?.(message.id, emoji)}
          trailing={
            <>
              {hasTools && (
                <button
                  type="button"
                  onClick={() => setToolsOpen((o) => !o)}
                  aria-expanded={toolsOpen}
                  aria-label={toolsOpen ? 'Hide tool calls' : 'Show tool calls'}
                  title={`${toolCalls.length} tool call${toolCalls.length !== 1 ? 's' : ''}`}
                  className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded-full text-sm border cursor-pointer transition-colors ${
                    toolsOpen
                      ? 'bg-surface-2 border-border-1 text-text-1'
                      : 'border-transparent text-text-3 hover:text-text-1 hover:bg-surface-2'
                  }`}
                >
                  <Wrench className="w-3.5 h-3.5" />
                  {toolCalls.length > 1 && <span className="text-xs">{toolCalls.length}</span>}
                </button>
              )}
              <ThreadAction count={message.thread_reply_count} onOpen={onOpenThread ? () => onOpenThread(message) : undefined} />
            </>
          }
        />
        <div className="flex items-center gap-2 mt-1 px-1 text-[10px] text-text-3">
          <span>{new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
          {message.cost_usd > 0 && (
            <>
              <span className="flex items-center gap-0.5"><DollarSign className="w-2.5 h-2.5" />{message.cost_usd.toFixed(4)}</span>
              <span className="flex items-center gap-0.5"><Zap className="w-2.5 h-2.5" />{((message.input_tokens || 0) + (message.output_tokens || 0)).toLocaleString()} tokens</span>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
