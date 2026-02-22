import { useState, useEffect } from 'react';
import {
  Clock,
  Plus,
  Play,
  Trash2,
  ChevronRight,
  ArrowLeft,
  Loader2,
  Bot,
  MessageSquare,
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
import { api, type Schedule, type ScheduleExecution, type AgentRole, type ChatThread } from '../lib/api';
import { useToast } from '../components/Toast';

const CRON_PRESETS = [
  { value: '0 */5 * * * *', label: 'Every 5 minutes' },
  { value: '0 */15 * * * *', label: 'Every 15 minutes' },
  { value: '0 */30 * * * *', label: 'Every 30 minutes' },
  { value: '0 0 * * * *', label: 'Every hour' },
  { value: '0 0 */2 * * *', label: 'Every 2 hours' },
  { value: '0 0 */6 * * *', label: 'Every 6 hours' },
  { value: '0 0 9 * * *', label: 'Daily at 9:00 AM' },
  { value: '0 0 9 * * 1-5', label: 'Weekdays at 9:00 AM' },
  { value: '0 0 9 * * 1', label: 'Weekly on Monday at 9:00 AM' },
  { value: '0 0 0 1 * *', label: 'Monthly on the 1st' },
  { value: 'custom', label: 'Custom...' },
];

function ScheduleDetail({ schedule, onBack, getAgentName, getThreadTitle, getCronLabel }: {
  schedule: Schedule;
  onBack: () => void;
  getAgentName: (slug: string) => string;
  getThreadTitle: (id: string) => string;
  getCronLabel: (expr: string) => string;
}) {
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

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <button onClick={onBack} aria-label="Back" className="p-2 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer">
          <ArrowLeft className="w-5 h-5" />
        </button>
        <div className="flex-1">
          <h2 className="text-lg font-semibold text-text-0">{schedule.name}</h2>
          <p className="text-sm text-text-2">{getCronLabel(schedule.cron_expr)}</p>
        </div>
        <Button size="sm" onClick={runNow} icon={<Play className="w-4 h-4" />}>Run Now</Button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Card>
          <p className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-1">Cron</p>
          <p className="text-sm font-mono text-text-1">{schedule.cron_expr}</p>
        </Card>
        <Card>
          <p className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-1">Next Run</p>
          <p className="text-sm text-text-1">{schedule.next_run_at ? new Date(schedule.next_run_at).toLocaleString() : 'Not scheduled'}</p>
        </Card>
        <Card>
          <p className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-1">Agent</p>
          <p className="text-sm text-text-1">{getAgentName(schedule.agent_role_slug)}</p>
        </Card>
      </div>

      {schedule.thread_id && (
        <Card>
          <p className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-1">Chat Thread</p>
          <p className="text-sm text-text-1">{getThreadTitle(schedule.thread_id)}</p>
        </Card>
      )}

      {schedule.prompt_content && (
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
  const [agents, setAgents] = useState<AgentRole[]>([]);
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Schedule | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [view, setView] = useState<ViewMode>('list');
  const [page, setPage] = useState(0);

  const [name, setName] = useState('');
  const [cronPreset, setCronPreset] = useState('0 0 9 * * *');
  const [customCron, setCustomCron] = useState('');
  const [agentSlug, setAgentSlug] = useState('');
  const [threadId, setThreadId] = useState('');
  const [promptContent, setPromptContent] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    try {
      const [schedulesData, agentsData, threadsData] = await Promise.all([
        api.get<Schedule[]>('/schedules'),
        api.get<AgentRole[]>('/agent-roles'),
        api.get<ChatThread[]>('/chat/threads'),
      ]);
      setSchedules(Array.isArray(schedulesData) ? schedulesData : []);
      setAgents(Array.isArray(agentsData) ? agentsData : []);
      setThreads(Array.isArray(threadsData) ? threadsData : []);
    } catch (e) {
      console.warn('loadSchedulerData failed:', e);
      setSchedules([]);
      setAgents([]);
      setThreads([]);
    } finally {
      setLoading(false);
    }
  };

  const resetForm = () => {
    setName('');
    setCronPreset('0 0 9 * * *');
    setCustomCron('');
    setAgentSlug('');
    setThreadId('');
    setPromptContent('');
  };

  const createSchedule = async () => {
    setSaving(true);
    try {
      const cron = cronPreset === 'custom' ? customCron : cronPreset;
      await api.post('/schedules', {
        name,
        cron_expr: cron,
        agent_role_slug: agentSlug,
        prompt_content: promptContent,
        thread_id: threadId,
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

  const agentOptions = [
    { value: '', label: 'Select an agent...' },
    ...agents.filter(a => a.enabled).map(a => ({ value: a.slug, label: a.name })),
  ];

  const threadOptions = [
    { value: '', label: 'Create new chat each run' },
    ...threads.map(t => ({ value: t.id, label: t.title })),
  ];

  const canCreate = name.trim() && agentSlug && promptContent.trim();

  const getAgentName = (slug: string) => agents.find(a => a.slug === slug)?.name || slug;
  const getThreadTitle = (id: string) => threads.find(t => t.id === id)?.title || id;
  const getCronLabel = (expr: string) => CRON_PRESETS.find(p => p.value === expr)?.label || expr;

  const handleSearch = (val: string) => { setSearch(val); setPage(0); };

  const filteredSchedules = schedules.filter(s => {
    if (!search.trim()) return true;
    const term = search.toLowerCase();
    return s.name.toLowerCase().includes(term) || getCronLabel(s.cron_expr).toLowerCase().includes(term);
  });
  const totalPages = Math.max(1, Math.ceil(filteredSchedules.length / PAGE_SIZE));
  const paginatedSchedules = filteredSchedules.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  if (selected) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Scheduler" />
        <div className="flex-1 overflow-y-auto p-4 md:p-6">
          <ScheduleDetail schedule={selected} onBack={() => setSelected(null)} getAgentName={getAgentName} getThreadTitle={getThreadTitle} getCronLabel={getCronLabel} />
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
              <Button onClick={() => setCreateOpen(true)} icon={<Plus className="w-4 h-4" />}>Create Schedule</Button>
            </div>

            {filteredSchedules.length === 0 ? (
              <EmptyState
                icon={<Clock className="w-8 h-8" />}
                title={search ? 'No schedules found' : 'No schedules yet'}
                description={search ? 'Try a different search term.' : 'Create schedules to send prompts to agents on a cron-based schedule.'}
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
                            <Bot className="w-4 h-4 text-accent-primary" />
                            <span className="text-xs text-text-2">{getAgentName(s.agent_role_slug)}</span>
                          </div>
                          <div onClick={(e) => e.stopPropagation()}>
                            <Toggle enabled={s.enabled} onChange={() => toggleSchedule(s)} label="Enable schedule" />
                          </div>
                        </div>
                        <div>
                          <p className="text-sm font-semibold text-text-0">{s.name}</p>
                          <p className="text-xs text-text-3 mt-1">{getCronLabel(s.cron_expr)}</p>
                        </div>
                        {s.thread_id && (
                          <div className="flex items-center gap-1.5 text-xs text-text-3">
                            <MessageSquare className="w-3 h-3" />
                            <span className="truncate">{getThreadTitle(s.thread_id)}</span>
                          </div>
                        )}
                        <div className="flex items-center justify-end gap-1">
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
                            <Bot className="w-4 h-4 text-accent-primary flex-shrink-0" />
                            <div>
                              <p className="text-sm font-medium text-text-0">{s.name}</p>
                              <p className="text-xs text-text-3">{getAgentName(s.agent_role_slug)}</p>
                              <p className="text-xs text-text-2 md:hidden mt-0.5">{getCronLabel(s.cron_expr)}</p>
                            </div>
                          </div>
                        ),
                      },
                      {
                        key: 'cron',
                        header: 'Schedule',
                        hideOnMobile: true,
                        render: (s: Schedule) => (
                          <div>
                            <p className="text-sm text-text-1">{getCronLabel(s.cron_expr)}</p>
                            <p className="text-xs font-mono text-text-3">{s.cron_expr}</p>
                          </div>
                        ),
                      },
                      {
                        key: 'thread',
                        header: 'Chat',
                        hideOnMobile: true,
                        render: (s: Schedule) => (
                          <span className="text-sm text-text-1">
                            {s.thread_id ? (
                              <span className="flex items-center gap-1.5">
                                <MessageSquare className="w-3.5 h-3.5 text-text-3" />
                                {getThreadTitle(s.thread_id)}
                              </span>
                            ) : (
                              <span className="text-text-3">New each run</span>
                            )}
                          </span>
                        ),
                      },
                      {
                        key: 'last',
                        header: 'Last Run',
                        hideOnMobile: true,
                        render: (s: Schedule) => (
                          <span className="text-sm text-text-1">
                            {s.last_run_at ? new Date(s.last_run_at).toLocaleString() : 'Never'}
                          </span>
                        ),
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

          <Select
            label="Agent"
            value={agentSlug}
            onChange={e => setAgentSlug(e.target.value)}
            options={agentOptions}
          />

          <Select
            label="Chat"
            value={threadId}
            onChange={e => setThreadId(e.target.value)}
            options={threadOptions}
          />

          <Select
            label="Schedule"
            value={cronPreset}
            onChange={e => setCronPreset(e.target.value)}
            options={CRON_PRESETS}
          />
          {cronPreset === 'custom' && (
            <div className="space-y-2">
              <Input
                label="Custom Cron Expression"
                value={customCron}
                onChange={e => setCustomCron(e.target.value)}
                placeholder="0 * * * * *"
              />
              <div className="flex flex-wrap gap-1.5">
                {[
                  { label: 'Every 5m', value: '0 */5 * * * *' },
                  { label: 'Every 2h', value: '0 0 */2 * * *' },
                  { label: 'Daily 8am', value: '0 0 8 * * *' },
                  { label: 'Daily 6pm', value: '0 0 18 * * *' },
                  { label: 'Weekdays 9am', value: '0 0 9 * * 1-5' },
                  { label: 'Sat 10am', value: '0 0 10 * * 6' },
                  { label: 'Every 6h', value: '0 0 */6 * * *' },
                  { label: '1st & 15th', value: '0 0 9 1,15 * *' },
                ].map(p => (
                  <button
                    key={p.value}
                    type="button"
                    onClick={() => setCustomCron(p.value)}
                    className={`px-2 py-1 text-xs rounded-md border transition-colors cursor-pointer ${
                      customCron === p.value
                        ? 'border-accent-primary bg-accent-muted/30 text-accent-text'
                        : 'border-border-1 text-text-3 hover:text-text-1 hover:border-border-0'
                    }`}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
              <p className="text-xs text-text-3">
                Format: <code className="px-1 py-0.5 rounded bg-surface-2 text-text-2">sec min hour day month weekday</code>
                {' \u00b7 '}
                <a
                  href="https://crontab.guru/"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-accent-text hover:underline"
                >
                  crontab.guru
                </a>
                {' '}(omit seconds field there)
              </p>
            </div>
          )}

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
