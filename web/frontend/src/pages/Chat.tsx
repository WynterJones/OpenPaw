import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useParams, useNavigate } from 'react-router';
import {
  Plus, MessageSquare, ArrowUp,
  ChevronDown, ChevronLeft, ChevronRight, ChevronUp, PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen, Loader2, Trash2, Pencil, Check, X,
  Coins, Zap, Minimize2, Square, Users, AlertTriangle,
  Paperclip, FileText, FolderOpen, FolderPlus, ListTodo, Bot, CircleCheck, ImageIcon, Wrench, Pin, Sparkles,
} from 'lucide-react';
import { Header } from '../components/Header';
import { Button } from '../components/Button';
import { SearchBar } from '../components/SearchBar';
import { Modal } from '../components/Modal';
import { api, type ChatThread, type ChatMessage, type AgentRole, type StreamEvent, type WSMessage, type ThreadStats, type ThreadMember, type SubAgentTask, contextApi, type ContextFile, type ContextTree, type ContextTreeNode, threadMembers } from '../lib/api';
import { todoApi, mediaApi, threadPins } from '../lib/api-helpers';
import { useOpenRouterBalance } from '../hooks/useOpenRouterBalance';
import { CLI_CONTEXT_LIMIT } from '../lib/provider';
import type { TodoItem, MediaItem, Tool, ThreadPin } from '../lib/types';
import { useToast } from '../components/Toast';
import { useAuth } from '../contexts/AuthContext';
import { useWebSocket } from '../lib/useWebSocket';
import { detectBestWidget } from '../components/widgets/detectWidget';
import { timeAgo, getToolDetail } from '../lib/chatUtils';
import { MessageBubble, StreamingMessage } from '../components/chat/MessageBubbles';
import { useThreadList } from '../hooks/useThreadList';
import { useStreamingState } from '../hooks/useStreamingState';
import { useAutocomplete } from '../hooks/useAutocomplete';
import { useViewToggles } from '../contexts/ViewTogglesContext';

type ContextItem =
  | { kind: 'file'; file: ContextFile }
  | { kind: 'folder'; folder: ContextTreeNode; files: ContextFile[] };

function collectFolderFiles(node: ContextTreeNode): ContextFile[] {
  const files = [...node.files];
  for (const child of node.children) {
    files.push(...collectFolderFiles(child));
  }
  return files;
}

function buildContextItems(tree: ContextTree): ContextItem[] {
  const items: ContextItem[] = [];
  for (const folder of tree.folders) {
    const files = collectFolderFiles(folder);
    if (files.length > 0) {
      items.push({ kind: 'folder', folder, files });
      for (const file of files) {
        items.push({ kind: 'file', file });
      }
    }
  }
  for (const file of tree.files) {
    items.push({ kind: 'file', file });
  }
  return items;
}

function ToolPickerRow({ tool, selected, onSelect }: { tool: Tool; selected: boolean; onSelect: () => void }) {
  return (
    <button
      role="option"
      aria-selected={selected}
      onClick={onSelect}
      className={`w-full flex items-center gap-3 px-4 py-2 text-left transition-colors cursor-pointer ${
        selected ? 'bg-accent-muted' : 'hover:bg-surface-2'
      }`}
    >
      <Wrench className="w-4 h-4 text-accent-primary flex-shrink-0" aria-hidden="true" />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-text-0 truncate">{tool.name}</p>
        {tool.description && (
          <p className="text-xs text-text-3 truncate">{tool.description}</p>
        )}
      </div>
      {tool.status !== 'running' && (
        <span className="text-[10px] uppercase tracking-wide text-text-3 flex-shrink-0">{tool.status}</span>
      )}
    </button>
  );
}

