import { useState, useEffect } from 'react';
import {
  Clock,
  Plus,
  Play,
  Trash2,
  ChevronRight,
  ArrowLeft,
  CheckCircle,
  XCircle,
  Loader2,
  Bot,
  Wrench,
} from 'lucide-react';
import { Header } from '../components/Header';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Modal } from '../components/Modal';
import { Input, Select } from '../components/Input';
import { StatusBadge } from '../components/StatusBadge';
import { EmptyState } from '../components/EmptyState';
import { Pagination } from '../components/Pagination';
import { DataTable } from '../components/DataTable';
import { SearchBar } from '../components/SearchBar';
import { ViewToggle, type ViewMode } from '../components/ViewToggle';
import { Toggle } from '../components/Toggle';
import { api, type Schedule, type ScheduleExecution, type Tool, type AgentRole } from '../lib/api';
import { useToast } from '../components/Toast';

const CRON_PRESETS = [
  { value: '0 * * * *', label: 'Every hour' },
  { value: '0 9 * * *', label: 'Daily at 9:00 AM' },
  { value: '0 9 * * 1', label: 'Weekly on Monday at 9:00 AM' },
  { value: '0 0 1 * *', label: 'Monthly on the 1st' },
  { value: '*/15 * * * *', label: 'Every 15 minutes' },
  { value: '*/30 * * * *', label: 'Every 30 minutes' },
  { value: 'custom', label: 'Custom...' },
];

