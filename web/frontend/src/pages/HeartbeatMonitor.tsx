import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Heart, Play, Clock, CheckCircle, XCircle, Loader2, RefreshCw, Settings2, Search } from 'lucide-react';
import { Toggle } from '../components/Toggle';
import { Header } from '../components/Header';
import { Card } from '../components/Card';
import { Button } from '../components/Button';
import { Select } from '../components/Input';
import { Modal } from '../components/Modal';
import { Pagination } from '../components/Pagination';
import { StatusBadge } from '../components/StatusBadge';
import { EmptyState } from '../components/EmptyState';
import { LoadingSpinner } from '../components/LoadingSpinner';
import { useToast } from '../components/Toast';
import { useWebSocket } from '../lib/useWebSocket';
import {
  api,
  heartbeatApi,
  type AgentRole,
  type HeartbeatExecution,
  type HeartbeatConfig,
  type WSMessage,
} from '../lib/api';

function timeAgo(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diff = Math.floor((now - then) / 1000);
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

function formatTimestamp(dateStr: string): string {
  return new Date(dateStr).toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit',
  });
}

function formatDuration(start: string, end: string | null): string {
  if (!end) return 'running...';
  const ms = new Date(end).getTime() - new Date(start).getTime();
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60000).toFixed(1)}m`;
}

function parseActions(s: string): string[] {
  try { return JSON.parse(s); } catch (e) { console.warn('parseActions: failed to parse JSON:', e); return []; }
}

function formatInterval(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m`;
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return m > 0 ? `${h}h ${m}m` : `${h}h`;
}

function parseDuration(input: string): number | null {
  const trimmed = input.trim().toLowerCase();
  if (!trimmed) return null;

  // Pure number = treat as seconds
  if (/^\d+$/.test(trimmed)) return parseInt(trimmed, 10);

  let total = 0;
  let matched = false;
  const regex = /(\d+)\s*(h|m|s)/g;
  let match;
  while ((match = regex.exec(trimmed)) !== null) {
    matched = true;
    const val = parseInt(match[1], 10);
    switch (match[2]) {
      case 'h': total += val * 3600; break;
      case 'm': total += val * 60; break;
      case 's': total += val; break;
    }
  }
  return matched ? total : null;
}

function secondsToDurationStr(sec: number): string {
  if (sec <= 0) return '0s';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const parts: string[] = [];
  if (h > 0) parts.push(`${h}h`);
  if (m > 0) parts.push(`${m}m`);
  if (s > 0) parts.push(`${s}s`);
  return parts.join(' ');
}

function DurationInput({ value, onSave }: { value: string; onSave: (seconds: string) => void }) {
  const numericValue = Number(value) || 0;
  const [text, setText] = useState(() => secondsToDurationStr(numericValue));
  const [error, setError] = useState('');

  const displayValue = useMemo(() => secondsToDurationStr(numericValue), [numericValue]);

  const handleBlur = () => {
    const parsed = parseDuration(text);
    if (parsed === null || parsed < 60) {
      setError('Minimum 1m');
      setText(displayValue);
      return;
    }
    setError('');
    if (parsed !== numericValue) {
      onSave(String(parsed));
    } else {
      setText(secondsToDurationStr(parsed));
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      (e.target as HTMLInputElement).blur();
    }
  };

  return (
    <div>
      <label htmlFor="heartbeat-interval" className="block text-xs font-medium text-text-2 mb-1.5">Interval</label>
      <input
        id="heartbeat-interval"
        type="text"
        value={text}
        onChange={e => { setText(e.target.value); setError(''); }}
        onBlur={handleBlur}
        onKeyDown={handleKeyDown}
        placeholder="e.g. 10m, 1h 30m, 2h"
        className={`w-full rounded-lg border px-3 py-2 text-sm text-text-1 bg-surface-0 placeholder:text-text-3 focus:outline-none focus:ring-2 focus:ring-accent-primary/50 ${error ? 'border-red-500/50' : 'border-border-1'}`}
      />
      {error ? (
        <p className="text-[11px] text-red-400 mt-1">{error}</p>
      ) : (
        <p className="text-[11px] text-text-3 mt-1">Use h, m, s &mdash; e.g. 1h 30m, 10m, 45s</p>
      )}
    </div>
  );
}

