import { useState, useEffect, useRef, type ReactNode } from 'react';
import { DollarSign, Zap, Download, Wrench } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { ChatMessage, AgentRole, WidgetPayload, SubAgentTask, Reaction } from '../../lib/api';
import { parseConfirmation, parseToolSummary, parseWidgets } from '../../lib/api';
import { cleanToolColons, type StreamingTool, type CostInfo } from '../../lib/chatUtils';
import { mentionComponents } from './MentionSystem';
import { ToolActivityPanel, StreamingToolPanel } from './ToolPanels';
import { ConfirmationCardUI, ToolSummaryCardUI } from './Cards';
import { WidgetRenderer } from '../widgets/WidgetRenderer';
import { SubAgentPanel } from './SubAgentPanel';
import { EmojiPicker } from './EmojiPicker';

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

function ThinkingIndicator({ label = 'Thinking…' }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 px-1 py-1.5" aria-live="polite">
      <div className="flex items-center gap-1">
        {[0, 150, 300].map((d) => (
          <span
            key={d}
            className="w-1.5 h-1.5 rounded-full bg-accent-primary animate-bounce"
            style={{ animationDelay: `${d}ms` }}
          />
        ))}
      </div>
      <span className="text-sm text-text-2 font-medium">{label}</span>
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
        <div className="w-7 h-7 md:w-8 md:h-8 rounded-md bg-surface-2 flex items-center justify-center flex-shrink-0 overflow-hidden ring-1 ring-border-1">
          {role ? (
            <img src={role.avatar_path} alt={role.name} className="w-7 h-7 md:w-8 md:h-8 rounded-md object-cover" />
          ) : (
            <img src={roles.find(r => r.slug === 'builder')?.avatar_path || '/gateway-avatar.png'} alt="AI" className="w-7 h-7 md:w-8 md:h-8 rounded-md object-cover" />
          )}
        </div>
        {role && (
          <p className="text-sm font-semibold text-accent-primary md:hidden">{role.name}</p>
        )}
      </div>
      <div className="max-w-full md:max-w-[75%]">
        {role && (
          <p className="text-xs font-medium text-accent-primary mb-0.5 px-1 hidden md:block">{role.name}</p>
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

function UserMessageBubble({ message, roles, onReact }: { message: ChatMessage; roles: AgentRole[]; onReact?: (messageId: string, emoji: string) => void }) {
  const [expanded, setExpanded] = useState(false);
  const [clamped, setClamped] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);

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
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={mentionComponents(roles)}>{message.content}</ReactMarkdown>
          </div>
          {clamped && (
            <button className="text-xs text-accent-primary mt-1 hover:underline cursor-pointer">
              {expanded ? 'Show less' : 'Show more'}
            </button>
          )}
        </div>
        <ReactionBar reactions={message.reactions} onReact={(emoji) => onReact?.(message.id, emoji)} />
        <div className="flex items-center gap-2 mt-1 px-1 text-[10px] text-text-3 justify-end">
          <span>{new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
        </div>
      </div>
    </div>
  );
}

export function MessageBubble({ message, roles, onRefresh, onReact }: { message: ChatMessage; roles: AgentRole[]; onRefresh: () => void; userAvatarPath?: string; onReact?: (messageId: string, emoji: string) => void }) {
  const isUser = message.role === 'user';
  const role = message.agent_role_slug ? roles.find(r => r.slug === message.agent_role_slug) : null;

  const [toolsOpen, setToolsOpen] = useState(false);

  if (isUser) return <UserMessageBubble message={message} roles={roles} onReact={onReact} />;

  const confirmation = parseConfirmation(message.content);
  const toolSummary = !confirmation ? parseToolSummary(message.content) : null;
  const widgets = parseWidgets(message.widget_data);
  const toolCalls = message.tool_calls ?? [];
  const hasTools = toolCalls.length > 0;

  return (
    <div className="flex flex-col md:flex-row gap-1 md:gap-3">
      <div className="flex items-center gap-2 md:block">
        <div className="w-7 h-7 md:w-8 md:h-8 rounded-md bg-surface-2 flex items-center justify-center flex-shrink-0 overflow-hidden ring-1 ring-border-1">
          {role ? (
            <img src={role.avatar_path} alt={role.name} className="w-7 h-7 md:w-8 md:h-8 rounded-md object-cover" />
          ) : (
            <img src={roles.find(r => r.slug === 'builder')?.avatar_path || '/gateway-avatar.png'} alt="AI" className="w-7 h-7 md:w-8 md:h-8 rounded-md object-cover" />
          )}
        </div>
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
            <div className="chat-bubble rounded-2xl rounded-tl-md px-4 py-2.5 text-base font-medium text-text-1">
              <div className="prose-chat">
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={mentionComponents(roles)}>{cleanToolColons(message.content, (message.tool_calls?.length ?? 0) > 0)}</ReactMarkdown>
              </div>
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
          trailing={hasTools ? (
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
          ) : undefined}
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