function ScheduleDetail({ schedule, onBack }: { schedule: Schedule; onBack: () => void }) {
  const { toast } = useToast();
  const [executions, setExecutions] = useState<ScheduleExecution[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadExecutions();
  }, [schedule.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadExecutions = async () => {
    try {
      const data = await api.get<ScheduleExecution[]>(`/schedules/${schedule.id}/executions`);
      setExecutions(Array.isArray(data) ? data : []);
    } catch (e) {
      console.warn('loadScheduleExecutions failed:', e);
      setExecutions([]);
    } finally {
      setLoading(false);
    }
  };

  const runNow = async () => {
    try {
      await api.post(`/schedules/${schedule.id}/run-now`);
      toast('success', 'Schedule executed');
      setTimeout(loadExecutions, 1000);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to run schedule');
    }
  };

  const isPrompt = schedule.type === 'prompt';

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <button onClick={onBack} aria-label="Back" className="p-2 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer">
          <ArrowLeft className="w-5 h-5" />
        </button>
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold text-text-0">{schedule.name}</h2>
            <span className={`text-xs px-2 py-0.5 rounded-full ${isPrompt ? 'bg-purple-500/20 text-purple-300' : 'bg-accent-muted text-accent-text'}`}>
              {isPrompt ? 'AI Prompt' : 'Tool Action'}
            </span>
          </div>
          <p className="text-sm text-text-2">{schedule.cron_label || schedule.cron_expr || schedule.cron}</p>
        </div>
        <Button size="sm" onClick={runNow} icon={<Play className="w-4 h-4" />}>Run Now</Button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Card>
          <p className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-1">Cron</p>
          <p className="text-sm font-mono text-text-1">{schedule.cron_expr || schedule.cron}</p>
        </Card>
        <Card>
          <p className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-1">Next Run</p>
          <p className="text-sm text-text-1">{schedule.next_run ? new Date(schedule.next_run).toLocaleString() : 'Not scheduled'}</p>
        </Card>
        <Card>
          <p className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-1">
            {isPrompt ? 'Agent' : 'Tool / Action'}
          </p>
          <p className="text-sm text-text-1">
            {isPrompt ? schedule.agent_role_slug : `${schedule.tool_name || schedule.tool_id} / ${schedule.action}`}
          </p>
        </Card>
      </div>

      {isPrompt && schedule.prompt_content && (
        <Card>
          <p className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-2">Prompt</p>
          <p className="text-sm text-text-1 whitespace-pre-wrap">{schedule.prompt_content}</p>
        </Card>
      )}

      <div>
        <h3 className="text-sm font-semibold text-text-1 mb-3">Execution History</h3>
        <Card padding={false}>
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="w-6 h-6 animate-spin text-accent-primary" />
            </div>
          ) : executions.length === 0 ? (
            <div className="text-center py-8 text-sm text-text-3">
              No executions yet
            </div>
          ) : (
            <DataTable
              columns={[
                {
                  key: 'status',
                  header: 'Status',
                  render: (ex: ScheduleExecution) => <StatusBadge status={ex.status} />,
                },
                {
                  key: 'started',
                  header: 'Started',
                  render: (ex: ScheduleExecution) => (
                    <span className="text-sm text-text-1">{new Date(ex.started_at).toLocaleString()}</span>
                  ),
                },
                {
                  key: 'duration',
                  header: 'Duration',
                  hideOnMobile: true,
                  render: (ex: ScheduleExecution) => {
                    if (!ex.finished_at) return <span className="text-sm text-text-2">Running...</span>;
                    const ms = new Date(ex.finished_at).getTime() - new Date(ex.started_at).getTime();
                    return <span className="text-sm text-text-1">{ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`}</span>;
                  },
                },
                {
                  key: 'output',
                  header: 'Output',
                  hideOnMobile: true,
                  render: (ex: ScheduleExecution) => (
                    <span className="text-sm text-text-2 truncate max-w-xs block">{ex.output || ex.error || '--'}</span>
                  ),
                },
              ]}
              data={executions}
              keyExtractor={ex => ex.id}
            />
          )}
        </Card>
      </div>
    </div>
  );
}

const PAGE_SIZE = 12;

export function Scheduler() {
  const { toast } = useToast();
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [tools, setTools] = useState<Tool[]>([]);
  const [agents, setAgents] = useState<AgentRole[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Schedule | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [view, setView] = useState<ViewMode>('list');
  const [page, setPage] = useState(0);

  const [name, setName] = useState('');
  const [schedType, setSchedType] = useState<'tool_action' | 'prompt'>('tool_action');
  const [cronPreset, setCronPreset] = useState('0 9 * * *');
  const [customCron, setCustomCron] = useState('');
  const [toolId, setToolId] = useState('');
  const [action, setAction] = useState('');
  const [agentSlug, setAgentSlug] = useState('');
  const [promptContent, setPromptContent] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    try {
      const [schedulesData, toolsData, agentsData] = await Promise.all([
        api.get<Schedule[]>('/schedules'),
        api.get<Tool[]>('/tools'),
        api.get<AgentRole[]>('/agent-roles'),
      ]);
      setSchedules(Array.isArray(schedulesData) ? schedulesData : []);
      setTools(Array.isArray(toolsData) ? toolsData : []);
      setAgents(Array.isArray(agentsData) ? agentsData : []);
    } catch (e) {
      console.warn('loadSchedulerData failed:', e);
      setSchedules([]);
      setTools([]);
      setAgents([]);
    } finally {
      setLoading(false);
    }
  };

  const resetForm = () => {
    setName('');
    setSchedType('tool_action');
    setCronPreset('0 9 * * *');
    setCustomCron('');
    setToolId('');
    setAction('');
    setAgentSlug('');
    setPromptContent('');
  };

  const createSchedule = async () => {
    setSaving(true);
    try {
      const cron = cronPreset === 'custom' ? customCron : cronPreset;
      await api.post('/schedules', {
        name,
        cron_expr: cron,
        type: schedType,
        tool_id: schedType === 'tool_action' ? toolId : '',
        action: schedType === 'tool_action' ? action : '',
        agent_role_slug: schedType === 'prompt' ? agentSlug : '',
        prompt_content: schedType === 'prompt' ? promptContent : '',
      });
      toast('success', 'Schedule created');
      setCreateOpen(false);
      resetForm();
      loadData();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to create schedule');
    } finally {
      setSaving(false);
    }
  };

  const toggleSchedule = async (schedule: Schedule) => {
    try {
      await api.post(`/schedules/${schedule.id}/toggle`);
      toast('success', `Schedule ${!schedule.enabled ? 'enabled' : 'paused'}`);
      loadData();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to update schedule');
    }
  };

  const runNow = async (id: string) => {
    try {
      await api.post(`/schedules/${id}/run-now`);
      toast('success', 'Schedule triggered');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to run schedule');
    }
  };

  const deleteSchedule = async (id: string) => {
    try {
      await api.delete(`/schedules/${id}`);
      toast('success', 'Schedule deleted');
      loadData();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to delete schedule');
    }
  };

  const toolOptions = [
    { value: '', label: 'Select a tool...' },
    ...tools.map(t => ({ value: t.id, label: t.name })),
  ];

  const selectedTool = tools.find(t => t.id === toolId);
  const actionOptions = [
    { value: '', label: 'Select an action...' },
    ...(selectedTool?.actions?.map(a => ({ value: a.name, label: a.name })) || []),
  ];

  const agentOptions = [
    { value: '', label: 'Select an agent...' },
    ...agents.filter(a => a.enabled).map(a => ({ value: a.slug, label: a.name })),
  ];

  const canCreate = name.trim() && (
    (schedType === 'tool_action' && toolId) ||
    (schedType === 'prompt' && agentSlug && promptContent.trim())
  );

  const handleSearch = (val: string) => { setSearch(val); setPage(0); };

  const filteredSchedules = schedules.filter(s => {
    if (!search.trim()) return true;
    const term = search.toLowerCase();
    return s.name.toLowerCase().includes(term) || (s.cron_label || '').toLowerCase().includes(term);
  });
  const totalPages = Math.max(1, Math.ceil(filteredSchedules.length / PAGE_SIZE));
  const paginatedSchedules = filteredSchedules.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  if (selected) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Scheduler" />
        <div className="flex-1 overflow-y-auto p-4 md:p-6">
          <ScheduleDetail schedule={selected} onBack={() => setSelected(null)} />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header title="Scheduler" />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        {loading ? (
          <div className="flex items-center justify-center py-16">
            <Loader2 className="w-8 h-8 animate-spin text-accent-primary" />
          </div>
        ) : (
          <>
            <div className="flex items-center gap-3 mb-4">
              <SearchBar value={search} onChange={handleSearch} placeholder="Search schedules..." className="flex-1" />
              <ViewToggle view={view} onViewChange={setView} />
              <Button size="sm" onClick={() => setCreateOpen(true)} icon={<Plus className="w-4 h-4" />}>Create Schedule</Button>
            </div>

            {filteredSchedules.length === 0 ? (
              <EmptyState
                icon={<Clock className="w-8 h-8" />}
                title={search ? 'No schedules found' : 'No schedules yet'}
                description={search ? 'Try a different search term.' : 'Create schedules to run tools or send prompts to agents on a cron-based schedule.'}
              />
            ) : view === 'grid' ? (
              <>
                <Pagination page={page} totalPages={totalPages} total={filteredSchedules.length} onPageChange={setPage} label="schedules" />
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                  {paginatedSchedules.map(s => (
                    <Card key={s.id} hover onClick={() => setSelected(s)}>
                      <div className="flex flex-col gap-3">
                        <div className="flex items-start justify-between">
                          <div className="flex items-center gap-2">
                            {s.type === 'prompt' ? (
                              <Bot className="w-4 h-4 text-purple-400" />
                            ) : (
                              <Wrench className="w-4 h-4 text-accent-primary" />
                            )}
                            <span className={`text-xs px-2 py-0.5 rounded-full ${s.type === 'prompt' ? 'bg-purple-500/20 text-purple-300' : 'bg-accent-muted text-accent-text'}`}>
                              {s.type === 'prompt' ? 'AI Prompt' : 'Tool Action'}
                            </span>
                          </div>
                          <div onClick={(e) => e.stopPropagation()}>
                            <Toggle enabled={s.enabled} onChange={() => toggleSchedule(s)} label="Enable schedule" />
                          </div>
                        </div>
                        <div>
                          <p className="text-sm font-semibold text-text-0">{s.name}</p>
                          <p className="text-xs text-text-3 mt-1">{s.cron_label || s.cron_expr || s.cron}</p>
                        </div>
                        <div className="flex items-center justify-between text-xs text-text-3">
                          <span>{s.type === 'prompt' ? s.agent_role_slug : (s.tool_name || s.tool_id)}</span>
                          <div className="flex items-center gap-1">
                            <button
                              onClick={(e) => { e.stopPropagation(); runNow(s.id); }}
                              className="p-1 rounded text-text-3 hover:text-accent-text transition-colors cursor-pointer"
                              title="Run now"
                              aria-label="Run now"
                            >
                              <Play className="w-3.5 h-3.5" />
                            </button>
                            <button
                              onClick={(e) => { e.stopPropagation(); deleteSchedule(s.id); }}
                              className="p-1 rounded text-text-3 hover:text-red-400 transition-colors cursor-pointer"
                              title="Delete"
                              aria-label="Delete schedule"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        </div>
                      </div>
                    </Card>
                  ))}
                </div>
              </>
            ) : (
              <>
                <Pagination page={page} totalPages={totalPages} total={filteredSchedules.length} onPageChange={setPage} label="schedules" />
                <Card padding={false}>
                  <DataTable
                    columns={[
                      {
                        key: 'name',
                        header: 'Name',
                        render: (s: Schedule) => (
                          <div className="flex items-center gap-2">
                            {s.type === 'prompt' ? (
                              <Bot className="w-4 h-4 text-purple-400 flex-shrink-0" />
                            ) : (
                              <Wrench className="w-4 h-4 text-accent-primary flex-shrink-0" />
                            )}
                            <div>
                              <p className="text-sm font-medium text-text-0">{s.name}</p>
                              <p className="text-xs text-text-3">
                                {s.type === 'prompt' ? s.agent_role_slug : (s.tool_name || s.tool_id)}
                              </p>
                              <p className="text-xs text-text-2 md:hidden mt-0.5">{s.cron_label || s.cron_expr || s.cron}</p>
                            </div>
                          </div>
                        ),
                      },
                      {
                        key: 'type',
                        header: 'Type',
                        hideOnMobile: true,
                        render: (s: Schedule) => (
                          <span className={`text-xs px-2 py-0.5 rounded-full ${s.type === 'prompt' ? 'bg-purple-500/20 text-purple-300' : 'bg-accent-muted text-accent-text'}`}>
                            {s.type === 'prompt' ? 'AI Prompt' : 'Tool Action'}
                          </span>
                        ),
                      },
                      {
                        key: 'cron',
                        header: 'Schedule',
                        hideOnMobile: true,
                        render: (s: Schedule) => (
                          <div>
                            <p className="text-sm text-text-1">{s.cron_label || s.cron_expr || s.cron}</p>
                            <p className="text-xs font-mono text-text-3">{s.cron_expr || s.cron}</p>
                          </div>
                        ),
                      },
                      {
                        key: 'last',
                        header: 'Last Run',
                        hideOnMobile: true,
                        render: (s: Schedule) => {
                          const lastRun = s.last_run || s.last_run_at;
                          return (
                            <div className="flex items-center gap-1.5">
                              {s.last_status === 'success' && <CheckCircle className="w-3.5 h-3.5 text-emerald-400" />}
                              {s.last_status === 'error' && <XCircle className="w-3.5 h-3.5 text-red-400" />}
                              <span className="text-sm text-text-1">
                                {lastRun ? new Date(lastRun).toLocaleString() : 'Never'}
                              </span>
                            </div>
                          );
                        },
                      },
                      {
                        key: 'status',
                        header: 'Status',
                        render: (s: Schedule) => (
                          <div onClick={(e) => e.stopPropagation()}>
                            <Toggle enabled={s.enabled} onChange={() => toggleSchedule(s)} label="Enable schedule" />
                          </div>
                        ),
                      },
                      {
                        key: 'actions',
                        header: '',
                        className: 'text-right',
                        render: (s: Schedule) => (
                          <div className="flex items-center justify-end gap-1">
                            <button
                              onClick={(e) => { e.stopPropagation(); runNow(s.id); }}
                              className="p-1.5 rounded-lg text-text-2 hover:text-accent-text hover:bg-surface-2 transition-colors cursor-pointer"
                              title="Run now"
                              aria-label="Run now"
                            >
                              <Play className="w-4 h-4" />
                            </button>
                            <button
                              onClick={(e) => { e.stopPropagation(); deleteSchedule(s.id); }}
                              className="hidden sm:block p-1.5 rounded-lg text-text-2 hover:text-red-400 hover:bg-surface-2 transition-colors cursor-pointer"
                              title="Delete"
                              aria-label="Delete schedule"
                            >
                              <Trash2 className="w-4 h-4" />
                            </button>
                            <button
                              onClick={(e) => { e.stopPropagation(); setSelected(s); }}
                              className="p-1.5 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                              title="View details"
                              aria-label="View details"
                            >
                              <ChevronRight className="w-4 h-4" />
                            </button>
                          </div>
                        ),
                      },
                    ]}
                    data={paginatedSchedules}
                    keyExtractor={s => s.id}
                    onRowClick={(s) => setSelected(s)}
                  />
                </Card>
              </>
            )}
          </>
        )}
      </div>

      <Modal open={createOpen} onClose={() => { setCreateOpen(false); resetForm(); }} title="Create Schedule" size="lg">
        <div className="space-y-4">
          <Input
            label="Name"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder="e.g. Daily report"
          />

          <div>
            <label className="block text-sm font-medium text-text-1 mb-2">Type</label>
            <div className="flex gap-2">
              <button
                onClick={() => setSchedType('tool_action')}
                aria-pressed={schedType === 'tool_action'}
                className={`flex-1 flex items-center gap-2 p-3 rounded-lg border transition-colors cursor-pointer ${
                  schedType === 'tool_action'
                    ? 'border-accent-primary bg-accent-muted/30 text-text-0'
                    : 'border-border-1 text-text-2 hover:border-border-0'
                }`}
              >
                <Wrench className="w-4 h-4" />
                <span className="text-sm font-medium">Tool Action</span>
              </button>
              <button
                onClick={() => setSchedType('prompt')}
                aria-pressed={schedType === 'prompt'}
                className={`flex-1 flex items-center gap-2 p-3 rounded-lg border transition-colors cursor-pointer ${
                  schedType === 'prompt'
                    ? 'border-purple-500 bg-purple-500/10 text-text-0'
                    : 'border-border-1 text-text-2 hover:border-border-0'
                }`}
              >
                <Bot className="w-4 h-4" />
                <span className="text-sm font-medium">AI Prompt</span>
              </button>
            </div>
          </div>

          <Select
            label="Schedule Preset"
            value={cronPreset}
            onChange={e => setCronPreset(e.target.value)}
            options={CRON_PRESETS}
          />
          {cronPreset === 'custom' && (
            <Input
              label="Custom Cron Expression"
              value={customCron}
              onChange={e => setCustomCron(e.target.value)}
              placeholder="* * * * *"
            />
          )}

          {schedType === 'tool_action' ? (
            <>
              <Select
                label="Tool"
                value={toolId}
                onChange={e => { setToolId(e.target.value); setAction(''); }}
                options={toolOptions}
              />
              {toolId && (
                <Select
                  label="Action"
                  value={action}
                  onChange={e => setAction(e.target.value)}
                  options={actionOptions}
                />
              )}
            </>
          ) : (
            <>
              <Select
                label="Agent"
                value={agentSlug}
                onChange={e => setAgentSlug(e.target.value)}
                options={agentOptions}
              />
              <div>
                <label className="block text-sm font-medium text-text-1 mb-1.5">Prompt</label>
                <textarea
                  value={promptContent}
                  onChange={e => setPromptContent(e.target.value)}
                  placeholder="Enter the prompt to send to the agent..."
                  rows={4}
                  className="w-full px-3 py-2 rounded-lg text-sm bg-surface-2 border border-border-1 text-text-0 placeholder:text-text-3 focus:outline-none focus:ring-1 focus:ring-accent-primary resize-none"
                />
              </div>
            </>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="secondary" onClick={() => { setCreateOpen(false); resetForm(); }}>Cancel</Button>
            <Button onClick={createSchedule} loading={saving} disabled={!canCreate}>
              Create Schedule
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