const TIMEZONE_OPTIONS = [
  'UTC', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Asia/Tokyo', 'Asia/Shanghai',
  'Asia/Kolkata', 'Australia/Sydney', 'Pacific/Auckland',
];

const PAGE_SIZE = 10;

export function HeartbeatMonitor() {
  const { toast } = useToast();
  const [loading, setLoading] = useState(true);
  const [config, setConfig] = useState<HeartbeatConfig | null>(null);
  const [executions, setExecutions] = useState<HeartbeatExecution[]>([]);
  const [totalExecs, setTotalExecs] = useState(0);
  const [agents, setAgents] = useState<AgentRole[]>([]);
  const [running, setRunning] = useState(false);
  const [showSettings, setShowSettings] = useState(false);

  const [page, setPage] = useState(0);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [selectedExec, setSelectedExec] = useState<HeartbeatExecution | null>(null);

  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const handleSearchChange = (val: string) => {
    setSearch(val);
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setDebouncedSearch(val);
      setPage(0);
    }, 300);
  };

  const fetchExecutions = useCallback(async (p: number, q: string) => {
    try {
      const result = await heartbeatApi.listExecutions({
        limit: PAGE_SIZE,
        offset: p * PAGE_SIZE,
        q: q || undefined,
      });
      setExecutions(result.items);
      setTotalExecs(result.total);
      setRunning(result.items.some(e => e.status === 'running'));
    } catch (e) {
      console.warn('loadExecutions failed:', e);
    }
  }, []);

  const refreshAll = useCallback(async () => {
    try {
      const [cfg, agentList] = await Promise.all([
        heartbeatApi.getConfig(),
        api.get<AgentRole[]>('/agent-roles'),
      ]);
      setConfig(cfg);
      setAgents(agentList);
    } catch (e) {
      console.warn('refreshHeartbeatData failed:', e);
      toast('error', 'Failed to load heartbeat data');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => { refreshAll(); }, [refreshAll]);
  useEffect(() => { fetchExecutions(page, debouncedSearch); }, [page, debouncedSearch, fetchExecutions]);

  const handleWsMessage = useCallback((msg: WSMessage) => {
    if (msg.type === 'heartbeat_started') {
      setRunning(true);
      fetchExecutions(page, debouncedSearch);
    }
    if (msg.type === 'heartbeat_completed') {
      fetchExecutions(page, debouncedSearch);
    }
    if (msg.type === 'heartbeat_cycle_done') {
      setRunning(false);
      fetchExecutions(page, debouncedSearch);
    }
  }, [fetchExecutions, page, debouncedSearch]);

  useWebSocket({ onMessage: handleWsMessage });

  const handleRunNow = async () => {
    try {
      await heartbeatApi.runNow();
      setRunning(true);
      toast('success', 'Heartbeat cycle triggered');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to trigger heartbeat');
    }
  };

  const handleToggleGlobal = async () => {
    if (!config) return;
    const newVal = config.heartbeat_enabled === 'true' ? 'false' : 'true';
    try {
      const updated = await heartbeatApi.updateConfig({ heartbeat_enabled: newVal });
      setConfig(updated);
      toast('success', newVal === 'true' ? 'Heartbeat enabled' : 'Heartbeat disabled');
    } catch (e) {
      console.warn('toggleHeartbeat failed:', e);
      toast('error', 'Failed to toggle heartbeat');
    }
  };

  const handleConfigSave = async (key: string, value: string) => {
    try {
      const updated = await heartbeatApi.updateConfig({ [key]: value });
      setConfig(updated);
    } catch (e) {
      console.warn('saveHeartbeatConfig failed:', e);
      toast('error', 'Failed to update setting');
    }
  };

  const handleToggleAgent = async (slug: string, current: boolean) => {
    try {
      const updated = await api.put<AgentRole>(`/agent-roles/${slug}`, { heartbeat_enabled: !current });
      setAgents(prev => prev.map(a => a.slug === slug ? updated : a));
      toast('success', !current ? `Heartbeat enabled for ${slug}` : `Heartbeat disabled for ${slug}`);
    } catch (e) {
      console.warn('toggleAgentHeartbeat failed:', e);
      toast('error', 'Failed to toggle agent heartbeat');
    }
  };

  if (loading) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Heartbeat" />
        <div className="flex-1 flex items-center justify-center">
          <LoadingSpinner message="Loading heartbeat monitor..." />
        </div>
      </div>
    );
  }

  const isEnabled = config?.heartbeat_enabled === 'true';
  const activeExecutions = executions.filter(e => e.status === 'running');
  const pastExecutions = executions.filter(e => e.status !== 'running');
  const enabledAgents = agents.filter(a => a.enabled && a.heartbeat_enabled);
  const intervalLabel = config?.heartbeat_interval_sec ? formatInterval(Number(config.heartbeat_interval_sec)) : '—';
  const totalPages = Math.ceil(totalExecs / PAGE_SIZE);

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Heartbeat"
        actions={
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowSettings(!showSettings)}
              icon={<Settings2 className="w-3.5 h-3.5" />}
            >
              <span className="hidden sm:inline">Settings</span>
            </Button>
            <Button
              size="sm"
              onClick={handleRunNow}
              disabled={running || !isEnabled}
              loading={running}
              icon={<Play className="w-3.5 h-3.5" />}
            >
              Run Now
            </Button>
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto">
        <div className="max-w-5xl mx-auto p-4 md:p-6 space-y-6">

          <Card>
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
              <div className="flex items-center gap-3">
                <div className={`w-10 h-10 rounded-xl flex items-center justify-center ${isEnabled ? 'bg-accent-primary/10' : 'bg-surface-3'}`}>
                  <Heart className={`w-5 h-5 ${isEnabled ? 'text-accent-primary' : 'text-text-3'}`} />
                </div>
                <div>
                  <h2 className="text-sm font-semibold text-text-0">
                    {isEnabled ? 'Heartbeat Active' : 'Heartbeat Disabled'}
                  </h2>
                  <p className="text-xs text-text-3">
                    {isEnabled ? `Every ${intervalLabel} \u00B7 ${enabledAgents.length} agent(s) \u00B7 ${config?.heartbeat_active_start}\u2013${config?.heartbeat_active_end} ${config?.heartbeat_timezone}` : 'Enable heartbeat to start periodic agent check-ins'}
                  </p>
                </div>
              </div>
              <Toggle enabled={isEnabled} onChange={handleToggleGlobal} label="Enable heartbeat monitoring" />
            </div>
          </Card>

          {showSettings && config && (
            <Card>
              <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-4">Configuration</h3>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <DurationInput
                  key={config.heartbeat_interval_sec}
                  value={config.heartbeat_interval_sec}
                  onSave={(v) => handleConfigSave('heartbeat_interval_sec', v)}
                />
                <Select
                  label="Timezone"
                  value={config.heartbeat_timezone}
                  onChange={e => handleConfigSave('heartbeat_timezone', e.target.value)}
                  options={TIMEZONE_OPTIONS.map(tz => ({ value: tz, label: tz }))}
                />
                <div>
                  <label htmlFor="heartbeat-active-start" className="block text-xs font-medium text-text-2 mb-1.5">Active Start</label>
                  <input
                    id="heartbeat-active-start"
                    type="time"
                    value={config.heartbeat_active_start}
                    onChange={e => handleConfigSave('heartbeat_active_start', e.target.value)}
                    className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-0 text-sm text-text-1"
                  />
                </div>
                <div>
                  <label htmlFor="heartbeat-active-end" className="block text-xs font-medium text-text-2 mb-1.5">Active End</label>
                  <input
                    id="heartbeat-active-end"
                    type="time"
                    value={config.heartbeat_active_end}
                    onChange={e => handleConfigSave('heartbeat_active_end', e.target.value)}
                    className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-0 text-sm text-text-1"
                  />
                </div>
              </div>
            </Card>
          )}

          {activeExecutions.length > 0 && (
            <div>
              <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3 flex items-center gap-2">
                <Loader2 className="w-3.5 h-3.5 animate-spin text-accent-primary" />
                Active
              </h3>
              <div className="space-y-2">
                {activeExecutions.map(exec => {
                  const agent = agents.find(a => a.slug === exec.agent_role_slug);
                  return (
                    <Card key={exec.id}>
                      <div className="flex items-center gap-3">
                        {agent?.avatar_path && (
                          <img src={agent.avatar_path} alt="" className="w-8 h-8 rounded-lg flex-shrink-0" />
                        )}
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium text-text-0">{agent?.name || exec.agent_role_slug}</p>
                          <p className="text-xs text-text-3">Running since {timeAgo(exec.started_at)}</p>
                        </div>
                        <StatusBadge status="running" />
                      </div>
                    </Card>
                  );
                })}
              </div>
            </div>
          )}

          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3 flex items-center gap-2">
              <Heart className="w-3.5 h-3.5" />
              Agents
            </h3>
            {agents.filter(a => a.enabled).length === 0 ? (
              <EmptyState icon={<Heart className="w-8 h-8" />} title="No agents" description="Create agents to enable heartbeat." />
            ) : (
              <div className="space-y-1.5">
                {agents.filter(a => a.enabled).map(agent => (
                  <div key={agent.slug} className="flex items-center gap-3 px-3 py-2.5 rounded-lg bg-surface-1 border border-border-0">
                    {agent.avatar_path && (
                      <img src={agent.avatar_path} alt="" className="w-7 h-7 rounded-lg flex-shrink-0" />
                    )}
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-text-0 truncate">{agent.name}</p>
                      <p className="text-[11px] text-text-3 truncate">{agent.description || agent.slug}</p>
                    </div>
                    <Toggle enabled={agent.heartbeat_enabled} onChange={() => handleToggleAgent(agent.slug, agent.heartbeat_enabled)} label={`Enable heartbeat for ${agent.name}`} />
                  </div>
                ))}
              </div>
            )}
          </div>

          <div>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3 flex items-center gap-2">
                <Clock className="w-3.5 h-3.5" />
                Executions
              </h3>
              <button
                onClick={() => fetchExecutions(page, debouncedSearch)}
                aria-label="Refresh executions"
                className="p-1 rounded text-text-3 hover:text-text-1 transition-colors cursor-pointer"
              >
                <RefreshCw className="w-3.5 h-3.5" />
              </button>
            </div>

            <div className="relative mb-3">
              <Search aria-hidden="true" className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-3 pointer-events-none" />
              <input
                type="text"
                placeholder="Search executions..."
                value={search}
                onChange={e => handleSearchChange(e.target.value)}
                aria-label="Search executions"
                className="w-full pl-9 pr-3 py-2 rounded-lg border border-border-1 bg-surface-0 text-sm text-text-1 placeholder:text-text-3"
              />
            </div>

            <Pagination
              page={page}
              totalPages={totalPages}
              total={totalExecs}
              onPageChange={setPage}
              label="executions"
            />

            {pastExecutions.length === 0 && activeExecutions.length === 0 ? (
              <EmptyState
                icon={<Clock className="w-8 h-8" />}
                title={debouncedSearch ? 'No matching executions' : 'No executions yet'}
                description={debouncedSearch ? 'Try a different search term.' : 'Heartbeat executions will appear here after the first cycle.'}
              />
            ) : (
              <div className="space-y-1.5">
                {pastExecutions.map(exec => {
                  const agent = agents.find(a => a.slug === exec.agent_role_slug);
                  const actions = parseActions(exec.actions_taken);
                  return (
                    <button
                      key={exec.id}
                      onClick={() => setSelectedExec(exec)}
                      className="w-full text-left flex items-start gap-3 px-3 py-2.5 rounded-lg bg-surface-1 border border-border-0 hover:border-border-1 hover:bg-surface-2/50 transition-colors cursor-pointer"
                    >
                      <div className="mt-0.5 flex-shrink-0">
                        {exec.status === 'completed' ? (
                          <CheckCircle className="w-4 h-4 text-green-400" />
                        ) : (
                          <XCircle className="w-4 h-4 text-red-400" />
                        )}
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2 flex-wrap">
                          {agent?.avatar_path && (
                            <img src={agent.avatar_path} alt="" className="w-4 h-4 rounded" />
                          )}
                          <p className="text-xs font-medium text-text-0">{agent?.name || exec.agent_role_slug}</p>
                          <span className="text-[10px] text-text-3">{timeAgo(exec.started_at)}</span>
                          <span className="text-[10px] text-text-3">{formatDuration(exec.started_at, exec.finished_at)}</span>
                        </div>
                        {actions.length > 0 && (
                          <div className="flex flex-wrap gap-1 mt-1">
                            {actions.map((a, i) => (
                              <span key={i} className="text-[10px] px-1.5 py-0.5 rounded bg-surface-2 text-text-2 truncate max-w-[200px]">{a}</span>
                            ))}
                          </div>
                        )}
                        {exec.error && (
                          <p className="text-[10px] text-red-400 mt-1 truncate">{exec.error}</p>
                        )}
                        {exec.cost_usd > 0 && (
                          <p className="text-[10px] text-text-3 mt-0.5">${exec.cost_usd.toFixed(4)} &middot; {exec.input_tokens + exec.output_tokens} tokens</p>
                        )}
                      </div>
                    </button>
                  );
                })}
              </div>
            )}
          </div>

        </div>
      </div>

      {selectedExec && (
        <ExecutionDetailModal
          exec={selectedExec}
          agent={agents.find(a => a.slug === selectedExec.agent_role_slug)}
          onClose={() => setSelectedExec(null)}
        />
      )}
    </div>
  );
}

