import { useState, useEffect, useCallback, useRef } from 'react';
import {
  FileText,
  RefreshCw,
  Clock,
  DollarSign,
  Zap,
  Activity,
} from 'lucide-react';
import { Toggle } from '../components/Toggle';
import { Header } from '../components/Header';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { EmptyState } from '../components/EmptyState';
import { DataTable } from '../components/DataTable';
import { Pagination } from '../components/Pagination';
import { Select } from '../components/Input';
import { api, type LogEntry, type LogStats, type WSMessage } from '../lib/api';
import { SearchBar } from '../components/SearchBar';
import { useWebSocket } from '../lib/useWebSocket';

const CATEGORIES = [
  { value: '', label: 'All Categories' },
  { value: 'auth', label: 'Auth' },
  { value: 'chat', label: 'Chat' },
  { value: 'agent', label: 'Agent' },
  { value: 'tool_call', label: 'Tool Calls' },
  { value: 'work_order', label: 'Work Orders' },
  { value: 'tool', label: 'Tools' },
  { value: 'secret', label: 'Secrets' },
  { value: 'schedule', label: 'Schedules' },
  { value: 'dashboard', label: 'Dashboards' },
  { value: 'settings', label: 'Settings' },
  { value: 'context', label: 'Context' },
  { value: 'system', label: 'System' },
];

const ACTION_TYPES = [
  { value: '', label: 'All Actions' },
  { value: 'login', label: 'Login' },
  { value: 'logout', label: 'Logout' },
  { value: 'password_changed', label: 'Password Changed' },
  { value: 'setup_init', label: 'Setup Init' },
  { value: 'user_message_sent', label: 'User Message' },
  { value: 'chat_thread_created', label: 'Chat Created' },
  { value: 'chat_thread_deleted', label: 'Chat Deleted' },
  { value: 'chat_thread_compacted', label: 'Chat Compacted' },
  { value: 'chat_thread_stopped', label: 'Chat Stopped' },
  { value: 'thread_member_joined', label: 'Member Joined' },
  { value: 'thread_member_removed', label: 'Member Removed' },
  { value: 'routing_explicit_agent', label: 'Route: Explicit' },
  { value: 'routing_mention', label: 'Route: Mention' },
  { value: 'routing_last_responder', label: 'Route: Last Responder' },
  { value: 'routing_gateway', label: 'Route: Gateway' },
  { value: 'agent_response', label: 'Agent Response' },
  { value: 'agent_tool_call', label: 'Agent Tool Call' },
  { value: 'agent_spawned', label: 'Agent Spawned' },
  { value: 'agent_completed', label: 'Agent Completed' },
  { value: 'agent_failed', label: 'Agent Failed' },
  { value: 'agent_stopped', label: 'Agent Stopped' },
  { value: 'builder_tool_call', label: 'Builder Tool Call' },
  { value: 'work_order_created', label: 'Work Order Created' },
  { value: 'work_order_confirmed', label: 'Work Order Confirmed' },
  { value: 'work_order_rejected', label: 'Work Order Rejected' },
  { value: 'tool_created', label: 'Tool Created' },
  { value: 'tool_updated', label: 'Tool Updated' },
  { value: 'tool_called', label: 'Tool Called' },
  { value: 'tool_enabled', label: 'Tool Enabled' },
  { value: 'tool_disabled', label: 'Tool Disabled' },
  { value: 'tool_deleted', label: 'Tool Deleted' },
  { value: 'tool_compiled', label: 'Tool Compiled' },
  { value: 'tool_started', label: 'Tool Started' },
  { value: 'tool_stopped', label: 'Tool Stopped' },
  { value: 'tool_restarted', label: 'Tool Restarted' },
  { value: 'secret_created', label: 'Secret Created' },
  { value: 'secret_rotated', label: 'Secret Rotated' },
  { value: 'secret_deleted', label: 'Secret Deleted' },
  { value: 'schedule_created', label: 'Schedule Created' },
  { value: 'schedule_updated', label: 'Schedule Updated' },
  { value: 'schedule_deleted', label: 'Schedule Deleted' },
  { value: 'schedule_run_now', label: 'Schedule Run' },
  { value: 'dashboard_created', label: 'Dashboard Created' },
  { value: 'dashboard_updated', label: 'Dashboard Updated' },
  { value: 'dashboard_deleted', label: 'Dashboard Deleted' },
  { value: 'settings_updated', label: 'Settings Updated' },
  { value: 'design_config_updated', label: 'Design Updated' },
  { value: 'model_settings_updated', label: 'Models Updated' },
  { value: 'api_key_updated', label: 'API Key Updated' },
  { value: 'context_folder_created', label: 'Context Folder Created' },
  { value: 'context_folder_deleted', label: 'Context Folder Deleted' },
  { value: 'context_file_uploaded', label: 'Context File Uploaded' },
  { value: 'context_file_deleted', label: 'Context File Deleted' },
  { value: 'data_deleted', label: 'Data Deleted' },
];

