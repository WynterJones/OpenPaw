import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router';
import {
  Plus, MessageSquare, ArrowUp,
  ChevronDown, ChevronLeft, ChevronRight, PanelLeftClose, PanelLeftOpen, Loader2, Trash2, Pencil, Check, X,
  DollarSign, Zap, Minimize2, Square, Users,
  Paperclip, FileText, FolderOpen,
} from 'lucide-react';
import { Header } from '../components/Header';
import { Button } from '../components/Button';
import { SearchBar } from '../components/SearchBar';
import { PagePanel } from '../components/PagePanel';
import { Modal } from '../components/Modal';
import { api, type ChatThread, type ChatMessage, type AgentRole, type StreamEvent, type WSMessage, type ThreadStats, type ThreadMember, type SubAgentTask, contextApi, type ContextFile, type ContextTree, type ContextTreeNode, threadMembers } from '../lib/api';
import { useToast } from '../components/Toast';
import { useAuth } from '../contexts/AuthContext';
import { useWebSocket } from '../lib/useWebSocket';
import { detectBestWidget } from '../components/widgets/detectWidget';
import { timeAgo, getToolDetail } from '../lib/chatUtils';
import { MessageBubble, StreamingMessage } from '../components/chat/MessageBubbles';
import { ThreadMembersPanel } from '../components/chat/ThreadMembersPanel';
import { useThreadList } from '../hooks/useThreadList';
import { useStreamingState } from '../hooks/useStreamingState';
import { useAutocomplete } from '../hooks/useAutocomplete';

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
  } = useAutocomplete();
  const [activeThread, setActiveThread] = useState<string | null>(() => localStorage.getItem('openpaw_active_thread'));
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [agent] = useState('');
  const [showThreads, setShowThreads] = useState(true);
  const [sending, setSending] = useState(false);
  const [thinking, setThinking] = useState(false);
  const [workStatus, setWorkStatus] = useState<string | null>(null);
  const [roles, setRoles] = useState<AgentRole[]>([]);
  const pollingRef = useRef<number | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const THREADS_PER_PAGE = 10;

  const [contextItems, setContextItems] = useState<ContextItem[]>([]);
  const [attachedContextFiles, setAttachedContextFiles] = useState<ContextFile[]>([]);

  // File attachment state
  const [pendingAttachments, setPendingAttachments] = useState<File[]>([]);
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
  };

  const handleInputKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
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
  const [threadStats, setThreadStats] = useState<ThreadStats | null>(null);
  const [compacting, setCompacting] = useState(false);
  const [showCompactConfirm, setShowCompactConfirm] = useState(false);
  const [members, setMembers] = useState<ThreadMember[]>([]);
  const [showMembers, setShowMembers] = useState(true);
  const [routingIndicator, setRoutingIndicator] = useState<string | null>(null);
  const [activeAgentSlug, setActiveAgentSlug] = useState<string | null>(null);
  const [subAgentTasks, setSubAgentTasks] = useState<SubAgentTask[]>([]);
  const activeThreadRef = useRef(activeThread);
  activeThreadRef.current = activeThread;
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
            if (event.tool_name === 'call_tool' && event.tool_input) {
              toolInputMapRef.current.set(toolId, {
                endpoint: (event.tool_input.endpoint as string) || undefined,
              });
            }
            setStreamingTools(prev => {
              const existing = prev.findIndex(t => t.id === toolId);
              if (existing >= 0) {
                return prev.map((t, i) => i === existing ? { ...t, done: false, detail: detail || t.detail } : t);
              }
              return [...prev, { name: event.tool_name!, id: toolId, done: false, detail }];
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
      // Immediately clear stale sidebar indicator; status check / WebSocket will restore if truly active
      setActiveThreadIds(prev => {
        if (!prev.has(activeThread)) return prev;
        const next = new Set(prev); next.delete(activeThread); return next;
      });

      localStorage.setItem('openpaw_active_thread', activeThread);
      loadMessages(activeThread);
      loadStats(activeThread);
      loadMembers(activeThread);

      // Recover streaming state if agent is mid-stream
      api.get<{ active: boolean; stream_state?: { text: string; tools: { name: string; id: string; done: boolean; detail?: string }[]; agent_slug: string; active: boolean } }>(`/chat/threads/${activeThread}/status`).then(status => {
        if (status.stream_state && status.stream_state.active) {
          const ss = status.stream_state;
          if (ss.text) setStreamingText(ss.text);
          if (ss.agent_slug) setActiveAgentSlug(ss.agent_slug);
          if (ss.tools && ss.tools.length > 0) {
            setStreamingTools(ss.tools.map(t => ({ name: t.name, id: t.id, done: t.done, detail: t.detail || '' })));
          }
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
      if (ticks > maxTicks) { setThinking(false); setWorkStatus(null); setActiveThreadIds(prev => { const next = new Set(prev); next.delete(threadId); return next; }); stopPolling(); return; }
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
          setThinking(false);
          setWorkStatus(null);
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
    if (!input.trim() || !activeThread || sending) return;
    let content = input.trim();
    const threadId = activeThread;
    const isFirstMessage = messages.length === 0;

    // Append context file content for [[filename]] references
    if (attachedContextFiles.length > 0) {
      const contextParts: string[] = [];
      for (const cf of attachedContextFiles) {
        try {
          const result = await contextApi.getFile(cf.id);
          if (result.content) {
            contextParts.push(`\n\n---\n**Context: ${cf.name}**\n${result.content}`);
          }
        } catch (e) { console.warn('fetchContextFile failed:', e); }
      }
      if (contextParts.length > 0) {
        content += contextParts.join('');
      }
    }

    // Append pending file attachments inline
    if (pendingAttachments.length > 0) {
      const attachParts: string[] = [];
      for (const file of pendingAttachments) {
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
      if (attachParts.length > 0) {
        content += attachParts.join('');
      }
    }

    // Reset streaming state
    resetStreaming();
    setSubAgentTasks([]);
    setRoutingIndicator(null);
    hadToolSinceLastTextRef.current = false;
    toolInputMapRef.current.clear();

    setInput('');
    setAttachedContextFiles([]);
    setPendingAttachments([]);
    if (textareaRef.current) { textareaRef.current.style.height = 'auto'; }
    setSending(true); setThinking(true);
    const userMsg: ChatMessage = { id: `temp-${Date.now()}`, thread_id: threadId, role: 'user', content, agent_role_slug: agent, cost_usd: 0, input_tokens: 0, output_tokens: 0, created_at: new Date().toISOString() };
    setMessages(prev => [...prev, userMsg]);
    try {
      const saved = await api.post<ChatMessage>(`/chat/threads/${threadId}/messages`, { content, agent_role_slug: agent });
      setMessages(prev => prev.map(m => m.id === userMsg.id ? saved : m));
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

  const filteredThreads = threads.filter(t => t?.title?.toLowerCase().includes(search.toLowerCase()));
  const totalPages = Math.max(1, Math.ceil(filteredThreads.length / THREADS_PER_PAGE));
  const clampedPage = Math.min(threadPage, totalPages);
  const pagedThreads = filteredThreads.slice((clampedPage - 1) * THREADS_PER_PAGE, clampedPage * THREADS_PER_PAGE);
  const activeRole = roles.find(r => r.slug === agent);
  const thinkingRole = activeAgentSlug ? roles.find(r => r.slug === activeAgentSlug) : activeRole;
  const isStreaming = streamingText.length > 0 || streamingTools.length > 0 || subAgentTasks.length > 0;

  return (
    <div className="flex flex-col h-full">
      <Header title="Chat"
        actions={
          <button onClick={() => setShowThreads(!showThreads)} className="md:hidden p-2 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer" aria-label={showThreads ? 'Hide chat threads' : 'Show chat threads'}>
            {showThreads ? <PanelLeftClose className="w-5 h-5" aria-hidden="true" /> : <PanelLeftOpen className="w-5 h-5" aria-hidden="true" />}
          </button>
        }
      />
      <div className="flex flex-1 overflow-hidden relative">
        <div className={`${showThreads ? 'translate-x-0' : '-translate-x-full md:translate-x-0'} absolute md:relative z-30 w-[85vw] max-w-72 md:w-72 h-full flex flex-col border-r border-border-0 bg-surface-1 transition-transform duration-200`}>
          <div className="p-3 space-y-2">
            <Button onClick={createThread} icon={<Plus className="w-4 h-4" />} className="w-full" size="sm">New Chat</Button>
            <SearchBar value={search} onChange={(v) => { setSearch(v); setThreadPage(1); }} placeholder="Search chats..." />
          </div>
          <div className="flex-1 overflow-y-auto px-2 pb-2 space-y-0.5">
            {pagedThreads.length === 0 ? (
              <div className="text-center py-8 text-sm text-text-3">No chats yet</div>
            ) : pagedThreads.map(thread => (
              <div key={thread.id} aria-current={activeThread === thread.id ? 'true' : undefined} className={`group relative rounded-lg transition-colors ${activeThread === thread.id ? 'bg-accent-muted text-accent-text' : 'text-text-1 hover:bg-surface-2'}`}>
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
                        {activeThreadIds.has(thread.id) && (
                          <Loader2 className="w-4 h-4 flex-shrink-0 text-accent-primary animate-spin" />
                        )}
                        <span className="text-sm font-medium truncate">{thread.title}</span>
                      </div>
                      <p className="text-xs text-text-3 mt-0.5">
                        {activeThreadIds.has(thread.id)
                          ? <span className="text-accent-primary">Working...</span>
                          : timeAgo(thread.updated_at)
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
                <div className="flex items-center gap-3 px-4 py-1.5 border-b border-border-0 bg-surface-1/80 text-[11px] text-text-3">
                  <span className="flex items-center gap-1"><DollarSign className="w-3 h-3" aria-hidden="true" />${threadStats.total_cost_usd.toFixed(4)}</span>
                  <span className="flex items-center gap-1"><Zap className="w-3 h-3" aria-hidden="true" />{((threadStats.total_input_tokens || 0) + (threadStats.total_output_tokens || 0)).toLocaleString()} tokens</span>
                  <div className="flex items-center gap-1.5 flex-1">
                    <div
                      className="flex-1 h-1.5 rounded-full bg-surface-3 overflow-hidden max-w-40"
                      role="progressbar"
                      aria-valuenow={Math.round((threadStats.context_used_tokens / threadStats.context_limit_tokens) * 100)}
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-label="Context window usage"
                    >
                      <div
                        className={`h-full rounded-full transition-all ${
                          (threadStats.context_used_tokens / threadStats.context_limit_tokens) > 0.8
                            ? 'bg-red-400'
                            : (threadStats.context_used_tokens / threadStats.context_limit_tokens) > 0.5
                              ? 'bg-amber-400'
                              : 'bg-emerald-400'
                        }`}
                        style={{ width: `${Math.min(100, (threadStats.context_used_tokens / threadStats.context_limit_tokens) * 100)}%` }}
                      />
                    </div>
                    <span>{Math.round((threadStats.context_used_tokens / threadStats.context_limit_tokens) * 100)}% context</span>
                  </div>
                  {(threadStats.context_used_tokens / threadStats.context_limit_tokens) > 0.5 && (
                    <button
                      onClick={() => setShowCompactConfirm(true)}
                      disabled={compacting}
                      className="flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-50"
                      title="Compact chat"
                      aria-label="Compact chat"
                    >
                      {compacting ? <Loader2 className="w-3 h-3 animate-spin" aria-hidden="true" /> : <Minimize2 className="w-3 h-3" aria-hidden="true" />}
                      Compact
                    </button>
                  )}
                  <button
                    onClick={() => setShowMembers(!showMembers)}
                    className={`flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium transition-colors cursor-pointer ${showMembers ? 'text-accent-primary bg-accent-muted' : 'text-text-2 hover:text-text-1 hover:bg-surface-2'}`}
                    title={showMembers ? 'Hide members' : 'Show members'}
                    aria-label={showMembers ? 'Hide members' : 'Show members'}
                    aria-pressed={showMembers}
                  >
                    <Users className="w-3 h-3" aria-hidden="true" />
                    {members.length > 0 && <span>{members.length}</span>}
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
                      Ask anything, mention agents with <span className="text-text-1 font-medium">@</span> to bring them into the chat, or attach files & folders with <span className="text-text-1 font-medium">!!</span>
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
                {messages.map(msg => <MessageBubble key={msg.id} message={msg} roles={roles} onRefresh={() => activeThread && loadMessages(activeThread)} userAvatarPath={user?.avatar_path} />)}
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
                    <div className="w-8 h-8 rounded-md bg-surface-2 flex items-center justify-center flex-shrink-0 overflow-hidden ring-1 ring-border-1">
                      {thinkingRole ? (
                        <img src={thinkingRole.avatar_path} alt={thinkingRole.name} className="w-8 h-8 rounded-md object-cover" />
                      ) : (
                        <img src={roles.find(r => r.slug === 'builder')?.avatar_path || '/gateway-avatar.png'} alt="AI" className="w-8 h-8 rounded-md object-cover" />
                      )}
                    </div>
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
              <div className="absolute bottom-0 left-0 right-0 z-10 p-3 md:p-4 border-t border-white/[0.06] bg-black/40 backdrop-blur-xl">
                <div className="max-w-[960px] mx-auto relative">
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
                  {(pendingAttachments.length > 0 || attachedContextFiles.length > 0) && (
                    <div className="flex flex-wrap gap-1.5 px-3 pt-2.5">
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
                    placeholder="Ask anything... (@ agents, !! context)"
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
        {activeThread && showMembers && (
          <PagePanel side="right" width="w-56" desktopOnly>
            <ThreadMembersPanel
              members={members}
              activeSlug={activeAgentSlug}
              onRemove={removeMember}
            />
          </PagePanel>
        )}
      </div>

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