function ExecutionDetailModal({
  exec,
  agent,
  onClose,
}: {
  exec: HeartbeatExecution;
  agent?: AgentRole;
  onClose: () => void;
}) {
  const actions = parseActions(exec.actions_taken);
  const agentName = agent?.name || exec.agent_role_slug;

  return (
    <Modal open onClose={onClose} title={`Heartbeat: ${agentName}`} size="lg">
      <div className="space-y-5">

        <div className="flex items-center gap-3">
          {agent?.avatar_path && (
            <img src={agent.avatar_path} alt="" className="w-10 h-10 rounded-xl" />
          )}
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-text-0">{agentName}</p>
            <p className="text-xs text-text-3">{exec.agent_role_slug}</p>
          </div>
          <StatusBadge status={exec.status} />
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <DetailStat label="Started" value={formatTimestamp(exec.started_at)} />
          <DetailStat label="Duration" value={formatDuration(exec.started_at, exec.finished_at)} />
          <DetailStat label="Cost" value={exec.cost_usd > 0 ? `$${exec.cost_usd.toFixed(4)}` : '--'} />
          <DetailStat label="Tokens" value={exec.input_tokens + exec.output_tokens > 0 ? `${(exec.input_tokens + exec.output_tokens).toLocaleString()}` : '--'} />
        </div>

        {exec.input_tokens + exec.output_tokens > 0 && (
          <div className="flex gap-4 text-[11px] text-text-3">
            <span>Input: {exec.input_tokens.toLocaleString()}</span>
            <span>Output: {exec.output_tokens.toLocaleString()}</span>
          </div>
        )}

        {actions.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-2">Actions Taken</h4>
            <div className="space-y-1.5">
              {actions.map((action, i) => {
                const [type, ...rest] = action.split(': ');
                const detail = rest.join(': ');
                return (
                  <div key={i} className="flex items-start gap-2 px-3 py-2 rounded-lg bg-surface-2/50 border border-border-0">
                    <span className="text-xs font-mono font-medium text-accent-primary flex-shrink-0">{type}</span>
                    {detail && <span className="text-xs text-text-2 break-all">{detail}</span>}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {exec.output && (
          <div>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-2">Output</h4>
            <pre className="text-xs text-text-1 bg-surface-0 border border-border-0 rounded-lg p-3 whitespace-pre-wrap break-words max-h-60 overflow-y-auto">
              {exec.output}
            </pre>
          </div>
        )}

        {exec.error && (
          <div>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-red-400 mb-2">Error</h4>
            <pre className="text-xs text-red-400 bg-red-500/5 border border-red-500/20 rounded-lg p-3 whitespace-pre-wrap break-words max-h-40 overflow-y-auto">
              {exec.error}
            </pre>
          </div>
        )}

        {exec.finished_at && (
          <p className="text-[11px] text-text-3">
            Finished {formatTimestamp(exec.finished_at)}
          </p>
        )}
      </div>
    </Modal>
  );
}

function DetailStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="px-3 py-2 rounded-lg bg-surface-2/50">
      <p className="text-[10px] uppercase tracking-wider text-text-3 mb-0.5">{label}</p>
      <p className="text-xs font-medium text-text-0 truncate">{value}</p>
    </div>
  );
}
