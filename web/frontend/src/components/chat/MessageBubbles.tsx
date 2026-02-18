import { useState, useEffect, useRef } from 'react';
import { DollarSign, Zap, Users, Download } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { ChatMessage, AgentRole, WidgetPayload } from '../../lib/api';
import { parseConfirmation, parseToolSummary, parseWidgets } from '../../lib/api';
import { cleanToolColons, type StreamingTool, type CostInfo } from '../../lib/chatUtils';
import { mentionComponents } from './MentionSystem';
import { ToolActivityPanel, StreamingToolPanel } from './ToolPanels';
import { ConfirmationCardUI, ToolSummaryCardUI } from './Cards';
import { WidgetRenderer } from '../widgets/WidgetRenderer';

export function StreamingMessage({ text, tools, cost, role, roles, widgets }: {
  text: string;
  tools: StreamingTool[];
  cost: CostInfo | null;
  role: AgentRole | null;
  roles: AgentRole[];
  widgets?: WidgetPayload[];
}) {
  return (
    <div className="flex flex-col md:flex-row gap-1 md:gap-3">
      <div className="flex items-center gap-2 md:block">
        <div className="w-7 h-7 md:w-8 md:h-8 rounded-full bg-surface-2 flex items-center justify-center flex-shrink-0 overflow-hidden ring-1 ring-border-1">
          {role ? (
            <img src={role.avatar_path} alt={role.name} className="w-7 h-7 md:w-8 md:h-8 rounded-full object-cover" />
          ) : (
            <img src="/gateway-avatar.png" alt="AI" className="w-7 h-7 md:w-8 md:h-8 rounded-full object-cover" />
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
        {text && (
          <div className="text-base font-medium text-text-1 px-1">
            <div className="prose-chat">
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={mentionComponents(roles)}>{cleanToolColons(text, tools.length > 0)}</ReactMarkdown>
              <span className="inline-block w-0.5 h-4 bg-accent-primary animate-pulse ml-0.5 align-text-bottom" />
            </div>
          </div>
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

function UserMessageBubble({ message, roles, avatarPath }: { message: ChatMessage; roles: AgentRole[]; avatarPath?: string }) {
  const [expanded, setExpanded] = useState(false);
  const [clamped, setClamped] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = contentRef.current;
    if (el) setClamped(el.scrollHeight > el.clientHeight);
  }, [message.content]);

  return (
    <div className="flex flex-col items-end md:flex-row md:justify-end gap-1 md:gap-3">
      <div className="w-7 h-7 md:w-8 md:h-8 rounded-full overflow-hidden flex items-center justify-center bg-accent-muted flex-shrink-0 ring-1 ring-border-1 md:order-2">
        {avatarPath ? (
          <img src={avatarPath} alt="You" className="w-7 h-7 md:w-8 md:h-8 rounded-full object-cover" />
        ) : (
          <Users className="w-4 h-4 text-accent-primary" />
        )}
      </div>
      <div className="max-w-[90%] md:max-w-[75%] md:order-1">
        <div
          className="rounded-2xl rounded-tr-md px-4 py-2.5 text-base font-medium bg-surface-1 text-text-1 cursor-pointer"
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
        <div className="flex items-center gap-2 mt-1 px-1 text-[10px] text-text-3 justify-end">
          <span>{new Date(message.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
        </div>
      </div>
    </div>
  );
}

export function MessageBubble({ message, roles, onRefresh, userAvatarPath }: { message: ChatMessage; roles: AgentRole[]; onRefresh: () => void; userAvatarPath?: string }) {
  const isUser = message.role === 'user';
  const role = message.agent_role_slug ? roles.find(r => r.slug === message.agent_role_slug) : null;

  if (isUser) return <UserMessageBubble message={message} roles={roles} avatarPath={userAvatarPath} />;

  const confirmation = parseConfirmation(message.content);
  const toolSummary = !confirmation ? parseToolSummary(message.content) : null;
  const widgets = parseWidgets(message.widget_data);

  return (
    <div className="flex flex-col md:flex-row gap-1 md:gap-3">
      <div className="flex items-center gap-2 md:block">
        <div className="w-7 h-7 md:w-8 md:h-8 rounded-full bg-surface-2 flex items-center justify-center flex-shrink-0 overflow-hidden ring-1 ring-border-1">
          {role ? (
            <img src={role.avatar_path} alt={role.name} className="w-7 h-7 md:w-8 md:h-8 rounded-full object-cover" />
          ) : (
            <img src="/gateway-avatar.png" alt="AI" className="w-7 h-7 md:w-8 md:h-8 rounded-full object-cover" />
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
            <div className="text-base font-medium text-text-1 px-1">
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
            {message.tool_calls && message.tool_calls.length > 0 && (
              <ToolActivityPanel tools={message.tool_calls} />
            )}
          </>
        )}
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