const categoryColorMap: Record<string, string> = {
  auth: 'text-violet-400 bg-violet-500/10',
  chat: 'text-blue-400 bg-blue-500/10',
  agent: 'text-emerald-400 bg-emerald-500/10',
  tool_call: 'text-amber-400 bg-amber-500/10',
  work_order: 'text-orange-400 bg-orange-500/10',
  tool: 'text-pink-400 bg-pink-500/10',
  secret: 'text-red-400 bg-red-500/10',
  schedule: 'text-cyan-400 bg-cyan-500/10',
  dashboard: 'text-fuchsia-400 bg-fuchsia-500/10',
  settings: 'text-rose-300 bg-rose-400/10',
  context: 'text-teal-400 bg-teal-500/10',
  system: 'text-gray-400 bg-gray-500/10',
};

const actionColorMap: Record<string, string> = {
  login: 'text-violet-400 bg-violet-500/10',
  logout: 'text-violet-300/60 bg-violet-400/5',
  password_changed: 'text-violet-400 bg-violet-500/10',
  user_message_sent: 'text-blue-400 bg-blue-500/10',
  chat_thread_created: 'text-blue-300 bg-blue-400/10',
  chat_thread_deleted: 'text-red-400 bg-red-500/10',
  chat_thread_compacted: 'text-blue-300 bg-blue-400/10',
  chat_thread_stopped: 'text-blue-300/60 bg-blue-400/5',
  thread_member_joined: 'text-blue-400 bg-blue-500/10',
  thread_member_removed: 'text-blue-300/60 bg-blue-400/5',
  routing_explicit_agent: 'text-emerald-300 bg-emerald-400/10',
  routing_mention: 'text-emerald-300 bg-emerald-400/10',
  routing_last_responder: 'text-emerald-300 bg-emerald-400/10',
  routing_gateway: 'text-emerald-400 bg-emerald-500/10',
  agent_response: 'text-emerald-400 bg-emerald-500/10',
  agent_tool_call: 'text-amber-400 bg-amber-500/10',
  agent_spawned: 'text-emerald-400 bg-emerald-500/10',
  agent_completed: 'text-emerald-300 bg-emerald-400/10',
  agent_failed: 'text-red-400 bg-red-500/10',
  agent_stopped: 'text-rose-300 bg-rose-400/10',
  builder_tool_call: 'text-amber-300 bg-amber-400/10',
  work_order_created: 'text-orange-400 bg-orange-500/10',
  work_order_confirmed: 'text-orange-300 bg-orange-400/10',
  work_order_rejected: 'text-red-400 bg-red-500/10',
  tool_created: 'text-pink-300 bg-pink-400/10',
  tool_updated: 'text-rose-400 bg-rose-500/10',
  tool_called: 'text-fuchsia-400 bg-fuchsia-500/10',
  tool_enabled: 'text-pink-300 bg-pink-400/10',
  tool_disabled: 'text-pink-300/60 bg-pink-400/5',
  tool_deleted: 'text-red-400 bg-red-500/10',
  tool_compiled: 'text-pink-400 bg-pink-500/10',
  tool_started: 'text-pink-400 bg-pink-500/10',
  tool_stopped: 'text-pink-300/60 bg-pink-400/5',
  tool_restarted: 'text-pink-400 bg-pink-500/10',
  secret_created: 'text-red-300 bg-red-400/10',
  secret_rotated: 'text-red-400 bg-red-500/10',
  secret_deleted: 'text-red-400 bg-red-500/10',
  schedule_created: 'text-cyan-300 bg-cyan-400/10',
  schedule_updated: 'text-cyan-400 bg-cyan-500/10',
  schedule_deleted: 'text-red-400 bg-red-500/10',
  schedule_run_now: 'text-cyan-400 bg-cyan-500/10',
  dashboard_created: 'text-fuchsia-300 bg-fuchsia-400/10',
  dashboard_updated: 'text-fuchsia-400 bg-fuchsia-500/10',
  dashboard_deleted: 'text-red-400 bg-red-500/10',
  settings_updated: 'text-rose-300 bg-rose-400/10',
  design_config_updated: 'text-rose-300 bg-rose-400/10',
  model_settings_updated: 'text-rose-300 bg-rose-400/10',
  api_key_updated: 'text-rose-400 bg-rose-500/10',
  context_folder_created: 'text-teal-300 bg-teal-400/10',
  context_folder_deleted: 'text-red-400 bg-red-500/10',
  context_file_uploaded: 'text-teal-400 bg-teal-500/10',
  context_file_deleted: 'text-red-400 bg-red-500/10',
  data_deleted: 'text-red-500 bg-red-500/10',
};