export function Chat() {
  const { threadId: urlThreadId } = useParams<{ threadId?: string }>();
  const chatNavigate = useNavigate();
  const { user } = useAuth();
  const { toast } = useToast();
  const {
    threads, setThreads,
    threadSearch: search, setThreadSearch: setSearch,
    threadPage, setThreadPage,
    editingThread, setEditingThread,
    editTitle, setEditTitle,
    deleteTarget, setDeleteTarget,
    editInputRef,
  } = useThreadList();
  const {
    streamingText, setStreamingText, appendStreamingText,
    streamingTools, setStreamingTools,
    streamingWidgets, setStreamingWidgets,
    costInfo, setCostInfo,
    thinkingText, setThinkingText,
    resetStreaming,
  } = useStreamingState();
  const {
    mentionOpen, setMentionOpen,
    mentionFilter, setMentionFilter,
    mentionIndex, setMentionIndex,
    mentionAnchorRef,
    contextOpen, setContextOpen,
    contextFilter, setContextFilter,
    contextIndex, setContextIndex,
    contextAnchorRef,
    mediaOpen, setMediaOpen,
    mediaFilter, setMediaFilter,
    mediaIndex, setMediaIndex,
    mediaAnchorRef,
    toolOpen, setToolOpen,
    toolFilter, setToolFilter,
    toolIndex, setToolIndex,
    toolAnchorRef,
  } = useAutocomplete();
  const [activeThread, setActiveThread] = useState<string | null>(() => localStorage.getItem('openpaw_active_thread'));
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [agent] = useState('');
  const [showThreads, setShowThreads] = useState(() => window.innerWidth >= 768);
  // Chat list + right panel visibility come from the header's view-options
  // menu so all three layout toggles live in one place.
  const { chatList, chatPanel, set: setViewToggle } = useViewToggles();
  const threadsCollapsed = !chatList;
  const [sending, setSending] = useState(false);
  const [thinking, setThinking] = useState(false);
  const [streamActive, setStreamActive] = useState(false);
  const [workStatus, setWorkStatus] = useState<string | null>(null);
  const [roles, setRoles] = useState<AgentRole[]>([]);
  const pollingRef = useRef<number | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const THREADS_PER_PAGE = 10;

  const [contextItems, setContextItems] = useState<ContextItem[]>([]);
  const [attachedContextFiles, setAttachedContextFiles] = useState<ContextFile[]>([]);
  const [mediaItems, setMediaItems] = useState<MediaItem[]>([]);

  // Tools attachable via the `#` trigger. `/tools` already returns this
  // workspace's tools plus global ones (workspace_id === null).
  const [toolItems, setToolItems] = useState<Tool[]>([]);
  const [attachedTools, setAttachedTools] = useState<Tool[]>([]);
  const [toolsLoaded, setToolsLoaded] = useState(false);

  // Pinned (archived) chats: the sidebar tab, the explainer modal target, and
  // the pin state of the thread currently open.
  const [threadTab, setThreadTab] = useState<'all' | 'pinned'>('all');
  const [pinTarget, setPinTarget] = useState<ChatThread | null>(null);
  const [pinning, setPinning] = useState(false);
  const [activePin, setActivePin] = useState<ThreadPin | null>(null);
  const [transcriptOpen, setTranscriptOpen] = useState(false);

  // File attachment state
  const [pendingAttachments, setPendingAttachments] = useState<File[]>([]);
  const [attachedDirectories, setAttachedDirectories] = useState<string[]>([]);
  const attachInputRef = useRef<HTMLInputElement>(null);

  const autoResize = (el: HTMLTextAreaElement) => {
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 350) + 'px';
  };

  const userRoles = useMemo(() => roles.filter(r => r.slug !== 'builder'), [roles]);

  const filteredMentionRoles = useMemo(() => userRoles.filter(r =>
    r.name.toLowerCase().includes(mentionFilter.toLowerCase()) ||
    r.slug.toLowerCase().includes(mentionFilter.toLowerCase())
  ), [userRoles, mentionFilter]);

  const filteredContextItems = useMemo(() => contextItems.filter(item => {
    const name = item.kind === 'file' ? item.file.name : item.folder.name;
    return name.toLowerCase().includes(contextFilter.toLowerCase());
  }), [contextItems, contextFilter]);

  const filteredMediaItems = useMemo(() => mediaItems.filter(item =>
    item.prompt.toLowerCase().includes(mediaFilter.toLowerCase()) ||
    item.source_model.toLowerCase().includes(mediaFilter.toLowerCase())
  ), [mediaItems, mediaFilter]);

  // `#` picker: filter by name/description, then split into workspace vs global
  // sections. `flatTools` keeps one running index so arrow keys move across
  // both sections as a single list.
  const filteredTools = useMemo(() => {
    const q = toolFilter.toLowerCase();
    const attachedIds = new Set(attachedTools.map(t => t.id));
    return toolItems.filter(t =>
      !attachedIds.has(t.id) &&
      (t.name.toLowerCase().includes(q) || (t.description || '').toLowerCase().includes(q))
    );
  }, [toolItems, toolFilter, attachedTools]);

  const workspaceTools = useMemo(() => filteredTools.filter(t => !!t.workspace_id), [filteredTools]);
  const globalTools = useMemo(() => filteredTools.filter(t => !t.workspace_id), [filteredTools]);
  const flatTools = useMemo(() => [...workspaceTools, ...globalTools], [workspaceTools, globalTools]);

  // The picker is only "live" (rendered + capturing keys) when it has matches
  // or the workspace has no tools at all.
  const toolPickerVisible = toolOpen && toolsLoaded && (flatTools.length > 0 || toolItems.length === 0);

  const insertMention = (role: AgentRole) => {
    const ta = textareaRef.current;
    if (!ta || mentionAnchorRef.current === null) return;
    const before = input.slice(0, mentionAnchorRef.current);
    const after = input.slice(ta.selectionStart);
    const newValue = `${before}@${role.name} ${after}`;
    setInput(newValue);
    setMentionOpen(false);
    setMentionFilter('');
    mentionAnchorRef.current = null;
    setTimeout(() => {
      const pos = before.length + role.name.length + 2;
      ta.selectionStart = pos;
      ta.selectionEnd = pos;
      ta.focus();
      autoResize(ta);
    }, 0);
  };

  const insertContextItem = (item: ContextItem) => {
    const ta = textareaRef.current;
    if (!ta || contextAnchorRef.current === null) return;
    const before = input.slice(0, contextAnchorRef.current);
    const after = input.slice(ta.selectionStart);

    if (item.kind === 'file') {
      const tag = `[[${item.file.name}]]`;
      const newValue = `${before}${tag} ${after}`;
      setInput(newValue);
      if (!attachedContextFiles.some(f => f.id === item.file.id)) {
        setAttachedContextFiles(prev => [...prev, item.file]);
      }
      setTimeout(() => {
        const pos = before.length + tag.length + 1;
        ta.selectionStart = pos;
        ta.selectionEnd = pos;
        ta.focus();
        autoResize(ta);
      }, 0);
    } else {
      const tag = `[[${item.folder.name}/]]`;
      const newValue = `${before}${tag} ${after}`;
      setInput(newValue);
      setAttachedContextFiles(prev => {
        const existingIds = new Set(prev.map(f => f.id));
        const newFiles = item.files.filter(f => !existingIds.has(f.id));
        return [...prev, ...newFiles];
      });
      setTimeout(() => {
        const pos = before.length + tag.length + 1;
        ta.selectionStart = pos;
        ta.selectionEnd = pos;
        ta.focus();
        autoResize(ta);
      }, 0);
    }

    setContextOpen(false);
    setContextFilter('');
    contextAnchorRef.current = null;
  };

  // Load context tree for !! autocomplete (files + folders)
  const loadContextItems = async () => {
    try {
      const tree = await contextApi.tree();
      if (tree) setContextItems(buildContextItems(tree));
    } catch (e) { console.warn('loadContextItems failed:', e); }
  };

  // Load media items for @@ autocomplete
  const loadMediaItems = async () => {
    try {
      const data = await mediaApi.list({ per_page: 20, type: 'image' });
      if (data?.items) setMediaItems(data.items);
    } catch { /* media library may not have items yet */ }
  };

  const insertMediaRef = (item: MediaItem) => {
    const ta = textareaRef.current;
    if (!ta || mediaAnchorRef.current === null) return;
    const before = input.slice(0, mediaAnchorRef.current);
    const after = input.slice(ta.selectionStart);
    const label = item.prompt.slice(0, 40) || item.filename || 'image';
    const tag = `@@[${label}](${item.id})`;
    const newValue = `${before}${tag} ${after}`;
    setInput(newValue);
    setMediaOpen(false);
    setMediaFilter('');
    mediaAnchorRef.current = null;
    setTimeout(() => {
      const pos = before.length + tag.length + 1;
      ta.selectionStart = pos;
      ta.selectionEnd = pos;
      ta.focus();
      autoResize(ta);
    }, 0);
  };

  // Load tools for the `#` picker. `/tools` is already scoped to the active
  // workspace + globals, so no extra filtering is needed here.
  const loadToolItems = async () => {
    try {
      const data = await api.get<Tool[]>('/tools');
      setToolItems(Array.isArray(data) ? data : []);
    } catch (e) {
      console.warn('loadToolItems failed:', e);
    } finally {
      setToolsLoaded(true);
    }
  };

  // Attaching a tool strips the `#query` text and adds a chip instead — the
  // tool travels in the `tools` payload field, not in the message text.
  const attachTool = (tool: Tool) => {
    const ta = textareaRef.current;
    if (!ta || toolAnchorRef.current === null) return;
    const before = input.slice(0, toolAnchorRef.current);
    const after = input.slice(ta.selectionStart);
    const newValue = `${before}${after}`;
    setInput(newValue);
    setAttachedTools(prev => prev.some(t => t.id === tool.id) ? prev : [...prev, tool]);
    setToolOpen(false);
    setToolFilter('');
    toolAnchorRef.current = null;
    setTimeout(() => {
      ta.selectionStart = before.length;
      ta.selectionEnd = before.length;
      ta.focus();
      autoResize(ta);
    }, 0);
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value;
    const cursorPos = e.target.selectionStart;
    setInput(value);
    autoResize(e.target);

    // Detect @ trigger
    const textBefore = value.slice(0, cursorPos);
    const atMatch = textBefore.match(/@(\w*)$/);
    if (atMatch) {
      mentionAnchorRef.current = cursorPos - atMatch[0].length;
      setMentionFilter(atMatch[1]);
      setMentionOpen(true);
      setMentionIndex(0);
      setContextOpen(false);
    } else {
      setMentionOpen(false);
      setMentionFilter('');
      mentionAnchorRef.current = null;
    }

    // Detect !! trigger for context file insertion
    const bangMatch = textBefore.match(/!!(\w*)$/);
    if (bangMatch && !atMatch) {
      contextAnchorRef.current = cursorPos - bangMatch[0].length;
      setContextFilter(bangMatch[1]);
      setContextOpen(true);
      setContextIndex(0);
      if (contextItems.length === 0) loadContextItems();
    } else if (!bangMatch) {
      setContextOpen(false);
      setContextFilter('');
      contextAnchorRef.current = null;
    }

    // Detect @@ trigger for media library references
    const mediaMatch = textBefore.match(/@@(\w*)$/);
    if (mediaMatch && !atMatch && !bangMatch) {
      mediaAnchorRef.current = cursorPos - mediaMatch[0].length;
      setMediaFilter(mediaMatch[1]);
      setMediaOpen(true);
      setMediaIndex(0);
      if (mediaItems.length === 0) loadMediaItems();
    } else if (!mediaMatch) {
      setMediaOpen(false);
      setMediaFilter('');
      mediaAnchorRef.current = null;
    }

    // Detect # trigger for attaching Tools (only at the start of the input or
    // after whitespace, so `#` inside a word or a markdown heading is ignored).
    const toolMatch = textBefore.match(/(^|\s)#([\w-]*)$/);
    if (toolMatch) {
      const query = toolMatch[2];
      toolAnchorRef.current = cursorPos - query.length - 1;
      setToolFilter(query);
      setToolOpen(true);
      setToolIndex(0);
      if (!toolsLoaded) loadToolItems();
    } else {
      setToolOpen(false);
      setToolFilter('');
      toolAnchorRef.current = null;
    }
  };

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // Tools picker (#). Escape is handled even with an empty list so the
    // "no tools available" state can still be dismissed.
    if (toolPickerVisible) {
      if (e.key === 'Escape') {
        e.preventDefault();
        setToolOpen(false);
        return;
      }
      if (flatTools.length > 0) {
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          setToolIndex(i => (i + 1) % flatTools.length);
          return;
        }
        if (e.key === 'ArrowUp') {
          e.preventDefault();
          setToolIndex(i => (i - 1 + flatTools.length) % flatTools.length);
          return;
        }
        if (e.key === 'Enter' || e.key === 'Tab') {
          e.preventDefault();
          attachTool(flatTools[Math.min(toolIndex, flatTools.length - 1)]);
          return;
        }
      }
    }

    // Context file/folder autocomplete
    if (contextOpen && filteredContextItems.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setContextIndex(i => (i + 1) % filteredContextItems.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setContextIndex(i => (i - 1 + filteredContextItems.length) % filteredContextItems.length);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        insertContextItem(filteredContextItems[contextIndex]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setContextOpen(false);
        return;
      }
    }

    if (mediaOpen && filteredMediaItems.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setMediaIndex(i => (i + 1) % filteredMediaItems.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setMediaIndex(i => (i - 1 + filteredMediaItems.length) % filteredMediaItems.length);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        insertMediaRef(filteredMediaItems[mediaIndex]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setMediaOpen(false);
        return;
      }
    }

    if (mentionOpen && filteredMentionRoles.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setMentionIndex(i => (i + 1) % filteredMentionRoles.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setMentionIndex(i => (i - 1 + filteredMentionRoles.length) % filteredMentionRoles.length);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        insertMention(filteredMentionRoles[mentionIndex]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setMentionOpen(false);
        return;
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  const [thinkingExpanded, setThinkingExpanded] = useState(false);
  const [activeThreadIds, setActiveThreadIds] = useState<Set<string>>(new Set());
  const [unreadThreadIds, setUnreadThreadIds] = useState<Set<string>>(new Set());
  const [threadStats, setThreadStats] = useState<ThreadStats | null>(null);
  const balance = useOpenRouterBalance();
  const [compacting, setCompacting] = useState(false);
  const [showCompactConfirm, setShowCompactConfirm] = useState(false);
  const [members, setMembers] = useState<ThreadMember[]>([]);
  const showRightPanel = chatPanel;
  const setShowRightPanel = (next: boolean | ((v: boolean) => boolean)) => {
    const value = typeof next === 'function' ? next(showRightPanel) : next;
    setViewToggle('chatPanel', value);
  };
  const [rightPanelCollapsed, setRightPanelCollapsed] = useState(false);
  const [sectionAgents, setSectionAgents] = useState(true);
  const [sectionContext, setSectionContext] = useState(true);
  const [sectionTodos, setSectionTodos] = useState(false);
  const [todoLists, setTodoLists] = useState<{ id: string; name: string; color: string; total_items: number; completed_items: number }[]>([]);
  const [todoExpandedList, setTodoExpandedList] = useState<string | null>(null);
  const [todoItems, setTodoItems] = useState<Record<string, TodoItem[]>>({});
  const [routingIndicator, setRoutingIndicator] = useState<string | null>(null);
  const [activeAgentSlug, setActiveAgentSlug] = useState<string | null>(null);
  const [subAgentTasks, setSubAgentTasks] = useState<SubAgentTask[]>([]);
  const activeThreadRef = useRef(activeThread);
  activeThreadRef.current = activeThread;
  const todoExpandedListRef = useRef(todoExpandedList);
  todoExpandedListRef.current = todoExpandedList;
  const hadToolSinceLastTextRef = useRef(false);
  const toolInputMapRef = useRef<Map<string, { endpoint?: string }>>(new Map());
  const loadThreadsRef = useRef<(() => Promise<void>) | undefined>(undefined);

  // WebSocket handler
  const handleWSMessage = useCallback((msg: WSMessage) => {
    const payload = msg.payload;
    const threadId = payload?.thread_id as string | undefined;
    const isActiveThread = !threadId || threadId === activeThreadRef.current;

    // Thread title updated
    if (msg.type === 'thread_updated') {
      const title = payload?.title as string;
      if (threadId && title) {
        setThreads(prev => prev.map(t => t.id === threadId ? { ...t, title } : t));
      }
    }

    // Track which threads have active work (for sidebar indicators)
    if (msg.type === 'agent_status' && threadId) {
      const status = payload?.status as string;
      if (status === 'analyzing' || status === 'thinking' || status === 'spawning') {
        setActiveThreadIds(prev => { const next = new Set(prev); next.add(threadId); return next; });
      } else if (status === 'done') {
        setActiveThreadIds(prev => { const next = new Set(prev); next.delete(threadId); return next; });
        loadThreadsRef.current?.();
      }
    }

    if (msg.type === 'agent_completed' && threadId) {
      setActiveThreadIds(prev => { const next = new Set(prev); next.delete(threadId); return next; });
      loadThreadsRef.current?.();
    }

    // Thread member events
    if (msg.type === 'thread_member_joined' && threadId && threadId === activeThreadRef.current) {
      loadMembers(threadId);
    }
    if (msg.type === 'thread_member_removed' && threadId && threadId === activeThreadRef.current) {
      loadMembers(threadId);
    }

    // Live-update emoji reactions
    if (msg.type === 'message_reacted') {
      const messageId = payload?.message_id as string;
      const reactions = payload?.reactions as { emoji: string; source: string; count: number }[] | undefined;
      if (messageId) {
        setMessages(prev => {
          const updated = prev.map(m => m.id === messageId ? { ...m, reactions: reactions || [] } : m);
          const found = prev.some(m => m.id === messageId);
          if (!found) {
            console.warn('[reaction] message not found in state:', messageId, 'active:', activeThreadRef.current, 'thread:', threadId);
          }
          return updated;
        });
      }
    }

    // Live-update todo lists when agents modify them
    if (msg.type === 'todo_updated') {
      loadTodoLists();
      const listId = payload?.list_id as string;
      if (listId && todoExpandedListRef.current === listId) {
        loadTodoItems(listId);
      }
    }

    // Live-update agent avatars when agents change their own avatar
    if (msg.type === 'agent_avatar_updated') {
      loadRoles();
    }

    // Track unread messages for non-active threads
    if (!isActiveThread && threadId) {
      if (msg.type === 'agent_status') {
        const status = payload?.status as string;
        if (status === 'message_saved' || status === 'done') {
          setUnreadThreadIds(prev => { const next = new Set(prev); next.add(threadId); return next; });
        }
      }
      if (msg.type === 'agent_completed') {
        setUnreadThreadIds(prev => { const next = new Set(prev); next.add(threadId); return next; });
      }
    }

    if (!isActiveThread) return;

    // Gateway/role thinking stream
    if (msg.type === 'gateway_thinking') {
      const text = payload?.text as string;
      if (text) {
        setThinkingText(prev => prev + text);
      }
    }

    // Agent lifecycle status events
    if (msg.type === 'agent_status') {
      const status = payload?.status as string;
      const message = payload?.message as string;
      switch (status) {
        case 'routing': {
          const agentSlug = payload?.agent_role_slug as string;
          setRoutingIndicator(message);
          if (agentSlug) setActiveAgentSlug(agentSlug);
          setTimeout(() => setRoutingIndicator(null), 3000);
          break;
        }
        case 'compacting':
          setThinking(true);
          setWorkStatus(message || 'Auto-compacting context...');
          setRoutingIndicator(null);
          break;
        case 'analyzing':
        case 'thinking':
        case 'spawning':
          setThinking(true);
          setWorkStatus(message);
          setRoutingIndicator(null);
          break;
        case 'message_saved':
          setThinkingText('');
          if (threadId) loadMessages(threadId);
          break;
        case 'done':
          setThinking(false);
          setStreamActive(false);
          setWorkStatus(null);
          setThinkingText('');
          setRoutingIndicator(null);
          setActiveAgentSlug(null);
          stopPolling();
          if (threadId) { loadMessages(threadId); loadStats(threadId); }
          setTimeout(() => textareaRef.current?.focus(), 100);
          break;
      }
    }

    // Streaming events from agents
    if (msg.type === 'agent_stream' && payload?.event) {
      const event = payload.event as StreamEvent;
      const agentSlug = payload?.agent_role_slug as string;
      if (agentSlug) setActiveAgentSlug(agentSlug);
      setThinking(false);
      setStreamActive(true);
      switch (event.type) {
        case 'text_delta':
          if (event.text) {
            const needsSep = hadToolSinceLastTextRef.current;
            hadToolSinceLastTextRef.current = false;
            appendStreamingText(event.text, needsSep);
          }
          break;
        case 'tool_start':
          if (event.tool_name) {
            const detail = event.tool_input ? getToolDetail(event.tool_name!, event.tool_input) : '';
            const toolId = event.tool_id || `tool-${Date.now()}`;
            let endpoint: string | undefined;
            if (event.tool_name === 'call_tool' && event.tool_input) {
              endpoint = (event.tool_input.endpoint as string) || undefined;
              toolInputMapRef.current.set(toolId, { endpoint });
            }
            setStreamingTools(prev => {
              const existing = prev.findIndex(t => t.id === toolId);
              if (existing >= 0) {
                return prev.map((t, i) => i === existing ? { ...t, done: false, detail: detail || t.detail, endpoint: endpoint || t.endpoint } : t);
              }
              return [...prev, { name: event.tool_name!, id: toolId, done: false, detail, endpoint }];
            });
          }
          break;
        case 'tool_end':
          hadToolSinceLastTextRef.current = true;
          setStreamingTools(prev => prev.map(t => {
            if (event.tool_id && t.id === event.tool_id) return { ...t, done: true };
            if (!event.tool_id && t.name === event.tool_name && !t.done) return { ...t, done: true };
            return t;
          }));
          if (event.tool_output) {
            try {
              const parsed = JSON.parse(event.tool_output);
              if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
                const toolUuid = parsed.__tool_uuid || event.tool_id;
                const { __tool_uuid: _, ...withoutUuid } = parsed;
                void _;

                const storedInput = toolInputMapRef.current.get(event.tool_id || '');
                const endpoint = storedInput?.endpoint;

                if (withoutUuid.__widget) {
                  const { __widget, ...rest } = withoutUuid;
                  setStreamingWidgets(prev => [...prev, {
                    type: __widget.type || 'json-viewer',
                    title: __widget.title,
                    tool_id: toolUuid,
                    tool_name: event.tool_name,
                    endpoint,
                    data: rest,
                  }]);
                } else if (event.tool_name === 'call_tool') {
                  const detected = detectBestWidget(withoutUuid);
                  setStreamingWidgets(prev => [...prev, {
                    type: detected,
                    tool_id: toolUuid,
                    tool_name: event.tool_name,
                    endpoint,
                    data: withoutUuid,
                  }]);
                }
              }
            } catch (e) { console.warn('tool_end: failed to parse tool output JSON:', e); }
          }
          break;
        case 'result':
          if (event.total_cost_usd) {
            setCostInfo({
              total_cost_usd: event.total_cost_usd,
              usage: event.usage,
              num_turns: event.num_turns,
            });
          }
          break;
      }
    }

    // Sub-agent delegation events
    if (msg.type === 'subagent_status') {
      const status = payload?.status as string;
      const subagentId = payload?.subagent_id as string;

      if (status === 'started' && subagentId) {
        setSubAgentTasks(prev => [...prev, {
          subagent_id: subagentId,
          agent_slug: payload?.agent_slug as string || '',
          agent_name: payload?.agent_name as string || '',
          task_summary: payload?.task_summary as string || '',
          status: 'started',
        }]);
      } else if ((status === 'completed' || status === 'failed') && subagentId) {
        setSubAgentTasks(prev => prev.map(t =>
          t.subagent_id === subagentId
            ? {
                ...t,
                status: status as 'completed' | 'failed',
                result_preview: (payload?.result_preview as string) || t.streaming_text || '',
                cost_usd: (payload?.cost_usd as number) || 0,
              }
            : t
        ));
      }
    }

    if (msg.type === 'subagent_stream') {
      const subagentId = payload?.subagent_id as string;
      const text = payload?.text as string;
      if (subagentId && text) {
        setSubAgentTasks(prev => prev.map(t =>
          t.subagent_id === subagentId
            ? { ...t, streaming_text: (t.streaming_text || '') + text }
            : t
        ));
      }
    }

    if (msg.type === 'agent_completed') {
      resetStreaming();
      setStreamActive(false);
      setSubAgentTasks([]);
      setThinking(false);
      setWorkStatus(null);
      setRoutingIndicator(null);
      setActiveAgentSlug(null);
      stopPolling();
      if (threadId) {
        loadMessages(threadId);
        loadStats(threadId);
      }
      setTimeout(() => textareaRef.current?.focus(), 100);
    }
  }, [resetStreaming, setCostInfo, appendStreamingText, setStreamingTools, setStreamingWidgets, setThinkingText, setThreads]);

  const { connected: wsConnected } = useWebSocket({
    onMessage: handleWSMessage,
    enabled: true,
  });

  useEffect(() => {
    const init = async () => {
      try {
        const data = await api.get<ChatThread[]>('/chat/threads');
        const list = data || [];
        setThreads(list);
        // Clear stale activeThread from localStorage if thread no longer exists
        const stored = localStorage.getItem('openpaw_active_thread');
        if (stored && !list.some(t => t.id === stored)) {
          setActiveThread(null);
        }
        initActiveThreadIds();
      } catch (e) { console.warn('loadThreads init failed:', e); setThreads([]); setActiveThread(null); }
      loadRoles();
      loadContextItems();
      loadTodoLists();
    };
    init();
  }, [setThreads]);

  // Open thread from URL param (e.g. /chat/:threadId from notification click)
  useEffect(() => {
    if (urlThreadId && urlThreadId !== activeThread) {
      setActiveThread(urlThreadId);
      chatNavigate('/chat', { replace: true });
    }
  }, [urlThreadId]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    // Clear all transient streaming/thinking state when switching threads
    resetStreaming();
    setStreamActive(false);
    setSubAgentTasks([]);
    setThinkingExpanded(false);
    setThinking(false);
    setWorkStatus(null);
    hadToolSinceLastTextRef.current = false;
    toolInputMapRef.current.clear();
    stopPolling();

    setRoutingIndicator(null);
    setActiveAgentSlug(null);

    if (activeThread) {
      // Clear unread indicator when switching to a thread
      setUnreadThreadIds(prev => {
        if (!prev.has(activeThread)) return prev;
        const next = new Set(prev); next.delete(activeThread); return next;
      });

      // Immediately clear stale sidebar indicator; status check / WebSocket will restore if truly active
      setActiveThreadIds(prev => {
        if (!prev.has(activeThread)) return prev;
        const next = new Set(prev); next.delete(activeThread); return next;
      });

      localStorage.setItem('openpaw_active_thread', activeThread);
      loadMessages(activeThread);
      loadStats(activeThread);
      loadMembers(activeThread);
      // Pin state decides whether this thread renders as a read-only archive.
      setTranscriptOpen(false);
      threadPins.get(activeThread)
        .then(p => setActivePin(p.pinned ? p : null))
        .catch(() => setActivePin(null));

      // Recover streaming state if agent is mid-stream
      api.get<{ active: boolean; stream_state?: { text: string; tools: { name: string; id: string; done: boolean; detail?: string }[]; agent_slug: string; active: boolean } }>(`/chat/threads/${activeThread}/status`).then(status => {
        if (status.stream_state && status.stream_state.active) {
          const ss = status.stream_state;
          if (ss.text) setStreamingText(ss.text);
          if (ss.agent_slug) setActiveAgentSlug(ss.agent_slug);
          if (ss.tools && ss.tools.length > 0) {
            setStreamingTools(ss.tools.map(t => ({ name: t.name, id: t.id, done: t.done, detail: t.detail || '' })));
          }
          setStreamActive(true);
          setThinking(false);
          setWorkStatus(null);
          setActiveThreadIds(prev => { const next = new Set(prev); next.add(activeThread!); return next; });
        } else if (status.active) {
          setThinking(true);
          setWorkStatus('Working...');
          setActiveThreadIds(prev => { const next = new Set(prev); next.add(activeThread!); return next; });
        }
      }).catch((e) => { console.warn('recoverStreamState failed:', e); });

      setTimeout(() => textareaRef.current?.focus(), 100);
    } else {
      localStorage.removeItem('openpaw_active_thread');
      setMessages([]);
      setThreadStats(null);
      setMembers([]);
    }
  }, [activeThread, resetStreaming, setStreamingText, setStreamingTools]);

  useEffect(() => {
  }, [threadsCollapsed]);

  // Persist the composer draft per-thread so navigating away and back (which
  // unmounts this page) keeps whatever you'd typed. Restore on thread change,
  // save on edit, clear on send/empty.
  const draftHydratedRef = useRef(false);
  useEffect(() => {
    draftHydratedRef.current = false; // skip the persist triggered by this restore
    setInput(localStorage.getItem(`openpaw_chat_draft_${activeThread ?? 'new'}`) ?? '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeThread]);
  useEffect(() => {
    if (!draftHydratedRef.current) { draftHydratedRef.current = true; return; }
    const key = `openpaw_chat_draft_${activeThread ?? 'new'}`;
    if (input) localStorage.setItem(key, input);
    else localStorage.removeItem(key);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [input]);

  const scrollTimeoutRef = useRef<number>(0);
  useEffect(() => {
    if (!scrollTimeoutRef.current) {
      scrollTimeoutRef.current = window.setTimeout(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
        scrollTimeoutRef.current = 0;
      }, 150);
    }
  }, [messages, streamingText]);

  const initActiveThreadIds = async () => {
    try {
      const data = await api.get<{ active_thread_ids: string[] }>('/chat/threads/active');
      const candidates = data.active_thread_ids || [];
      if (candidates.length === 0) { setActiveThreadIds(new Set()); return; }
      const verified = new Set<string>();
      await Promise.all(candidates.map(async (id) => {
        try {
          const status = await api.get<{ active: boolean; stream_state?: { active: boolean } }>(`/chat/threads/${id}/status`);
          if (status.active || status.stream_state?.active) verified.add(id);
        } catch { /* thread may not exist */ }
      }));
      setActiveThreadIds(verified);
    } catch (e) { console.warn('initActiveThreadIds failed:', e); }
  };

  const loadThreads = async () => {
    try {
      const data = await api.get<ChatThread[]>('/chat/threads');
      const list = data || [];
      setThreads(list);
      if (activeThread && !list.some(t => t.id === activeThread)) {
        setActiveThread(null);
      }
      initActiveThreadIds();
    } catch (e) { console.warn('loadThreads failed:', e); setThreads([]); }
  };
  loadThreadsRef.current = loadThreads;
  const loadMessages = async (threadId: string) => { try { const data = await api.get<ChatMessage[]>(`/chat/threads/${threadId}/messages`); setMessages(data || []); } catch (e) { console.warn('loadMessages failed:', e); setMessages([]); } };
  const loadStats = async (threadId: string) => { try { const data = await api.get<ThreadStats>(`/chat/threads/${threadId}/stats`); setThreadStats(data); } catch (e) { console.warn('loadStats failed:', e); setThreadStats(null); } };
  const loadRoles = async () => { try { const data = await api.get<AgentRole[]>('/agent-roles?enabled=true'); setRoles(data || []); } catch (e) { console.warn('loadRoles failed:', e); setRoles([]); } };
  const loadMembers = async (threadId: string) => { try { const data = await threadMembers.list(threadId); setMembers(data || []); } catch (e) { console.warn('loadMembers failed:', e); setMembers([]); } };
  const removeMember = async (slug: string) => {
    if (!activeThread) return;
    try { await threadMembers.remove(activeThread, slug); loadMembers(activeThread); } catch (e) { console.warn('removeMember failed:', e); }
  };
  const handleReaction = async (messageId: string, emoji: string) => {
    try {
      const data = await api.post<{ message_id: string; reactions: { emoji: string; source: string; count: number }[] }>(`/chat/messages/${messageId}/reactions`, { emoji });
      setMessages(prev => prev.map(m => m.id === messageId ? { ...m, reactions: data.reactions } : m));
    } catch (e) { console.warn('handleReaction failed:', e); }
  };

  const loadTodoLists = async () => {
    try {
      const data = await todoApi.summary();
      setTodoLists(data || []);
    } catch { setTodoLists([]); }
  };

  const loadTodoItems = async (listId: string) => {
    try {
      const data = await todoApi.listItems(listId);
      setTodoItems(prev => ({ ...prev, [listId]: data || [] }));
    } catch { /* ignore */ }
  };

  const toggleTodoItem = async (listId: string, itemId: string) => {
    try {
      await todoApi.toggleItem(listId, itemId);
      loadTodoItems(listId);
      loadTodoLists();
    } catch { /* ignore */ }
  };

  const createThread = async () => {
    try {
      const thread = await api.post<ChatThread>('/chat/threads', { agent });
      setThreads(prev => [thread, ...prev]);
      setActiveThread(thread.id);
      setMessages([]);
      setShowThreads(false);
    } catch (err) { toast('error', err instanceof Error ? err.message : 'Failed to create thread'); }
  };

  const stopPolling = () => {
    if (pollingRef.current) { clearInterval(pollingRef.current); pollingRef.current = null; }
  };

  const startPolling = (threadId: string, sentMsgId: string) => {
    stopPolling();
    let ticks = 0;
    const maxTicks = wsConnected ? 360 : 720;
    const interval = wsConnected ? 10000 : 2500;
    pollingRef.current = window.setInterval(async () => {
      ticks++;
      if (ticks > maxTicks) { resetStreaming(); setStreamActive(false); setThinking(false); setWorkStatus(null); setActiveThreadIds(prev => { const next = new Set(prev); next.delete(threadId); return next; }); stopPolling(); return; }
      try {
        const [msgs, status] = await Promise.all([
          api.get<ChatMessage[]>(`/chat/threads/${threadId}/messages`),
          api.get<{ active: boolean; work_order_status?: string; work_order_title?: string }>(`/chat/threads/${threadId}/status`),
        ]);
        const latest = msgs || [];
        setMessages(latest);

        const hasAssistantReply = latest.some(m => m.role === 'assistant' && m.id !== sentMsgId);

        if (status.active) {
          const label = status.work_order_status === 'in_progress'
            ? `Building ${status.work_order_title || ''}...`
            : 'Processing...';
          setWorkStatus(label);
          setThinking(true);
        } else if (hasAssistantReply) {
          resetStreaming();
          setStreamActive(false);
          setSubAgentTasks([]);
          setThinking(false);
          setWorkStatus(null);
          setRoutingIndicator(null);
          setActiveAgentSlug(null);
          setActiveThreadIds(prev => { const next = new Set(prev); next.delete(threadId); return next; });
          stopPolling();
          loadThreads();
        } else {
          setWorkStatus(null);
          setThinking(true);
        }
      } catch (e) { console.warn('polling tick failed:', e); }
    }, interval);
  };

  // On load: check if active thread has ongoing work and resume polling
  useEffect(() => {
    if (!activeThread) return;
    const checkOngoing = async () => {
      try {
        const status = await api.get<{ active: boolean; work_order_status?: string; work_order_title?: string }>(`/chat/threads/${activeThread}/status`);
        if (status.active) {
          setThinking(true);
          const label = status.work_order_status === 'in_progress'
            ? `Building ${status.work_order_title || ''}...`
            : 'Processing...';
          setWorkStatus(label);
          setActiveThreadIds(prev => { const next = new Set(prev); next.add(activeThread); return next; });
          startPolling(activeThread, '');
        }
      } catch (e) { console.warn('checkOngoing failed:', e); }
    };
    checkOngoing();
    return () => stopPolling();
  }, [activeThread]); // eslint-disable-line react-hooks/exhaustive-deps

  const sendMessage = async () => {
    // Block a new turn until the current one fully finishes: `sending` only
    // covers the POST, but the AI keeps streaming after that (thinking /
    // streamActive) until the 'done' event.
    if (!input.trim() || !activeThread || sending || thinking || streamActive) return;
    const typed = input.trim();
    const threadId = activeThread;
    const isFirstMessage = messages.length === 0;

    // Snapshot attachments before clearing the composer state below.
    const ctxFiles = attachedContextFiles;
    const attachFiles = pendingAttachments;
    const dirs = attachedDirectories;
    const toolIds = attachedTools.map(t => t.id);
    const hasAttachments = ctxFiles.length > 0 || attachFiles.length > 0 || dirs.length > 0;

    // Reset streaming state
    resetStreaming();
    setStreamActive(false);
    setSubAgentTasks([]);
    setRoutingIndicator(null);
    hadToolSinceLastTextRef.current = false;
    toolInputMapRef.current.clear();

    // Clear the composer and surface the pending state IMMEDIATELY — reading a
    // large attached context file can take a while and must not look frozen.
    setInput('');
    setAttachedContextFiles([]);
    setPendingAttachments([]);
    setAttachedDirectories([]);
    setAttachedTools([]);
    if (textareaRef.current) { textareaRef.current.style.height = 'auto'; }
    setSending(true); setThinking(true);
    setWorkStatus(hasAttachments ? 'Reading attached files…' : 'Preparing response...');

    // Optimistic user bubble (typed text; replaced by the saved message below).
    const tempId = `temp-${Date.now()}`;
    setMessages(prev => [...prev, { id: tempId, thread_id: threadId, role: 'user', content: typed, agent_role_slug: agent, cost_usd: 0, input_tokens: 0, output_tokens: 0, created_at: new Date().toISOString() }]);

    // Assemble the full message: typed text + attached context/files/directories.
    let content = typed;

    if (ctxFiles.length > 0) {
      const contextParts: string[] = [];
      for (const cf of ctxFiles) {
        try {
          const result = await contextApi.getFile(cf.id);
          if (result.content) {
            contextParts.push(`\n\n---\n**Context: ${cf.name}**\n${result.content}`);
          }
        } catch (e) { console.warn('fetchContextFile failed:', e); }
      }
      if (contextParts.length > 0) content += contextParts.join('');
    }

    if (attachFiles.length > 0) {
      const attachParts: string[] = [];
      for (const file of attachFiles) {
        if (file.type.startsWith('text/') || file.type === 'application/json') {
          try {
            const text = await file.text();
            attachParts.push(`\n\n---\n**Attached: ${file.name}**\n${text}`);
          } catch (e) { console.warn('readAttachmentText failed:', e); }
        } else if (file.type.startsWith('image/')) {
          attachParts.push(`\n\n[Attached image: ${file.name}]`);
        } else {
          attachParts.push(`\n\n[Attached file: ${file.name} (${file.type})]`);
        }
      }
      if (attachParts.length > 0) content += attachParts.join('');
    }

    if (dirs.length > 0) {
      content += dirs.map(d => `\n\n---\n**Directory: ${d}**`).join('');
    }

    // Reflect the assembled content in the optimistic bubble now that reads are done.
    if (content !== typed) {
      setMessages(prev => prev.map(m => m.id === tempId ? { ...m, content } : m));
    }
    setWorkStatus('Preparing response...');

    try {
      // `tools` is omitted entirely for plain text-only sends so the existing
      // request shape is unchanged when nothing is attached.
      const payload: { content: string; agent_role_slug: string; tools?: string[] } =
        { content, agent_role_slug: agent };
      if (toolIds.length > 0) payload.tools = toolIds;
      const saved = await api.post<ChatMessage>(`/chat/threads/${threadId}/messages`, payload);
      setMessages(prev => prev.map(m => m.id === tempId ? saved : m));
      setSending(false);
      startPolling(threadId, saved.id);
      if (isFirstMessage) {
        setTimeout(() => loadThreads(), 5000);
      }
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to send message');
      setSending(false);
      setThinking(false);
    }
  };

  const pinThread = async (thread: ChatThread) => {
    setPinning(true);
    try {
      const result = await threadPins.pin(thread.id);
      setThreads(prev => prev.map(t => t.id === thread.id ? { ...t, pinned: true } : t));
      if (activeThread === thread.id) {
        setActivePin({ pinned: true, pin_summary: result.pin_summary });
      }
      setPinTarget(null);
      toast('success', 'Chat pinned and summarised');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to pin chat');
    } finally {
      setPinning(false);
    }
  };

  const unpinThread = async (thread: ChatThread) => {
    try {
      await threadPins.unpin(thread.id);
      setThreads(prev => prev.map(t => t.id === thread.id ? { ...t, pinned: false } : t));
      if (activeThread === thread.id) setActivePin(null);
      toast('success', 'Chat unpinned — you can continue it now');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to unpin chat');
    }
  };

  const renameThread = async (threadId: string, title: string) => {
    if (!title.trim()) { setEditingThread(null); return; }
    try {
      await api.put(`/chat/threads/${threadId}`, { title: title.trim() });
      setThreads(prev => prev.map(t => t.id === threadId ? { ...t, title: title.trim() } : t));
    } catch (err) { toast('error', err instanceof Error ? err.message : 'Failed to rename thread'); }
    setEditingThread(null);
  };

  const startEditing = (thread: ChatThread) => {
    setEditingThread(thread.id);
    setEditTitle(thread.title);
    setTimeout(() => editInputRef.current?.focus(), 0);
  };

  const deleteThread = async (thread: ChatThread) => {
    try {
      await api.delete(`/chat/threads/${thread.id}`);
      setThreads(prev => prev.filter(t => t.id !== thread.id));
      if (activeThread === thread.id) {
        setActiveThread(null);
        setMessages([]);
      }
      setDeleteTarget(null);
      toast('success', 'Chat deleted');
    } catch (err) { toast('error', err instanceof Error ? err.message : 'Failed to delete thread'); }
  };

  const compactThread = async () => {
    if (!activeThread) return;
    setCompacting(true);
    setShowCompactConfirm(false);
    try {
      await api.post(`/chat/threads/${activeThread}/compact`);
      await loadMessages(activeThread);
      await loadStats(activeThread);
      toast('success', 'Chat compacted');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Compaction failed');
    } finally {
      setCompacting(false);
    }
  };

  const stopThread = async () => {
    if (!activeThread) return;
    try {
      await api.post(`/chat/threads/${activeThread}/stop`);
    } catch (e) { console.warn('stopThread failed:', e); }
  };

  const filteredThreads = threads.filter(t =>
    t?.title?.toLowerCase().includes(search.toLowerCase()) &&
    (threadTab === 'pinned' ? !!t.pinned : !t.pinned)
  );
  const totalPages = Math.max(1, Math.ceil(filteredThreads.length / THREADS_PER_PAGE));
  const clampedPage = Math.min(threadPage, totalPages);
  const pagedThreads = filteredThreads.slice((clampedPage - 1) * THREADS_PER_PAGE, clampedPage * THREADS_PER_PAGE);
  const activeRole = roles.find(r => r.slug === agent);
  const thinkingRole = activeAgentSlug ? roles.find(r => r.slug === activeAgentSlug) : activeRole;
  const isStreaming = streamActive || streamingText.length > 0 || streamingTools.length > 0 || subAgentTasks.length > 0;
  // Provider-aware usage bar: CLI subscription providers (Claude Code / Codex) have
  // no per-token cost and a 1M context window; OpenRouter uses the model's window.
  const isSubscription = balance.subscription;
  const ctxLimit = isSubscription ? CLI_CONTEXT_LIMIT : (threadStats?.context_limit_tokens ?? 0);
  const ctxUsed = threadStats?.context_used_tokens ?? 0;
  const ctxPct = ctxLimit > 0 ? ctxUsed / ctxLimit : 0;

  return (
    <div className="flex flex-col h-full">
      <Header title="Chat"
        actions={
          <>
            {/* Desktop pane visibility is driven by the header's view-options
                menu. The buttons below are the mobile-only drawer controls. */}
            <button onClick={() => setShowThreads(!showThreads)} className="md:hidden p-2 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer" aria-label={showThreads ? 'Hide chat threads' : 'Show chat threads'}>
              {showThreads ? <PanelLeftClose className="w-5 h-5" aria-hidden="true" /> : <PanelLeftOpen className="w-5 h-5" aria-hidden="true" />}
            </button>
            {activeThread && (
              <button onClick={() => setShowRightPanel(!showRightPanel)} className="md:hidden p-2 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer" aria-label={showRightPanel ? 'Hide panel' : 'Show panel'}>
                {showRightPanel ? <PanelRightClose className="w-5 h-5" aria-hidden="true" /> : <PanelRightOpen className="w-5 h-5" aria-hidden="true" />}
              </button>
            )}
          </>
        }
      />
      <div className="flex flex-1 overflow-hidden relative">
        <div className={`${showThreads ? 'translate-x-0' : '-translate-x-full md:translate-x-0'} ${threadsCollapsed ? 'md:w-0 md:border-r-0' : 'md:w-72 md:border-r'} absolute md:relative z-30 w-[85vw] max-w-72 h-full flex flex-col overflow-hidden border-r border-border-0 bg-surface-1 transition-all duration-200`}>
          <div className="p-3 space-y-2">
            <Button onClick={createThread} icon={<Plus className="w-4 h-4" />} className="w-full" size="sm">New Chat</Button>
            <SearchBar value={search} onChange={(v) => { setSearch(v); setThreadPage(1); }} placeholder="Search chats..." />
            <div className="flex items-center gap-1 rounded-lg bg-surface-2 p-0.5" role="tablist">
              {([['all', 'All Chats'], ['pinned', 'Pinned']] as const).map(([key, label]) => (
                <button
                  key={key}
                  role="tab"
                  aria-selected={threadTab === key}
                  onClick={() => { setThreadTab(key); setThreadPage(1); }}
                  className={`flex-1 px-2 py-1 rounded-md text-xs font-medium transition-colors cursor-pointer ${
                    threadTab === key ? 'bg-surface-0 text-text-0 shadow-sm' : 'text-text-3 hover:text-text-1'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div className="flex-1 overflow-y-auto px-2 pb-2 space-y-0.5">
            {pagedThreads.length === 0 ? (
              <div className="text-center py-8 text-sm text-text-3">No chats yet</div>
            ) : pagedThreads.map(thread => (
              <div key={thread.id} aria-current={activeThread === thread.id ? 'true' : undefined} className={`group relative rounded-lg transition-colors ${activeThread === thread.id ? 'bg-accent-muted text-accent-text' : unreadThreadIds.has(thread.id) && !activeThreadIds.has(thread.id) ? 'bg-accent-primary/5 text-text-0 hover:bg-accent-primary/10' : 'text-text-1 hover:bg-surface-2'}`}>
                {editingThread === thread.id ? (
                  <div className="flex items-center gap-1 px-2 py-2">
                    <input
                      ref={editInputRef}
                      value={editTitle}
                      onChange={e => setEditTitle(e.target.value)}
                      onKeyDown={e => {
                        if (e.key === 'Enter') renameThread(thread.id, editTitle);
                        if (e.key === 'Escape') setEditingThread(null);
                      }}
                      className="flex-1 min-w-0 px-2 py-1 rounded text-sm bg-surface-2 border border-border-1 text-text-0 focus:outline-none focus:ring-1 focus:ring-accent-primary"
                    />
                    <button onClick={() => renameThread(thread.id, editTitle)} className="p-1 rounded text-emerald-400 hover:bg-emerald-500/10 cursor-pointer" aria-label="Confirm rename"><Check className="w-3.5 h-3.5" aria-hidden="true" /></button>
                    <button onClick={() => setEditingThread(null)} className="p-1 rounded text-text-3 hover:bg-surface-3 cursor-pointer" aria-label="Cancel rename"><X className="w-3.5 h-3.5" aria-hidden="true" /></button>
                  </div>
                ) : (
                  <>
                    <button onClick={() => { setActiveThread(thread.id); setShowThreads(false); }}
                      className="w-full text-left px-3 py-2.5 cursor-pointer pr-16">
                      <div className="flex items-center gap-2">
                        {activeThreadIds.has(thread.id) ? (
                          <Loader2 className="w-4 h-4 flex-shrink-0 text-accent-primary animate-spin" />
                        ) : unreadThreadIds.has(thread.id) && activeThread !== thread.id ? (
                          <span className="w-2 h-2 flex-shrink-0 rounded-full bg-accent-primary" />
                        ) : null}
                        <span className="text-sm font-medium truncate">{thread.title}</span>
                      </div>
                      <p className="text-xs text-text-3 mt-0.5">
                        {activeThreadIds.has(thread.id)
                          ? <span className="text-accent-primary">Working...</span>
                          : unreadThreadIds.has(thread.id) && activeThread !== thread.id
                          ? <span className="text-accent-primary/70">New messages</span>
                          : <>
                              {timeAgo(thread.updated_at)}
                              {thread.total_cost_usd > 0 && <span className="ml-2 text-text-3/70">${thread.total_cost_usd < 0.01 ? thread.total_cost_usd.toFixed(4) : thread.total_cost_usd.toFixed(2)}</span>}
                            </>
                        }
                      </p>
                    </button>
                    <div className="absolute right-1.5 top-1/2 -translate-y-1/2 flex items-center gap-0.5 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-all">
                      <button
                        onClick={(e) => { e.stopPropagation(); startEditing(thread); }}
                        className="p-1.5 rounded-md text-text-3 hover:text-text-1 hover:bg-surface-3 cursor-pointer focus:opacity-100"
                        aria-label="Rename thread"
                      >
                        <Pencil className="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                      <button
                        onClick={(e) => { e.stopPropagation(); thread.pinned ? unpinThread(thread) : setPinTarget(thread); }}
                        className={`p-1.5 rounded-md cursor-pointer focus:opacity-100 ${thread.pinned ? 'text-accent-primary hover:bg-accent-muted' : 'text-text-3 hover:text-text-1 hover:bg-surface-3'}`}
                        aria-label={thread.pinned ? 'Unpin chat' : 'Pin chat'}
                        title={thread.pinned ? 'Unpin — makes this chat editable again' : 'Pin — archive this chat with a summary'}
                      >
                        <Pin className={`w-3.5 h-3.5 ${thread.pinned ? 'fill-current' : ''}`} aria-hidden="true" />
                      </button>
                      <button
                        onClick={(e) => { e.stopPropagation(); setDeleteTarget(thread); }}
                        className="p-1.5 rounded-md text-text-3 hover:text-red-400 hover:bg-red-500/10 cursor-pointer focus:opacity-100"
                        aria-label="Delete thread"
                      >
                        <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                    </div>
                  </>
                )}
              </div>
            ))}
          </div>
          <div className="flex items-center justify-between px-4 py-2 border-t border-border-0">
            <button
              onClick={() => setThreadPage(p => Math.max(1, p - 1))}
              disabled={clampedPage <= 1}
              className="p-1 rounded-md text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
              aria-label="Previous page"
            >
              <ChevronLeft className="w-4 h-4" aria-hidden="true" />
            </button>
            <span className="text-[11px] text-text-3 tabular-nums">
              {clampedPage} / {totalPages}
            </span>
            <button
              onClick={() => setThreadPage(p => Math.min(totalPages, p + 1))}
              disabled={clampedPage >= totalPages}
              className="p-1 rounded-md text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
              aria-label="Next page"
            >
              <ChevronRight className="w-4 h-4" aria-hidden="true" />
            </button>
          </div>
        </div>
        {showThreads && <div className="md:hidden fixed inset-0 bg-black/40 z-20" onClick={() => setShowThreads(false)} />}
        <div className="flex-1 flex flex-col overflow-hidden relative">
          {activeThread ? (
            <>
              {threadStats && threadStats.message_count > 0 && (
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 md:px-4 py-1.5 border-b border-border-0 bg-surface-1/80 text-[11px] text-text-3">
                  {!isSubscription && (
                    <span className="flex items-center gap-1"><Coins className="w-3 h-3" aria-hidden="true" />${threadStats.total_cost_usd.toFixed(4)}</span>
                  )}
                  <span className="flex items-center gap-1"><Zap className="w-3 h-3" aria-hidden="true" />{((threadStats.total_input_tokens || 0) + (threadStats.total_output_tokens || 0)).toLocaleString()}</span>
                  {ctxLimit > 0 && (
                  <div className="flex items-center gap-1.5">
                    <div
                      className="w-16 md:w-28 h-1.5 rounded-full bg-surface-3 overflow-hidden"
                      role="progressbar"
                      aria-valuenow={Math.round(ctxPct * 100)}
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-label="Context window usage"
                    >
                      <div
                        className={`h-full rounded-full transition-all ${
                          ctxPct > 0.8
                            ? 'bg-red-400'
                            : ctxPct > 0.5
                              ? 'bg-amber-400'
                              : 'bg-emerald-400'
                        }`}
                        style={{ width: `${Math.min(100, ctxPct * 100)}%` }}
                      />
                    </div>
                    <span className="whitespace-nowrap">{Math.round(ctxPct * 100)}%<span className="hidden sm:inline"> of {ctxLimit >= 1000000 ? `${(ctxLimit / 1000000).toFixed(ctxLimit % 1000000 === 0 ? 0 : 1)}M` : `${Math.round(ctxLimit / 1000)}k`}</span></span>
                  </div>
                  )}
                  <div className="flex items-center gap-1 ml-auto">
                    <button
                      onClick={() => setShowCompactConfirm(true)}
                      disabled={compacting}
                      className="flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-50"
                      title="Compact chat"
                      aria-label="Compact chat"
                    >
                      {compacting ? <Loader2 className="w-3 h-3 animate-spin" aria-hidden="true" /> : <Minimize2 className="w-3 h-3" aria-hidden="true" />}
                      <span className="hidden sm:inline">Compact</span>
                    </button>
                  </div>
                </div>
              )}
              {threadStats && ctxLimit > 0 && ctxPct >= 0.8 && (
                <div className={`flex items-center gap-2 px-3 md:px-4 py-2 border-b text-xs ${
                  ctxPct >= 1
                    ? 'bg-red-500/10 border-red-500/30 text-red-400'
                    : 'bg-amber-500/10 border-amber-500/30 text-amber-400'
                }`}>
                  <AlertTriangle className="w-3.5 h-3.5 shrink-0" />
                  <span>
                    {ctxPct >= 1
                      ? 'Context window is full. The AI may stop responding. Compact this chat to continue.'
                      : 'Context window is nearly full. Consider compacting this chat to free up space.'}
                  </span>
                  <button
                    onClick={() => setShowCompactConfirm(true)}
                    disabled={compacting}
                    className="ml-auto shrink-0 px-2 py-0.5 rounded text-[10px] font-semibold bg-current/10 hover:bg-current/20 transition-colors cursor-pointer disabled:opacity-50"
                  >
                    {compacting ? 'Compacting...' : 'Compact now'}
                  </button>
                </div>
              )}
              <div className="flex-1 overflow-y-auto p-4 pb-[180px]">
                <div className="max-w-[960px] mx-auto space-y-4">
                {messages.length === 0 && !isStreaming && !thinking && (
                  <div className="flex flex-col items-center justify-center min-h-[calc(100vh-320px)] text-center">
                    <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-accent-primary/20 to-accent-primary/5 flex items-center justify-center mb-5 ring-1 ring-accent-primary/20">
                      <MessageSquare className="w-7 h-7 text-accent-primary" />
                    </div>
                    <h2 className="text-xl font-bold text-text-0 mb-1.5">Start a conversation</h2>
                    <p className="text-sm text-text-3 max-w-sm mb-8">
                      Ask anything, mention agents with <span className="text-text-1 font-medium">@</span>, attach files with <span className="text-text-1 font-medium">!!</span>, or reference media with <span className="text-text-1 font-medium">@@</span>
                    </p>
                    {userRoles.length > 0 ? (
                      <div className="grid grid-cols-3 gap-2 max-w-xs w-full">
                        {(userRoles.length > 6 ? userRoles.slice(0, 5) : userRoles.slice(0, 6)).map(role => (
                          <button
                            key={role.slug}
                            onClick={() => {
                              setInput(prev => `@${role.name} ${prev}`);
                              textareaRef.current?.focus();
                            }}
                            className="group flex flex-col items-center gap-2 p-3 rounded-xl bg-surface-2/60 hover:bg-surface-2 border border-transparent hover:border-border-1 transition-all cursor-pointer"
                          >
                            <div className="w-10 h-10 rounded-md overflow-hidden ring-1 ring-border-1 group-hover:ring-accent-primary/40 transition-all">
                              <img src={role.avatar_path} alt={role.name} className="w-10 h-10 rounded-md object-cover" />
                            </div>
                            <span className="text-xs font-medium text-text-2 group-hover:text-text-0 truncate w-full transition-colors">{role.name}</span>
                          </button>
                        ))}
                        {userRoles.length > 6 && (
                          <button
                            onClick={() => {
                              setInput('@');
                              textareaRef.current?.focus();
                              setMentionOpen(true);
                              setMentionFilter('');
                              mentionAnchorRef.current = 0;
                            }}
                            className="group flex flex-col items-center gap-2 p-3 rounded-xl bg-surface-2/60 hover:bg-surface-2 border border-transparent hover:border-border-1 transition-all cursor-pointer"
                          >
                            <div className="w-10 h-10 rounded-md bg-surface-3 flex items-center justify-center ring-1 ring-border-1 group-hover:ring-accent-primary/40 transition-all">
                              <Plus className="w-5 h-5 text-text-3 group-hover:text-accent-primary transition-colors" />
                            </div>
                            <span className="text-xs font-medium text-text-3 group-hover:text-text-1 transition-colors">+{userRoles.length - 5} more</span>
                          </button>
                        )}
                      </div>
                    ) : (
                      <p className="text-xs text-text-3">Add your agents to show up here</p>
                    )}
                  </div>
                )}
                {activePin?.pinned && (
                  <div className="mb-6">
                    <div className="rounded-2xl border-2 border-accent-primary bg-surface-0/95 overflow-hidden">
                      <div className="flex items-center gap-2 px-4 py-2 border-b-2 border-accent-primary/40 bg-black/30">
                        <Pin className="w-3.5 h-3.5 text-accent-primary fill-current flex-shrink-0" aria-hidden="true" />
                        <span className="text-xs font-semibold uppercase tracking-wider text-accent-primary">Pinned Chat</span>
                        <span className="text-[10px] text-text-3 ml-auto">Read-only archive</span>
                      </div>
                      <div className="px-6 py-6 md:px-8 md:py-7 text-base font-medium text-text-1">
                        {activePin.pin_summary ? (
                          <div className="prose-chat">
                            <ReactMarkdown remarkPlugins={[remarkGfm]}>{activePin.pin_summary}</ReactMarkdown>
                          </div>
                        ) : (
                          <p className="text-sm text-text-3">
                            No summary was generated for this chat. The full conversation is below.
                          </p>
                        )}
                      </div>
                    </div>
                    <button
                      onClick={() => setTranscriptOpen(o => !o)}
                      aria-expanded={transcriptOpen}
                      className="mt-3 flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                    >
                      <ChevronDown className={`w-3.5 h-3.5 transition-transform ${transcriptOpen ? 'rotate-180' : ''}`} aria-hidden="true" />
                      {transcriptOpen ? 'Hide full conversation' : `Show full conversation (${messages.length} messages)`}
                    </button>
                  </div>
                )}
                {(!activePin?.pinned || transcriptOpen) &&
                  messages.map(msg => <MessageBubble key={msg.id} message={msg} roles={roles} onRefresh={() => activeThread && loadMessages(activeThread)} userAvatarPath={user?.avatar_path} onReact={handleReaction} />)}
                {isStreaming && (
                  <StreamingMessage
                    text={streamingText}
                    tools={streamingTools}
                    cost={costInfo}
                    role={thinkingRole || null}
                    roles={roles}
                    widgets={streamingWidgets}
                    subAgentTasks={subAgentTasks}
                  />
                )}
                {routingIndicator && !thinking && !isStreaming && (
                  <div className="flex items-center gap-2 px-3 py-1.5">
                    <Loader2 className="w-3.5 h-3.5 text-accent-primary animate-spin" />
                    <span className="text-xs text-text-2 font-medium">{routingIndicator}</span>
                  </div>
                )}
                {thinking && !isStreaming && (
                  <div className="flex gap-3">
                    <div className="max-w-[85%] md:max-w-[75%]">
                      <div className="rounded-2xl rounded-bl-md px-4 py-3 bg-surface-2">
                        <button
                          onClick={() => thinkingText && setThinkingExpanded(!thinkingExpanded)}
                          className="flex items-center gap-2 w-full cursor-pointer"
                        >
                          <Loader2 className="w-4 h-4 text-accent-primary animate-spin flex-shrink-0" />
                          <span className="text-sm text-text-2 flex-1 text-left">{workStatus || 'Thinking...'}</span>
                          {thinkingText && (
                            <ChevronDown className={`w-3.5 h-3.5 text-text-3 transition-transform ${thinkingExpanded ? 'rotate-180' : ''}`} />
                          )}
                        </button>
                      </div>
                      {thinkingText && thinkingExpanded && (
                        <div className="mt-1 rounded-xl bg-surface-2/70 border border-border-1 px-4 py-3 max-h-64 overflow-y-auto">
                          <pre className="text-xs text-text-2 whitespace-pre-wrap font-mono leading-relaxed">{thinkingText}</pre>
                        </div>
                      )}
                    </div>
                  </div>
                )}
                <div ref={messagesEndRef} />
                </div>
              </div>
              {activePin?.pinned ? (
                <div className="absolute bottom-0 left-0 right-0 z-10 p-4 border-t border-white/[0.06] bg-black/40 backdrop-blur-xl">
                  <div className="max-w-[960px] mx-auto flex items-center gap-3">
                    <Pin className="w-4 h-4 text-accent-primary fill-current flex-shrink-0" aria-hidden="true" />
                    <p className="flex-1 text-sm text-text-2">
                      This chat is pinned and read-only.
                    </p>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => {
                        const t = threads.find(x => x.id === activeThread);
                        if (t) unpinThread(t);
                      }}
                    >
                      Unpin to continue
                    </Button>
                  </div>
                </div>
              ) : (
              <div className="absolute bottom-0 left-0 right-0 z-10 p-3 md:p-4 border-t border-white/[0.06] bg-black/40 backdrop-blur-xl">
                <div className="max-w-[960px] mx-auto relative">
                  {/* Tools # autocomplete dropdown */}
                  {/* Shown when there are matches, or when the workspace genuinely
                      has no tools. A filter that matches nothing just closes,
                      so typing "#5" in prose isn't interrupted. */}
                  {toolPickerVisible && (
                    <div className="absolute bottom-full left-0 right-0 mb-1 rounded-xl border border-border-1 bg-surface-1 shadow-xl shadow-black/20 overflow-hidden z-50 max-h-64 overflow-y-auto" role="listbox" aria-label="Attach tools">
                      {flatTools.length === 0 ? (
                        <div className="px-4 py-3 text-sm text-text-3">
                          No tools available. Add tools in Settings.
                        </div>
                      ) : (
                        <>
                          {workspaceTools.length > 0 && (
                            <div className="px-4 py-1.5 text-[11px] font-semibold text-text-3 uppercase tracking-wider border-b border-border-0">
                              Workspace Tools
                            </div>
                          )}
                          {workspaceTools.map((tool, i) => (
                            <ToolPickerRow
                              key={tool.id}
                              tool={tool}
                              selected={i === toolIndex}
                              onSelect={() => attachTool(tool)}
                            />
                          ))}
                          {globalTools.length > 0 && (
                            <div className="px-4 py-1.5 text-[11px] font-semibold text-text-3 uppercase tracking-wider border-b border-border-0 border-t">
                              Global Tools
                            </div>
                          )}
                          {globalTools.map((tool, i) => (
                            <ToolPickerRow
                              key={tool.id}
                              tool={tool}
                              selected={workspaceTools.length + i === toolIndex}
                              onSelect={() => attachTool(tool)}
                            />
                          ))}
                        </>
                      )}
                    </div>
                  )}
                  {/* Media @@ autocomplete dropdown */}
                  {mediaOpen && filteredMediaItems.length > 0 && (
                    <div className="absolute bottom-full left-0 right-0 mb-1 rounded-xl border border-border-1 bg-surface-1 shadow-xl shadow-black/20 overflow-hidden z-50 max-h-64 overflow-y-auto" role="listbox" aria-label="Media library">
                      <div className="px-4 py-1.5 text-[11px] font-semibold text-text-3 uppercase tracking-wider border-b border-border-0">
                        Media Library
                      </div>
                      {filteredMediaItems.map((item, i) => (
                        <button
                          key={item.id}
                          role="option"
                          aria-selected={i === mediaIndex}
                          onClick={() => insertMediaRef(item)}
                          className={`w-full flex items-center gap-3 px-4 py-2 text-left transition-colors cursor-pointer ${
                            i === mediaIndex ? 'bg-accent-muted' : 'hover:bg-surface-2'
                          }`}
                        >
                          <div className="w-10 h-10 rounded-md overflow-hidden flex-shrink-0 bg-surface-2">
                            <img
                              src={mediaApi.fileUrl(item.id)}
                              alt=""
                              className="w-full h-full object-cover"
                              loading="lazy"
                            />
                          </div>
                          <div className="min-w-0 flex-1">
                            <p className="text-sm font-medium text-text-0 truncate">
                              {item.prompt || item.filename || 'Untitled'}
                            </p>
                            <p className="text-xs text-text-3 truncate">
                              {item.source_model} &middot; {item.width}x{item.height}
                            </p>
                          </div>
                          <ImageIcon className="w-4 h-4 text-text-3 flex-shrink-0" />
                        </button>
                      ))}
                    </div>
                  )}
                  {/* Context file/folder !! autocomplete dropdown */}
                  {contextOpen && filteredContextItems.length > 0 && (
                    <div className="absolute bottom-full left-0 right-0 mb-1 rounded-xl border border-border-1 bg-surface-1 shadow-xl shadow-black/20 overflow-hidden z-50 max-h-64 overflow-y-auto" role="listbox" aria-label="Context files">
                      <div className="px-4 py-1.5 text-[11px] font-semibold text-text-3 uppercase tracking-wider border-b border-border-0">
                        Context
                      </div>
                      {filteredContextItems.map((item, i) => {
                        const key = item.kind === 'file' ? `f-${item.file.id}` : `d-${item.folder.id}`;
                        const name = item.kind === 'file' ? item.file.name : item.folder.name;
                        const sub = item.kind === 'file' ? item.file.mime_type : `${item.files.length} file${item.files.length !== 1 ? 's' : ''}`;
                        const isNested = item.kind === 'file' && item.file.folder_id;
                        return (
                          <button
                            key={key}
                            role="option"
                            aria-selected={i === contextIndex}
                            onClick={() => insertContextItem(item)}
                            className={`w-full flex items-center justify-between gap-3 px-4 py-2 text-left transition-colors cursor-pointer ${
                              i === contextIndex ? 'bg-accent-muted' : 'hover:bg-surface-2'
                            } ${isNested ? 'pl-8' : ''}`}
                          >
                            <div className="flex items-center gap-2 min-w-0">
                              {item.kind === 'folder'
                                ? <FolderOpen className="w-4 h-4 text-accent-primary flex-shrink-0" />
                                : <FileText className="w-4 h-4 text-text-3 flex-shrink-0" />
                              }
                              <span className="text-sm font-medium text-text-0 truncate">{name}{item.kind === 'folder' ? '/' : ''}</span>
                            </div>
                            <span className="text-xs text-text-3 flex-shrink-0">{sub}</span>
                          </button>
                        );
                      })}
                    </div>
                  )}
                  {/* Mention @ autocomplete dropdown */}
                  {mentionOpen && filteredMentionRoles.length > 0 && (
                    <div className="absolute bottom-full left-0 right-0 mb-1 rounded-xl border border-border-1 bg-surface-1 shadow-xl shadow-black/20 overflow-hidden z-50 max-h-64 overflow-y-auto" role="listbox" aria-label="Mention agents">
                      {filteredMentionRoles.map((role, i) => (
                        <button
                          key={role.slug}
                          role="option"
                          aria-selected={i === mentionIndex}
                          onClick={() => insertMention(role)}
                          className={`w-full flex items-center gap-3 px-4 py-2.5 text-left transition-colors cursor-pointer ${
                            i === mentionIndex ? 'bg-accent-muted' : 'hover:bg-surface-2'
                          }`}
                        >
                          <img src={role.avatar_path} alt={role.name} className="w-7 h-7 rounded-md flex-shrink-0" />
                          <div className="min-w-0">
                            <p className="text-sm font-medium text-text-0 truncate">@{role.name}</p>
                            {role.description && (
                              <p className="text-xs text-text-3 truncate">{role.description}</p>
                            )}
                          </div>
                        </button>
                      ))}
                    </div>
                  )}
                  <div
                    className="rounded-2xl border border-border-1 bg-surface-1 shadow-lg shadow-black/10 overflow-hidden focus-within:border-text-3 transition-colors"
                    onDragOver={(e) => e.preventDefault()}
                    onDrop={(e) => {
                      e.preventDefault();
                      if (e.dataTransfer.files.length > 0) {
                        setPendingAttachments(prev => [...prev, ...Array.from(e.dataTransfer.files)]);
                      }
                    }}
                  >
                  {/* Pending attachments / context files strip */}
                  {(pendingAttachments.length > 0 || attachedContextFiles.length > 0 || attachedDirectories.length > 0 || attachedTools.length > 0) && (
                    <div className="flex flex-wrap gap-1.5 px-3 pt-2.5">
                      {attachedTools.map(tool => (
                        <span key={`tool-${tool.id}`} className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-accent-muted text-accent-text text-xs font-medium">
                          <Wrench className="w-3 h-3" />
                          {tool.name}
                          <button
                            onClick={() => setAttachedTools(prev => prev.filter(t => t.id !== tool.id))}
                            className="ml-0.5 hover:text-red-400 cursor-pointer"
                            aria-label={`Remove ${tool.name}`}
                            title={`Remove ${tool.name}`}
                          >
                            <X className="w-3 h-3" />
                          </button>
                        </span>
                      ))}
                      {attachedContextFiles.map(cf => (
                        <span key={`ctx-${cf.id}`} className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-accent-muted text-accent-text text-xs font-medium">
                          <FileText className="w-3 h-3" />
                          {cf.name}
                          <button
                            onClick={() => setAttachedContextFiles(prev => prev.filter(f => f.id !== cf.id))}
                            className="ml-0.5 hover:text-red-400 cursor-pointer"
                          >
                            <X className="w-3 h-3" />
                          </button>
                        </span>
                      ))}
                      {attachedDirectories.map((dir, i) => (
                        <span key={`dir-${i}`} className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-surface-3 text-text-2 text-xs font-medium">
                          <FolderOpen className="w-3 h-3" />
                          {dir.split('/').filter(Boolean).pop() || dir}
                          <button
                            onClick={() => setAttachedDirectories(prev => prev.filter((_, j) => j !== i))}
                            className="ml-0.5 hover:text-red-400 cursor-pointer"
                          >
                            <X className="w-3 h-3" />
                          </button>
                        </span>
                      ))}
                      {pendingAttachments.map((file, i) => (
                        <span key={`att-${i}`} className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-surface-3 text-text-2 text-xs font-medium">
                          <Paperclip className="w-3 h-3" />
                          {file.name}
                          <button
                            onClick={() => setPendingAttachments(prev => prev.filter((_, j) => j !== i))}
                            className="ml-0.5 hover:text-red-400 cursor-pointer"
                          >
                            <X className="w-3 h-3" />
                          </button>
                        </span>
                      ))}
                    </div>
                  )}
                  <textarea
                    ref={textareaRef}
                    value={input}
                    onChange={handleInputChange}
                    onKeyDown={handleInputKeyDown}
                    placeholder="Ask anything... (@ agents, # tools, !! context, @@ media)"
                    aria-label="Type a message"
                    aria-keyshortcuts="Enter"
                    disabled={sending}
                    rows={2}
                    className="w-full resize-none bg-transparent text-text-0 text-base placeholder:text-text-3 px-4 pt-3 pb-1 focus:outline-none focus:ring-0 focus:border-transparent border-none shadow-none min-h-[56px]"
                    style={{ maxHeight: '350px' }}
                  />
                  <div className="flex items-center justify-between px-3 pb-2.5 pt-1">
                    <div className="flex items-center gap-1">
                      <label className="p-1.5 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer" title="Attach file" aria-label="Attach file">
                        <Paperclip className="w-4 h-4" aria-hidden="true" />
                        <input
                          ref={attachInputRef}
                          type="file"
                          multiple
                          className="hidden"
                          onChange={(e) => {
                            if (e.target.files) {
                              setPendingAttachments(prev => [...prev, ...Array.from(e.target.files!)]);
                            }
                            e.target.value = '';
                          }}
                        />
                      </label>
                      <button
                        className="p-1.5 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                        title="Attach directory"
                        aria-label="Attach directory"
                        onClick={async () => {
                          try {
                            const result = await api.post<{ path: string }>('/system/pick-folder', {});
                            if (result.path && !attachedDirectories.includes(result.path)) {
                              setAttachedDirectories(prev => [...prev, result.path]);
                            }
                          } catch {
                            // dialog cancelled or failed — ignore
                          }
                        }}
                      >
                        <FolderPlus className="w-4 h-4" aria-hidden="true" />
                      </button>
                    </div>
                    {(thinking || isStreaming) ? (
                      <button
                        onClick={stopThread}
                        className="flex items-center justify-center w-8 h-8 rounded-full bg-surface-3 text-text-1 transition-all cursor-pointer flex-shrink-0 hover:bg-danger hover:text-white"
                        title="Stop"
                        aria-label="Stop generation"
                      >
                        <Square className="w-3.5 h-3.5 fill-current" aria-hidden="true" />
                      </button>
                    ) : (
                      <button
                        onClick={sendMessage}
                        disabled={!input.trim() || sending}
                        className="flex items-center justify-center w-8 h-8 rounded-full bg-accent-primary text-white disabled:opacity-30 disabled:cursor-not-allowed transition-all cursor-pointer flex-shrink-0 hover:bg-accent-hover"
                        aria-label="Send message"
                      >
                        {sending ? <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" /> : <ArrowUp className="w-4 h-4" aria-hidden="true" />}
                      </button>
                    )}
                  </div>
                  </div>
                </div>
              </div>
              )}
            </>
          ) : (
            <div className="flex-1 flex flex-col items-center justify-center text-center p-4 md:p-8">
              <div className="w-16 h-16 rounded-2xl bg-surface-2 flex items-center justify-center mb-4"><MessageSquare className="w-8 h-8 text-text-3" /></div>
              <h2 className="text-lg font-semibold text-text-1 mb-1">No chat selected</h2>
              <p className="text-sm text-text-2 mb-4 max-w-xs">Create a new chat or select an existing one to start talking with your AI agents.</p>
              <Button onClick={createThread} icon={<Plus className="w-4 h-4" />}>New Chat</Button>
            </div>
          )}
        </div>
        {activeThread && showRightPanel && (
          <>
            {!rightPanelCollapsed && <div className="md:hidden fixed inset-0 bg-black/40 z-20" onClick={() => setShowRightPanel(false)} />}
            <div className={`absolute md:relative right-0 z-30 h-full flex flex-col border-l border-border-0 bg-surface-1 transition-all duration-200 ${
              rightPanelCollapsed ? 'w-10' : 'w-[85vw] max-w-72 md:w-56'
            }`}>
              {rightPanelCollapsed ? (
                <div className="flex flex-col items-center py-3 gap-2">
                  <button
                    onClick={() => setRightPanelCollapsed(false)}
                    className="p-1.5 rounded text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                    aria-label="Expand panel"
                  >
                    <ChevronLeft className="w-4 h-4" />
                  </button>
                  <span className="text-[10px] font-semibold text-text-3 uppercase tracking-wider [writing-mode:vertical-rl] rotate-180 select-none">Panel</span>
                </div>
              ) : (
                <>
                  <div className="p-2 border-b border-border-0 flex items-center justify-between">
                    <span className="text-[10px] font-semibold uppercase tracking-wider text-text-3">Chat Panel</span>
                    <div className="flex items-center gap-1">
                      <button onClick={() => setRightPanelCollapsed(true)} className="hidden md:block p-1 rounded text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer" aria-label="Collapse panel">
                        <ChevronRight className="w-4 h-4" />
                      </button>
                      <button onClick={() => setShowRightPanel(false)} className="md:hidden p-1 rounded text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer">
                        <X className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                  <div className="flex-1 overflow-y-auto">
                    {/* Agents in Chat */}
                    <div className="border-b border-border-0">
                      <button
                        onClick={() => setSectionAgents(p => !p)}
                        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-surface-2 transition-colors cursor-pointer"
                      >
                        {sectionAgents ? <ChevronDown className="w-3 h-3 text-text-3" /> : <ChevronRight className="w-3 h-3 text-text-3" />}
                        <Users className="w-3.5 h-3.5 text-text-3" />
                        <span className="text-[11px] font-semibold uppercase tracking-wider text-text-2 flex-1">Agents in Chat</span>
                        {members.length > 0 && <span className="text-[10px] text-text-3">{members.length}</span>}
                      </button>
                      {sectionAgents && (
                        <div className="px-2 pb-2 space-y-0.5">
                          {members.length === 0 ? (
                            <p className="text-[11px] text-text-3 px-2 py-1">No agents yet</p>
                          ) : members.map(m => (
                            <div key={m.agent_role_slug} className="group flex items-center gap-2 px-2 py-1 rounded-lg hover:bg-surface-2 transition-colors">
                              <button
                                onClick={() => chatNavigate(m.agent_role_slug === 'builder' ? '/agents/gateway' : `/agents/${m.agent_role_slug}`)}
                                className="flex items-center gap-2 flex-1 min-w-0 cursor-pointer text-left"
                                title={`Edit ${m.name}`}
                              >
                                <div className="relative flex-shrink-0">
                                  {m.avatar_path ? (
                                    <img src={m.avatar_path} alt={m.name} className="w-6 h-6 rounded-md object-cover ring-1 ring-border-1" />
                                  ) : (
                                    <div className="w-6 h-6 rounded-md bg-surface-3 flex items-center justify-center ring-1 ring-border-1">
                                      <Bot className="w-3 h-3 text-text-3" />
                                    </div>
                                  )}
                                  {activeAgentSlug === m.agent_role_slug && (
                                    <div className="absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full bg-emerald-400 ring-2 ring-surface-1" />
                                  )}
                                </div>
                                <p className="text-[11px] font-medium text-text-1 truncate flex-1">{m.name}</p>
                              </button>
                              <button
                                onClick={() => removeMember(m.agent_role_slug)}
                                className="p-0.5 rounded text-text-3 hover:text-red-400 hover:bg-red-500/10 opacity-0 group-hover:opacity-100 transition-all cursor-pointer"
                                title="Remove from chat"
                              >
                                <X className="w-3 h-3" />
                              </button>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>

                    {/* Context */}
                    <div className="border-b border-border-0">
                      <button
                        onClick={() => setSectionContext(p => !p)}
                        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-surface-2 transition-colors cursor-pointer"
                      >
                        {sectionContext ? <ChevronDown className="w-3 h-3 text-text-3" /> : <ChevronRight className="w-3 h-3 text-text-3" />}
                        <FolderOpen className="w-3.5 h-3.5 text-text-3" />
                        <span className="text-[11px] font-semibold uppercase tracking-wider text-text-2 flex-1">Context</span>
                        {filteredContextItems.filter(i => i.kind === 'file').length > 0 && (
                          <span className="text-[10px] text-text-3">{filteredContextItems.filter(i => i.kind === 'file').length}</span>
                        )}
                      </button>
                      {sectionContext && (
                        <div className="px-2 pb-2 space-y-0.5">
                          {filteredContextItems.length === 0 ? (
                            <p className="text-[11px] text-text-3 px-2 py-1">No context files</p>
                          ) : (
                            filteredContextItems.map((item, i) => {
                              if (item.kind === 'folder') {
                                return (
                                  <div key={`folder-${i}`} className="flex items-center gap-1.5 px-2 py-1 text-[10px] font-semibold text-text-2 uppercase tracking-wider">
                                    <FolderOpen className="w-3 h-3" />
                                    {item.folder.name}
                                  </div>
                                );
                              }
                              const file = item.file;
                              const isAttached = attachedContextFiles.some(f => f.id === file.id);
                              return (
                                <button
                                  key={file.id}
                                  onClick={() => {
                                    if (isAttached) {
                                      setAttachedContextFiles(prev => prev.filter(f => f.id !== file.id));
                                    } else {
                                      setAttachedContextFiles(prev => [...prev, file]);
                                    }
                                  }}
                                  className={`w-full text-left px-2 py-1 rounded-md text-[11px] transition-colors cursor-pointer truncate ${
                                    isAttached ? 'bg-accent-muted text-accent-text' : 'text-text-2 hover:bg-surface-2'
                                  }`}
                                  title={file.name}
                                >
                                  <FileText className="w-3 h-3 inline mr-1.5 flex-shrink-0" />
                                  {file.name}
                                </button>
                              );
                            })
                          )}
                        </div>
                      )}
                    </div>

                    {/* Todo Lists */}
                    <div>
                      <button
                        onClick={() => { setSectionTodos(p => !p); if (!sectionTodos && todoLists.length === 0) loadTodoLists(); }}
                        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-surface-2 transition-colors cursor-pointer"
                      >
                        {sectionTodos ? <ChevronDown className="w-3 h-3 text-text-3" /> : <ChevronRight className="w-3 h-3 text-text-3" />}
                        <ListTodo className="w-3.5 h-3.5 text-text-3" />
                        <span className="text-[11px] font-semibold uppercase tracking-wider text-text-2 flex-1">Tasks</span>
                        {todoLists.length > 0 && <span className="text-[10px] text-text-3">{todoLists.length}</span>}
                      </button>
                      {sectionTodos && (
                        <div className="px-2 pb-2 space-y-0.5">
                          {todoLists.length === 0 ? (
                            <p className="text-[11px] text-text-3 px-2 py-1">No tasks</p>
                          ) : todoLists.map(list => (
                            <div key={list.id}>
                              <button
                                onClick={() => {
                                  const expanding = todoExpandedList !== list.id;
                                  setTodoExpandedList(expanding ? list.id : null);
                                  if (expanding && !todoItems[list.id]) loadTodoItems(list.id);
                                }}
                                className="w-full flex items-center gap-2 px-2 py-1.5 rounded-md text-left hover:bg-surface-2 transition-colors cursor-pointer"
                              >
                                {list.color && <div className="w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: list.color }} />}
                                <span className="text-[11px] text-text-1 truncate flex-1">{list.name}</span>
                                <span className="text-[10px] text-text-3 tabular-nums">{list.completed_items ?? 0}/{list.total_items ?? 0}</span>
                                {todoExpandedList === list.id ? <ChevronUp className="w-3 h-3 text-text-3" /> : <ChevronDown className="w-3 h-3 text-text-3" />}
                              </button>
                              {todoExpandedList === list.id && todoItems[list.id] && (
                                <div className="ml-2 pl-2 border-l border-border-0 space-y-0.5 py-1">
                                  {todoItems[list.id].length === 0 ? (
                                    <p className="text-[10px] text-text-3 py-0.5 px-1">No items</p>
                                  ) : todoItems[list.id].map(item => (
                                    <button
                                      key={item.id}
                                      onClick={() => toggleTodoItem(list.id, item.id)}
                                      className="w-full flex items-start gap-1.5 px-1 py-0.5 rounded text-left hover:bg-surface-2 transition-colors cursor-pointer group/todo"
                                    >
                                      {item.completed ? (
                                        <CircleCheck className="w-3.5 h-3.5 text-accent-primary flex-shrink-0 mt-px" />
                                      ) : (
                                        <div className="w-3.5 h-3.5 rounded-full border border-border-1 flex-shrink-0 mt-px group-hover/todo:border-accent-primary transition-colors" />
                                      )}
                                      <span className={`text-[11px] leading-tight ${item.completed ? 'text-text-3 line-through' : 'text-text-1'}`}>
                                        {item.title}
                                      </span>
                                      {item.last_actor_agent_slug && item.last_actor_avatar && (
                                        <img
                                          src={item.last_actor_avatar}
                                          alt={item.last_actor_agent_name || ''}
                                          className="w-3.5 h-3.5 rounded-full flex-shrink-0 ml-auto mt-px"
                                          title={`${item.last_actor_agent_name}: ${item.last_actor_note}`}
                                        />
                                      )}
                                    </button>
                                  ))}
                                </div>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                </>
              )}
            </div>
          </>
        )}
      </div>

      <Modal open={!!pinTarget} onClose={() => !pinning && setPinTarget(null)} title="Pin this chat?" size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-2">
            Pinning turns <span className="font-medium text-text-1">"{pinTarget?.title}"</span> into a saved reference.
          </p>
          <ul className="space-y-2 text-sm text-text-2">
            <li className="flex gap-2">
              <Pin className="w-4 h-4 text-accent-primary flex-shrink-0 mt-0.5" aria-hidden="true" />
              <span>It moves to the <span className="text-text-1 font-medium">Pinned</span> tab.</span>
            </li>
            <li className="flex gap-2">
              <Sparkles className="w-4 h-4 text-accent-primary flex-shrink-0 mt-0.5" aria-hidden="true" />
              <span>The AI writes a detailed summary so you can catch up without re-reading everything.</span>
            </li>
            <li className="flex gap-2">
              <CircleCheck className="w-4 h-4 text-accent-primary flex-shrink-0 mt-0.5" aria-hidden="true" />
              <span>The chat becomes <span className="text-text-1 font-medium">read-only</span> — nothing is deleted, and the full conversation stays available behind a toggle.</span>
            </li>
          </ul>
          <p className="text-xs text-text-3">You can unpin at any time to carry on the conversation.</p>
          <div className="flex justify-end gap-2 pt-1">
            <Button variant="secondary" size="sm" onClick={() => setPinTarget(null)} disabled={pinning}>Cancel</Button>
            <Button size="sm" onClick={() => pinTarget && pinThread(pinTarget)} loading={pinning}>
              {pinning ? 'Summarising…' : 'Pin chat'}
            </Button>
          </div>
        </div>
      </Modal>

      <Modal open={!!deleteTarget} onClose={() => setDeleteTarget(null)} title="Delete Chat" size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-2">
            Are you sure you want to delete <span className="font-medium text-text-1">"{deleteTarget?.title}"</span>? All messages in this chat will be permanently removed.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setDeleteTarget(null)}>Cancel</Button>
            <Button variant="danger" size="sm" onClick={() => deleteTarget && deleteThread(deleteTarget)}>Delete</Button>
          </div>
        </div>
      </Modal>

      <Modal open={showCompactConfirm} onClose={() => setShowCompactConfirm(false)} title="Compact Chat" size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-2">
            This will summarize all messages into a single compact summary. This cannot be undone.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setShowCompactConfirm(false)}>Cancel</Button>
            <Button size="sm" onClick={compactThread}>Compact</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