const PAGE_SIZE = 50;

function formatCost(value: number): string {
  if (value === 0) return '$0.00';
  if (value < 0.01) return `$${value.toFixed(4)}`;
  return `$${value.toFixed(2)}`;
}

function formatNumber(value: number): string {
  return value.toLocaleString();
}

export function Logs() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('');
  const [actionFilter, setActionFilter] = useState('');
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [stats, setStats] = useState<LogStats | null>(null);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const loadStats = useCallback(async () => {
    try {
      const data = await api.get<LogStats>('/logs/stats');
      setStats(data);
    } catch (e) { console.warn('loadLogStats failed:', e); }
  }, []);

  const loadLogs = useCallback(async () => {
    try {
      const params = new URLSearchParams();
      if (categoryFilter) params.set('category', categoryFilter);
      if (actionFilter) params.set('action', actionFilter);
      params.set('limit', String(PAGE_SIZE));
      params.set('offset', String(page * PAGE_SIZE));
      const data = await api.get<{ logs: LogEntry[]; total: number }>(`/logs?${params.toString()}`);
      setLogs(data.logs || []);
      setTotal(data.total || 0);
    } catch (e) {
      console.warn('loadLogs failed:', e);
      setLogs([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [categoryFilter, actionFilter, page]);

  useEffect(() => {
    setPage(0);
  }, [categoryFilter, actionFilter, search]);

  useEffect(() => {
    loadLogs();
    loadStats();
  }, [loadLogs, loadStats]);

  const wsDebounceRef = useRef<number | null>(null);

  const handleWsMessage = useCallback((msg: WSMessage) => {
    if (!autoRefresh) return;
    if (msg.type === 'audit_log_created') {
      if (wsDebounceRef.current) clearTimeout(wsDebounceRef.current);
      wsDebounceRef.current = window.setTimeout(() => {
        loadLogs();
        loadStats();
      }, 1_000);
    }
  }, [autoRefresh, loadLogs, loadStats]);

  const { connected: wsConnected } = useWebSocket({ onMessage: handleWsMessage, enabled: autoRefresh });

  // Fallback: poll every 10s when auto-refresh is on but WS is disconnected
  useEffect(() => {
    if (!autoRefresh || wsConnected) return;
    const interval = setInterval(loadLogs, 10_000);
    return () => clearInterval(interval);
  }, [autoRefresh, wsConnected, loadLogs]);

  // Clean up debounce timer
  useEffect(() => {
    return () => { if (wsDebounceRef.current) clearTimeout(wsDebounceRef.current); };
  }, []);

  const filtered = logs.filter(l => {
    if (search) {
      const q = search.toLowerCase();
      if (!l.username.toLowerCase().includes(q) && !l.target.toLowerCase().includes(q) && !l.details.toLowerCase().includes(q)) {
        return false;
      }
    }
    return true;
  });

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Activity Log"
        actions={
          <div className="flex items-center gap-2">
            <span className="text-xs font-medium text-text-2">Auto-refresh</span>
            <Toggle enabled={autoRefresh} onChange={setAutoRefresh} label="Auto-refresh logs" />
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        {stats && (
          <div className="flex gap-3 mb-4 md:mb-6 overflow-x-auto md:grid md:grid-cols-3">
            <Card className="shrink-0 min-w-[140px] flex-1">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-emerald-500/10 shrink-0">
                  <DollarSign className="w-4 h-4 text-emerald-400" />
                </div>
                <div className="min-w-0">
                  <p className="text-xs text-text-3 whitespace-nowrap">Est. Cost</p>
                  <p className="text-lg font-semibold text-text-0 truncate">{formatCost(stats.total_cost_usd)}</p>
                </div>
              </div>
            </Card>
            <Card className="shrink-0 min-w-[140px] flex-1">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-amber-500/10 shrink-0">
                  <Zap className="w-4 h-4 text-amber-400" />
                </div>
                <div className="min-w-0">
                  <p className="text-xs text-text-3 whitespace-nowrap">Tokens</p>
                  <p className="text-lg font-semibold text-text-0 truncate">{formatNumber(stats.total_tokens)}</p>
                </div>
              </div>
            </Card>
            <Card className="shrink-0 min-w-[140px] flex-1">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-blue-500/10 shrink-0">
                  <Activity className="w-4 h-4 text-blue-400" />
                </div>
                <div className="min-w-0">
                  <p className="text-xs text-text-3 whitespace-nowrap">Activity</p>
                  <p className="text-lg font-semibold text-text-0 truncate">{formatNumber(stats.total_activity)}</p>
                </div>
              </div>
            </Card>
          </div>
        )}

        <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 sm:gap-3 mb-4 md:mb-6">
          <SearchBar value={search} onChange={setSearch} placeholder="Search logs..." className="flex-1 sm:max-w-sm" />
          <div className="flex gap-2">
            <div className="flex-1 sm:flex-none">
              <Select
                value={categoryFilter}
                onChange={e => setCategoryFilter(e.target.value)}
                options={CATEGORIES}
              />
            </div>
            <div className="flex-1 sm:flex-none">
              <Select
                value={actionFilter}
                onChange={e => setActionFilter(e.target.value)}
                options={ACTION_TYPES}
              />
            </div>
            <Button variant="secondary" size="sm" onClick={loadLogs} icon={<RefreshCw className="w-4 h-4" />}>
              <span className="hidden sm:inline">Refresh</span>
            </Button>
          </div>
        </div>

        <Pagination page={page} totalPages={totalPages} total={total} onPageChange={setPage} />

        <Card padding={false}>
          {loading ? (
            <div className="flex items-center justify-center py-16">
              <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
            </div>
          ) : (
            <DataTable
              columns={[
                {
                  key: 'category',
                  header: 'Category',
                  hideOnMobile: true,
                  render: (l: LogEntry) => {
                    const color = categoryColorMap[l.category] || 'text-gray-400 bg-gray-500/10';
                    return (
                      <span className={`inline-flex px-2 py-0.5 rounded-full text-xs font-medium ${color}`}>
                        {l.category}
                      </span>
                    );
                  },
                },
                {
                  key: 'action',
                  header: 'Action',
                  render: (l: LogEntry) => {
                    const color = actionColorMap[l.action] || 'text-gray-400 bg-gray-500/10';
                    return (
                      <div>
                        <span className={`inline-flex px-2 py-0.5 rounded-full text-xs font-medium ${color}`}>
                          {l.action}
                        </span>
                        <p className="text-xs text-text-3 mt-1 md:hidden">{l.username} &middot; {new Date(l.created_at).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</p>
                      </div>
                    );
                  },
                },
                {
                  key: 'username',
                  header: 'User',
                  hideOnMobile: true,
                  render: (l: LogEntry) => (
                    <span className="text-sm font-medium text-text-1">{l.username}</span>
                  ),
                },
                {
                  key: 'target',
                  header: 'Target',
                  render: (l: LogEntry) => (
                    <span className="text-sm text-text-1 truncate max-w-[120px] md:max-w-xs block">{l.target}</span>
                  ),
                },
                {
                  key: 'created_at',
                  header: 'Timestamp',
                  hideOnMobile: true,
                  render: (l: LogEntry) => (
                    <span className="text-sm text-text-1 flex items-center gap-1.5 whitespace-nowrap">
                      <Clock className="w-3.5 h-3.5 text-text-3" />
                      {new Date(l.created_at).toLocaleString()}
                    </span>
                  ),
                },
                {
                  key: 'details',
                  header: 'Details',
                  hideOnMobile: true,
                  render: (l: LogEntry) => (
                    <span className="text-sm text-text-2 truncate max-w-xs block">{l.details}</span>
                  ),
                },
              ]}
              data={filtered}
              keyExtractor={l => l.id}
              emptyState={
                <EmptyState
                  icon={<FileText className="w-8 h-8" />}
                  title="No activity yet"
                  description="Activity will appear here as you interact with OpenPaw."
                />
              }
            />
          )}
        </Card>
      </div>
    </div>
  );
}
