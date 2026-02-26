import { useState, useEffect, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router';
import { ArrowLeft, Save, Upload, Sparkles, Plus, Trash2, BookOpen, ArrowUpFromLine, Wrench, Search, FolderOpen, GripVertical, Clock, AlertCircle, CheckCircle2, Circle, ChevronRight } from 'lucide-react';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Input } from '../components/Input';
import { Modal } from '../components/Modal';
import { LoadingSpinner } from '../components/LoadingSpinner';
import { useToast } from '../components/Toast';
import { Toggle } from '../components/Toggle';
import { FolderAssign } from '../components/FolderAssign';
import { api, agentFiles, agentMemories, agentSkills, agentTasks, skills as skillsApi, type AgentRole, type Skill, type MemoryItem, type Tool, type AgentTask, type AgentTaskStatus } from '../lib/api';

interface AgentTool extends Tool {
  access_type: 'owned' | 'granted';
}

const PRESET_AVATARS = Array.from({ length: 45 }, (_, i) => `/avatars/avatar-${i + 1}.webp`);

interface FileTab {
  key: string;
  label: string;
  filename: string;
  description: string;
}

const FILE_TABS: FileTab[] = [
  { key: 'soul', label: 'Soul', filename: 'SOUL.md', description: 'Persona, tone, values, name — the core of who this agent is.' },
  { key: 'user', label: 'User', filename: 'USER.md', description: 'Human profile and preferences the agent learns over time.' },
  { key: 'runbook', label: 'Runbook', filename: 'RUNBOOK.md', description: 'Self-managed operational playbook — lessons learned, process notes, session rules.' },
  { key: 'boot', label: 'Boot', filename: 'BOOT.md', description: 'Startup instructions — run at the beginning of each session.' },
  { key: 'heartbeat', label: 'Heartbeat', filename: 'HEARTBEAT.md', description: 'Periodic checklist. Empty = skip.' },
];

type TopTab = 'about' | 'soul' | 'user' | 'runbook' | 'boot' | 'heartbeat' | 'memory' | 'skills' | 'tools' | 'work';

const STATUS_COLUMNS: { key: AgentTaskStatus; label: string; color: string; bg: string; border: string; icon: typeof Circle }[] = [
  { key: 'backlog', label: 'Backlog', color: 'text-text-3', bg: 'bg-surface-2/60', border: 'border-border-0', icon: Circle },
  { key: 'doing', label: 'In Progress', color: 'text-blue-400', bg: 'bg-blue-500/5', border: 'border-blue-500/20', icon: Clock },
  { key: 'blocked', label: 'Blocked', color: 'text-red-400', bg: 'bg-red-500/5', border: 'border-red-500/20', icon: AlertCircle },
  { key: 'done', label: 'Done', color: 'text-green-400', bg: 'bg-green-500/5', border: 'border-green-500/20', icon: CheckCircle2 },
];

function timeAgo(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diff = Math.max(0, now - then);
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h`;
  const days = Math.floor(hrs / 24);
  return `${days}d`;
}

export function AgentEdit() {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  const { toast } = useToast();
  const promptRef = useRef<HTMLTextAreaElement>(null);

  const [role, setRole] = useState<AgentRole | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [initializing, setInitializing] = useState(false);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [model, setModel] = useState('anthropic/claude-sonnet-4-6');
  const [systemPrompt, setSystemPrompt] = useState('');
  const [folder, setFolder] = useState('');
  const [allFolders, setAllFolders] = useState<string[]>([]);
  const [availableModels, setAvailableModels] = useState<{ id: string; name: string }[]>([]);
  const [modelSearch, setModelSearch] = useState('');
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [avatarPath, setAvatarPath] = useState('');

  // Top-level tab
  const [activeTab, setActiveTab] = useState<TopTab>('about');

  // Identity files state
  const [files, setFiles] = useState<Record<string, string>>({});
  const [fileDirty, setFileDirty] = useState<Record<string, boolean>>({});
  const [fileSaving, setFileSaving] = useState(false);

  // Memory state
  const [memories, setMemories] = useState<MemoryItem[]>([]);
  const [memoryStats, setMemoryStats] = useState<{ total_active: number; total_archived: number; categories: Record<string, number> } | null>(null);
  const [expandedMemory, setExpandedMemory] = useState<string | null>(null);

  // Skills state
  const [agentSkillList, setAgentSkillList] = useState<Skill[]>([]);
  const [globalSkillList, setGlobalSkillList] = useState<Skill[]>([]);
  const [showSkillPicker, setShowSkillPicker] = useState(false);

  // Tools state
  const [agentTools, setAgentTools] = useState<AgentTool[]>([]);
  const [allTools, setAllTools] = useState<Tool[]>([]);
  const [showToolPicker, setShowToolPicker] = useState(false);

  // Tasks (Kanban) state
  const [tasks, setTasks] = useState<AgentTask[]>([]);
  const [showCreateTask, setShowCreateTask] = useState(false);
  const [createTaskTitle, setCreateTaskTitle] = useState('');
  const [createTaskDesc, setCreateTaskDesc] = useState('');
  const [createTaskStatus, setCreateTaskStatus] = useState<AgentTaskStatus>('backlog');
  const [addingTask, setAddingTask] = useState(false);
  const [viewingTask, setViewingTask] = useState<AgentTask | null>(null);
  const [editTitle, setEditTitle] = useState('');
  const [editDesc, setEditDesc] = useState('');
  const [editSaving, setEditSaving] = useState(false);

  // Delete state
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const loadFiles = useCallback(async (s: string) => {
    try {
      const data = await agentFiles.getAll(s);
      setFiles(data);
      setFileDirty({});
    } catch (e) { console.warn('loadFiles failed:', e); }
  }, []);

  const loadMemory = useCallback(async (s: string) => {
    try {
      const [mems, stats] = await Promise.all([
        agentMemories.list(s),
        agentMemories.stats(s),
      ]);
      setMemories(mems || []);
      setMemoryStats(stats);
    } catch (e) { console.warn('loadMemory failed:', e); }
  }, []);

  const loadSkills = useCallback(async (s: string) => {
    try {
      const [agent, global] = await Promise.all([
        agentSkills.list(s),
        skillsApi.list(),
      ]);
      setAgentSkillList(agent || []);
      setGlobalSkillList(global || []);
    } catch (e) { console.warn('loadSkills failed:', e); }
  }, []);

  const loadTools = useCallback(async (s: string) => {
    try {
      const [agentT, allT] = await Promise.all([
        api.get<AgentTool[]>(`/agent-roles/${s}/tools`),
        api.get<Tool[]>('/tools'),
      ]);
      setAgentTools(agentT || []);
      setAllTools(allT || []);
    } catch (e) { console.warn('loadTools failed:', e); }
  }, []);

  const loadTasks = useCallback(async (s: string) => {
    try {
      const data = await agentTasks.list(s);
      setTasks(data || []);
    } catch (e) { console.warn('loadTasks failed:', e); }
  }, []);

  useEffect(() => {
    api.get<{ id: string; name: string }[]>('/settings/available-models')
      .then(setAvailableModels)
      .catch((e) => { console.warn('loadAvailableModels failed:', e); });
  }, []);

  useEffect(() => {
    if (!slug) return;
    api.get<AgentRole[]>('/agent-roles').then(roles => {
      const folders = [...new Set(roles.map(r => r.folder).filter(Boolean))].sort();
      setAllFolders(folders);
    }).catch(() => {});
    api.get<AgentRole>(`/agent-roles/${slug}`)
      .then(data => {
        setRole(data);
        setName(data.name);
        setDescription(data.description);
        setModel(data.model);
        setSystemPrompt(data.system_prompt);
        setAvatarPath(data.avatar_path);
        setFolder(data.folder || '');

        if (data.identity_initialized) {
          loadFiles(slug);
          loadMemory(slug);
          loadSkills(slug);
          loadTools(slug);
        }
        loadTasks(slug);
      })
      .catch(() => {
        toast('error', 'Agent not found');
        navigate('/agents');
      })
      .finally(() => setLoading(false));
  }, [slug, toast, navigate, loadFiles, loadMemory, loadSkills, loadTools, loadTasks]);

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
      toast('error', 'Please upload a PNG, JPEG, or WebP image');
      return;
    }
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append('avatar', file);
      const csrfHeaders: Record<string, string> = {};
      const csrf = (await import('../lib/api')).getCSRFToken();
      if (csrf) csrfHeaders['X-CSRF-Token'] = csrf;
      const res = await fetch('/api/v1/agent-roles/upload-avatar', {
        method: 'POST',
        headers: csrfHeaders,
        body: formData,
        credentials: 'same-origin',
      });
      if (!res.ok) throw new Error('Upload failed');
      const data = await res.json();
      setAvatarPath(data.avatar_path);
      toast('success', 'Avatar uploaded');
    } catch (e) {
      console.warn('uploadAgentAvatar failed:', e);
      toast('error', 'Failed to upload avatar');
    } finally {
      setUploading(false);
    }
  };

  const handleFolderChange = async (newFolder: string) => {
    if (!slug) return;
    setFolder(newFolder);
    try {
      await api.put<AgentRole>(`/agent-roles/${slug}`, { folder: newFolder });
      if (newFolder && !allFolders.includes(newFolder)) {
        setAllFolders(prev => [...prev, newFolder].sort());
      }
    } catch {
      toast('error', 'Failed to update folder');
    }
  };

  const handleSave = async () => {
    if (!slug || !name.trim()) {
      toast('error', 'Name is required');
      return;
    }
    setSaving(true);
    try {
      const updated = await api.put<AgentRole>(`/agent-roles/${slug}`, {
        name: name.trim(),
        description: description.trim(),
        system_prompt: systemPrompt,
        model,
        avatar_path: avatarPath,
        folder,
      });
      setRole(updated);
      toast('success', 'Agent saved');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const handleInitIdentity = async () => {
    if (!slug) return;
    setInitializing(true);
    try {
      await agentFiles.init(slug);
      const updated = await api.get<AgentRole>(`/agent-roles/${slug}`);
      setRole(updated);
      await loadFiles(slug);
      await loadMemory(slug);
      await loadSkills(slug);
      toast('success', 'Identity system initialized');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to initialize');
    } finally {
      setInitializing(false);
    }
  };

  const handleFileChange = (filename: string, content: string) => {
    setFiles(prev => ({ ...prev, [filename]: content }));
    setFileDirty(prev => ({ ...prev, [filename]: true }));
  };

  const handleFileSave = async (filename: string) => {
    if (!slug) return;
    setFileSaving(true);
    try {
      await agentFiles.update(slug, filename, files[filename] || '');
      setFileDirty(prev => ({ ...prev, [filename]: false }));
      toast('success', `${filename} saved`);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to save file');
    } finally {
      setFileSaving(false);
    }
  };

  const handleMemoryDelete = async (memoryId: string) => {
    if (!slug) return;
    try {
      await agentMemories.delete(slug, memoryId);
      setMemories(prev => prev.filter(m => m.id !== memoryId));
      if (memoryStats) {
        setMemoryStats({ ...memoryStats, total_active: memoryStats.total_active - 1 });
      }
      toast('success', 'Memory deleted');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to delete memory');
    }
  };

  const handleAddSkill = async (skillName: string) => {
    if (!slug) return;
    try {
      await agentSkills.add(slug, skillName);
      await loadSkills(slug);
      setShowSkillPicker(false);
      toast('success', `Skill "${skillName}" added`);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to add skill');
    }
  };

  const handleRemoveSkill = async (skillName: string) => {
    if (!slug) return;
    try {
      await agentSkills.remove(slug, skillName);
      setAgentSkillList(prev => prev.filter(s => s.name !== skillName));
      toast('success', `Skill "${skillName}" removed`);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to remove skill');
    }
  };

  const handleGrantTool = async (toolId: string) => {
    if (!slug) return;
    try {
      await api.post(`/agent-roles/${slug}/tools/${toolId}/grant`);
      await loadTools(slug);
      setShowToolPicker(false);
      toast('success', 'Tool access granted');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to grant access');
    }
  };

  const handleRevokeTool = async (toolId: string) => {
    if (!slug) return;
    try {
      await api.delete(`/agent-roles/${slug}/tools/${toolId}/revoke`);
      setAgentTools(prev => prev.filter(t => !(t.id === toolId && t.access_type === 'granted')));
      toast('success', 'Tool access revoked');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to revoke access');
    }
  };

  const handlePublishSkill = async (skillName: string) => {
    if (!slug) return;
    try {
      await agentSkills.publish(slug, skillName);
      toast('success', `Skill "${skillName}" published to global`);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to publish skill');
    }
  };

  const handleDelete = async () => {
    if (!slug) return;
    setDeleting(true);
    try {
      await api.delete(`/agent-roles/${slug}`);
      toast('success', 'Agent deleted');
      navigate('/agents');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to delete agent');
    } finally {
      setDeleting(false);
      setShowDeleteConfirm(false);
    }
  };

  const handlePromptKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      const ta = promptRef.current;
      if (!ta) return;
      const start = ta.selectionStart;
      const end = ta.selectionEnd;
      const fileTab = FILE_TABS.find(t => t.key === activeTab);
      const setter = !fileTab ? setSystemPrompt : (v: string) => {
        handleFileChange(fileTab.filename, v);
      };
      const currentVal = !fileTab ? systemPrompt : (files[fileTab.filename] || '');
      setter(currentVal.substring(0, start) + '  ' + currentVal.substring(end));
      requestAnimationFrame(() => {
        ta.selectionStart = ta.selectionEnd = start + 2;
      });
    }
  };

  // Task handlers
  const handleCreateTask = async () => {
    if (!slug || !createTaskTitle.trim()) return;
    setAddingTask(true);
    try {
      const task = await agentTasks.create(slug, {
        title: createTaskTitle.trim(),
        description: createTaskDesc.trim(),
        status: createTaskStatus,
      });
      setTasks(prev => [...prev, task]);
      setCreateTaskTitle('');
      setCreateTaskDesc('');
      setCreateTaskStatus('backlog');
      setShowCreateTask(false);
      toast('success', 'Task created');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to create task');
    } finally {
      setAddingTask(false);
    }
  };

  const handleMoveTask = async (taskId: string, newStatus: AgentTaskStatus) => {
    if (!slug) return;
    const prev = tasks;
    setTasks(t => t.map(item => item.id === taskId ? { ...item, status: newStatus } : item));
    if (viewingTask?.id === taskId) setViewingTask(v => v ? { ...v, status: newStatus } : v);
    try {
      await agentTasks.update(slug, taskId, { status: newStatus });
    } catch (err) {
      setTasks(prev);
      toast('error', err instanceof Error ? err.message : 'Failed to move task');
    }
  };

  const handleSaveTask = async () => {
    if (!slug || !viewingTask || !editTitle.trim()) return;
    setEditSaving(true);
    try {
      const updated = await agentTasks.update(slug, viewingTask.id, {
        title: editTitle.trim(),
        description: editDesc.trim(),
      });
      setTasks(prev => prev.map(t => t.id === updated.id ? updated : t));
      setViewingTask(updated);
      toast('success', 'Task updated');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to update task');
    } finally {
      setEditSaving(false);
    }
  };

  const handleDeleteTask = async (taskId: string) => {
    if (!slug) return;
    setTasks(prev => prev.filter(t => t.id !== taskId));
    if (viewingTask?.id === taskId) setViewingTask(null);
    try {
      await agentTasks.delete(slug, taskId);
    } catch (err) {
      if (slug) loadTasks(slug);
      toast('error', err instanceof Error ? err.message : 'Failed to delete task');
    }
  };

  const handleClearDone = async () => {
    if (!slug) return;
    try {
      await agentTasks.clearDone(slug);
      setTasks(prev => prev.filter(t => t.status !== 'done'));
      toast('success', 'Done tasks cleared');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to clear done tasks');
    }
  };

  const openTaskDetail = (task: AgentTask) => {
    setViewingTask(task);
    setEditTitle(task.title);
    setEditDesc(task.description);
  };

  const hasChanges = role && (
    name !== role.name ||
    description !== role.description ||
    model !== role.model ||
    systemPrompt !== role.system_prompt ||
    avatarPath !== role.avatar_path ||
    folder !== (role.folder || '')
  );

  if (loading) {
    return (
      <div className="flex flex-col h-full items-center justify-center">
        <LoadingSpinner message="Loading agent..." />
      </div>
    );
  }

  if (!role) return null;

  const isInitialized = role.identity_initialized;
  const fileTabForActive = FILE_TABS.find(t => t.key === activeTab);
  const currentFileContent = fileTabForActive ? (files[fileTabForActive.filename] || '') : '';
  const currentLineCount = currentFileContent.split('\n').length;
  const installedSkillNames = new Set(agentSkillList.map(s => s.name));
  const availableGlobalSkills = globalSkillList.filter(s => !installedSkillNames.has(s.name));

  const requiresInit = !isInitialized && !['about', 'work'].includes(activeTab);
  const activeTaskCount = tasks.filter(t => t.status !== 'done').length;

  const tabs: { key: TopTab; label: string; badge?: number }[] = [
    { key: 'about', label: 'About' },
    { key: 'soul', label: 'Soul' },
    { key: 'user', label: 'User' },
    { key: 'runbook', label: 'Runbook' },
    { key: 'boot', label: 'Boot' },
    { key: 'heartbeat', label: 'Heartbeat' },
    { key: 'memory', label: 'Memory', badge: memories.length || undefined },
    { key: 'skills', label: 'Skills', badge: agentSkillList.length || undefined },
    { key: 'tools', label: 'Tools' },
    { key: 'work', label: 'Work', badge: activeTaskCount || undefined },
  ];

  return (
    <div className="flex flex-col h-full">
      {/* Top bar */}
      <div className="h-14 md:h-16 flex items-center justify-between px-4 md:px-6 border-b border-border-0 bg-surface-1/50 backdrop-blur-sm flex-shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          <button
            onClick={() => navigate('/agents')}
            className="p-2 -ml-2 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
          >
            <ArrowLeft className="w-5 h-5" />
          </button>
          <div className="flex items-center gap-2.5 min-w-0">
            <img src={avatarPath} alt="" className="w-7 h-7 rounded-lg flex-shrink-0" />
            <div className="min-w-0">
              <h1 className="text-sm md:text-base font-semibold text-text-0 truncate">{name || 'Untitled'}</h1>
              <p className="text-[11px] text-text-3 truncate">Edit Agent</p>
            </div>
          </div>
          {role.is_preset && (
            <span className="text-[10px] px-2 py-0.5 rounded-full bg-surface-3 text-text-3 border border-border-1 flex-shrink-0">
              Preset
            </span>
          )}
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <Button variant="ghost" size="sm" onClick={() => navigate('/agents')}>
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            loading={saving}
            disabled={!hasChanges || !name.trim()}
            icon={<Save className="w-3.5 h-3.5" />}
            aria-label={hasChanges ? 'Save (unsaved changes)' : 'Save'}
          >
            Save
          </Button>
        </div>
      </div>

      {/* Tab bar */}
      <div className="flex items-center gap-1 px-4 md:px-6 border-b border-border-0 overflow-x-auto flex-shrink-0">
        {tabs.map(tab => (
          <button
            key={tab.key}
            onClick={() => {
              setActiveTab(tab.key);
              if (tab.key === 'memory' && slug) loadMemory(slug);
              if (tab.key === 'tools' && slug) loadTools(slug);
              if (tab.key === 'work' && slug) loadTasks(slug);
            }}
            className={`px-3 py-2.5 text-sm font-medium transition-colors relative cursor-pointer whitespace-nowrap ${
              activeTab === tab.key ? 'text-text-0' : 'text-text-3 hover:text-text-1'
            }`}
          >
            <span className="flex items-center gap-1.5">
              {tab.label}
              {fileDirty[FILE_TABS.find(f => f.key === tab.key)?.filename || ''] && (
                <span className="w-1.5 h-1.5 rounded-full bg-accent-primary" />
              )}
              {tab.badge !== undefined && (
                <span className={`px-1.5 py-0.5 rounded-full text-[10px] font-semibold leading-none ${
                  activeTab === tab.key
                    ? 'bg-accent-primary/15 text-accent-primary'
                    : 'bg-surface-3 text-text-3'
                }`}>
                  {tab.badge}
                </span>
              )}
            </span>
            {activeTab === tab.key && (
              <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-accent-primary rounded-t" />
            )}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto">
        {requiresInit ? (
          <div className="p-4 md:p-6 max-w-2xl">
            <Card>
              <div className="flex flex-col items-center gap-4 py-8">
                <Sparkles className="w-8 h-8 text-accent-primary" />
                <div className="text-center">
                  <p className="text-sm font-medium text-text-0">Identity System Required</p>
                  <p className="text-xs text-text-3 mt-1">
                    Initialize the identity file system to unlock this tab.
                  </p>
                </div>
                <Button onClick={handleInitIdentity} loading={initializing} icon={<Sparkles className="w-3.5 h-3.5" />}>
                  Initialize Identity
                </Button>
              </div>
            </Card>
          </div>
        ) : activeTab === 'about' ? (
          <div className="p-4 md:p-6 max-w-2xl space-y-5">
            {/* Avatar */}
            <Card>
              <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-4">Avatar</h3>
              <div className="flex flex-col items-center gap-4">
                <div className="relative group">
                  <img src={avatarPath} alt={name} className="w-24 h-24 rounded-2xl shadow-lg shadow-black/20" />
                  <label className="absolute inset-0 rounded-2xl bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center cursor-pointer">
                    {uploading ? (
                      <div className="w-6 h-6 border-2 border-white border-t-transparent rounded-full animate-spin" />
                    ) : (
                      <Upload className="w-5 h-5 text-white" />
                    )}
                    <input type="file" accept="image/png,image/jpeg,image/webp" onChange={handleUpload} className="hidden" aria-label="Upload avatar" tabIndex={-1} />
                  </label>
                </div>
                <div className="w-full max-h-48 overflow-y-auto rounded-lg border border-border-1 bg-surface-0 p-2">
                  <div className="grid grid-cols-8 md:grid-cols-10 gap-1.5">
                    {PRESET_AVATARS.map((path, i) => (
                      <button
                        key={path}
                        onClick={() => setAvatarPath(path)}
                        aria-label={`Select avatar ${i + 1}`}
                        aria-pressed={avatarPath === path}
                        className={`aspect-square rounded-lg overflow-hidden border-2 transition-all cursor-pointer ${
                          avatarPath === path
                            ? 'border-accent-primary ring-2 ring-accent-primary/20 scale-105'
                            : 'border-transparent hover:border-border-0 opacity-70 hover:opacity-100'
                        }`}
                      >
                        <img src={path} alt="" className="w-full h-full object-cover" />
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            </Card>

            {/* Details */}
            <Card>
              <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-4">Details</h3>
              <div className="space-y-4">
                <Input label="Name" value={name} onChange={e => setName(e.target.value)} placeholder="Agent name" />
                <Input label="Description" value={description} onChange={e => setDescription(e.target.value)} placeholder="What does this agent do?" />
                <div>
                  <label className="block text-xs font-medium text-text-2 mb-1.5 flex items-center gap-1.5">
                    <FolderOpen className="w-3.5 h-3.5" />
                    Folder
                  </label>
                  <FolderAssign value={folder} folders={allFolders} onChange={handleFolderChange} />
                </div>
                <div>
                  <label className="block text-xs font-medium text-text-2 mb-1.5">Model</label>
                  <div className="relative">
                    <button
                      onClick={() => setModelPickerOpen(!modelPickerOpen)}
                      className="w-full flex items-center justify-between px-3 py-2 rounded-lg border border-border-1 bg-surface-0 text-sm text-text-1 hover:border-border-0 transition-colors cursor-pointer"
                    >
                      <span className="truncate">{availableModels.find(m => m.id === model)?.name || model || 'Select a model'}</span>
                      <span className="text-text-3 ml-2 text-xs">{modelPickerOpen ? '\u25B2' : '\u25BC'}</span>
                    </button>
                    {modelPickerOpen && (
                      <div className="absolute z-20 mt-1 w-full rounded-lg border border-border-1 bg-surface-1 shadow-xl max-h-64 overflow-hidden flex flex-col">
                        <div className="p-2 border-b border-border-0">
                          <div className="relative">
                            <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-3" />
                            <input
                              type="text"
                              value={modelSearch}
                              onChange={e => setModelSearch(e.target.value)}
                              placeholder="Search models..."
                              className="w-full pl-7 pr-2 py-1.5 rounded-md bg-surface-2 border border-border-1 text-sm text-text-1 placeholder:text-text-3 outline-none focus:border-accent-primary"
                              autoFocus
                            />
                          </div>
                        </div>
                        <div className="overflow-y-auto flex-1">
                          {availableModels
                            .filter(m => m.id.toLowerCase().includes(modelSearch.toLowerCase()) || m.name.toLowerCase().includes(modelSearch.toLowerCase()))
                            .slice(0, 30)
                            .map(m => (
                              <button
                                key={m.id}
                                onClick={() => { setModel(m.id); setModelPickerOpen(false); setModelSearch(''); }}
                                className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer hover:bg-surface-2 ${
                                  m.id === model ? 'bg-accent-muted text-accent-text' : 'text-text-1'
                                }`}
                              >
                                <span className="block truncate font-medium">{m.name}</span>
                                <span className="block truncate text-xs text-text-3">{m.id}</span>
                              </button>
                            ))}
                          {availableModels.filter(m => m.id.toLowerCase().includes(modelSearch.toLowerCase()) || m.name.toLowerCase().includes(modelSearch.toLowerCase())).length === 0 && (
                            <p className="p-3 text-xs text-text-3 text-center">No models found</p>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </Card>

            {/* Heartbeat */}
            <Card>
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3">Heartbeat</h3>
                  <p className="text-[11px] text-text-3 mt-0.5">Periodic autonomous check-ins</p>
                </div>
                <Toggle
                  enabled={!!role?.heartbeat_enabled}
                  onChange={async () => {
                    if (!slug) return;
                    const newVal = !role?.heartbeat_enabled;
                    try {
                      const updated = await api.put<AgentRole>(`/agent-roles/${slug}`, { heartbeat_enabled: newVal });
                      setRole(updated);
                      toast('success', newVal ? 'Heartbeat enabled' : 'Heartbeat disabled');
                    } catch (e) { console.warn('toggleHeartbeat failed:', e); toast('error', 'Failed to toggle heartbeat'); }
                  }}
                  label="Enable heartbeat"
                />
              </div>
            </Card>

            {/* Metadata */}
            <div className="px-1 space-y-1.5">
              <div className="flex justify-between text-[11px]">
                <span className="text-text-3">Slug</span>
                <span className="text-text-2 font-mono">{role.slug}</span>
              </div>
              <div className="flex justify-between text-[11px]">
                <span className="text-text-3">Created</span>
                <span className="text-text-2">{new Date(role.created_at).toLocaleDateString()}</span>
              </div>
              <div className="flex justify-between text-[11px]">
                <span className="text-text-3">Updated</span>
                <span className="text-text-2">{new Date(role.updated_at).toLocaleDateString()}</span>
              </div>
              <div className="flex justify-between text-[11px]">
                <span className="text-text-3">Identity</span>
                <span className={`text-text-2 ${isInitialized ? 'text-green-400' : 'text-text-3'}`}>
                  {isInitialized ? 'Initialized' : 'Legacy'}
                </span>
              </div>
            </div>

            {/* Delete */}
            <div className="pt-2">
              <button
                onClick={() => setShowDeleteConfirm(true)}
                className="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-xs font-medium text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer border border-transparent hover:border-red-500/20"
              >
                <Trash2 className="w-3.5 h-3.5" />
                Delete Agent
              </button>
            </div>
          </div>
        ) : activeTab === 'work' ? (
          <div className="flex flex-col h-full">
            {/* Board toolbar */}
            <div className="flex items-center justify-between px-4 md:px-6 py-3 border-b border-border-0 flex-shrink-0">
              <div className="flex items-center gap-4">
                {STATUS_COLUMNS.map(col => {
                  const count = tasks.filter(t => t.status === col.key).length;
                  return (
                    <div key={col.key} className="flex items-center gap-1.5">
                      <col.icon className={`w-3.5 h-3.5 ${col.color}`} />
                      <span className="text-xs text-text-3">{count}</span>
                    </div>
                  );
                })}
              </div>
              <div className="flex items-center gap-2">
                {tasks.some(t => t.status === 'done') && (
                  <button
                    onClick={handleClearDone}
                    className="text-xs text-text-3 hover:text-text-1 transition-colors cursor-pointer px-2 py-1 rounded-md hover:bg-surface-2"
                  >
                    Clear done
                  </button>
                )}
                <Button
                  size="sm"
                  onClick={() => setShowCreateTask(true)}
                  icon={<Plus className="w-3.5 h-3.5" />}
                >
                  New Task
                </Button>
              </div>
            </div>

            {/* Kanban columns */}
            <div className="flex-1 overflow-x-auto overflow-y-hidden">
              <div className="flex gap-4 p-4 md:px-6 h-full min-w-min">
                {STATUS_COLUMNS.map(col => {
                  const colTasks = tasks
                    .filter(t => t.status === col.key)
                    .sort((a, b) => a.sort_order - b.sort_order);
                  const ColIcon = col.icon;
                  return (
                    <div key={col.key} className={`flex-shrink-0 w-72 md:w-80 flex flex-col rounded-xl ${col.bg} border ${col.border}`}>
                      {/* Column header */}
                      <div className="flex items-center justify-between px-4 py-3 flex-shrink-0">
                        <div className="flex items-center gap-2">
                          <ColIcon className={`w-4 h-4 ${col.color}`} />
                          <h3 className={`text-sm font-semibold ${col.color}`}>{col.label}</h3>
                          <span className="text-[11px] text-text-3 bg-surface-0/50 px-2 py-0.5 rounded-full font-medium">{colTasks.length}</span>
                        </div>
                      </div>

                      {/* Task cards */}
                      <div className="flex-1 overflow-y-auto px-2 pb-2 space-y-2">
                        {colTasks.map(task => (
                          <button
                            key={task.id}
                            onClick={() => openTaskDetail(task)}
                            className="w-full text-left rounded-lg border border-border-1 bg-surface-1 p-3 hover:border-border-0 hover:shadow-md transition-all cursor-pointer group"
                          >
                            <div className="flex items-start gap-2">
                              <GripVertical className="w-3.5 h-3.5 text-text-3/0 group-hover:text-text-3/50 mt-0.5 flex-shrink-0 transition-colors" />
                              <div className="flex-1 min-w-0">
                                <p className="text-sm font-medium text-text-0 leading-snug">{task.title}</p>
                                {task.description && (
                                  <p className="text-[11px] text-text-3 line-clamp-2 mt-1 leading-relaxed">{task.description}</p>
                                )}
                                <div className="flex items-center gap-2 mt-2">
                                  <span className="text-[10px] text-text-3 flex items-center gap-1">
                                    <Clock className="w-3 h-3" />
                                    {timeAgo(task.updated_at || task.created_at)}
                                  </span>
                                </div>
                              </div>
                              <ChevronRight className="w-3.5 h-3.5 text-text-3/0 group-hover:text-text-3/50 mt-0.5 flex-shrink-0 transition-colors" />
                            </div>
                          </button>
                        ))}

                        {colTasks.length === 0 && (
                          <div className="flex items-center justify-center py-8 text-xs text-text-3/50">
                            No tasks
                          </div>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Create task modal */}
            <Modal open={showCreateTask} onClose={() => { setShowCreateTask(false); setCreateTaskTitle(''); setCreateTaskDesc(''); setCreateTaskStatus('backlog'); }} title="New Task" size="sm">
              <form onSubmit={e => { e.preventDefault(); handleCreateTask(); }} className="space-y-4">
                <Input
                  label="Title"
                  value={createTaskTitle}
                  onChange={e => setCreateTaskTitle(e.target.value)}
                  placeholder="Short, action-oriented title"
                  autoFocus
                />
                <div className="space-y-1.5">
                  <label htmlFor="task-desc" className="block text-sm font-medium text-text-1">Description</label>
                  <textarea
                    id="task-desc"
                    value={createTaskDesc}
                    onChange={e => setCreateTaskDesc(e.target.value)}
                    placeholder="Context, details, what 'done' looks like..."
                    rows={4}
                    className="block w-full rounded-lg border border-border-1 bg-surface-2 text-text-0 px-3 py-2 text-sm placeholder:text-text-3 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors resize-none"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="block text-sm font-medium text-text-1">Status</label>
                  <div className="flex gap-2">
                    {STATUS_COLUMNS.filter(c => c.key !== 'done').map(col => {
                      const ColIcon = col.icon;
                      return (
                        <button
                          key={col.key}
                          type="button"
                          onClick={() => setCreateTaskStatus(col.key)}
                          className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all cursor-pointer border ${
                            createTaskStatus === col.key
                              ? `${col.color} ${col.bg} ${col.border}`
                              : 'text-text-3 border-border-1 hover:border-border-0'
                          }`}
                        >
                          <ColIcon className="w-3.5 h-3.5" />
                          {col.label}
                        </button>
                      );
                    })}
                  </div>
                </div>
                <div className="flex justify-end gap-2 pt-2">
                  <Button variant="ghost" size="sm" type="button" onClick={() => { setShowCreateTask(false); setCreateTaskTitle(''); setCreateTaskDesc(''); setCreateTaskStatus('backlog'); }}>
                    Cancel
                  </Button>
                  <Button size="sm" type="submit" loading={addingTask} disabled={!createTaskTitle.trim()} icon={<Plus className="w-3.5 h-3.5" />}>
                    Create Task
                  </Button>
                </div>
              </form>
            </Modal>

            {/* Task detail/edit modal */}
            <Modal open={!!viewingTask} onClose={() => setViewingTask(null)} title="Task" size="sm">
              {viewingTask && (
                <div className="space-y-4">
                  <Input
                    label="Title"
                    value={editTitle}
                    onChange={e => setEditTitle(e.target.value)}
                    placeholder="Task title"
                  />
                  <div className="space-y-1.5">
                    <label htmlFor="edit-task-desc" className="block text-sm font-medium text-text-1">Description</label>
                    <textarea
                      id="edit-task-desc"
                      value={editDesc}
                      onChange={e => setEditDesc(e.target.value)}
                      placeholder="Add details, context, links..."
                      rows={5}
                      className="block w-full rounded-lg border border-border-1 bg-surface-2 text-text-0 px-3 py-2 text-sm placeholder:text-text-3 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors resize-none"
                    />
                  </div>

                  {/* Status selector */}
                  <div className="space-y-1.5">
                    <label className="block text-sm font-medium text-text-1">Status</label>
                    <div className="flex gap-2">
                      {STATUS_COLUMNS.map(col => {
                        const ColIcon = col.icon;
                        return (
                          <button
                            key={col.key}
                            type="button"
                            onClick={() => handleMoveTask(viewingTask.id, col.key)}
                            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all cursor-pointer border ${
                              viewingTask.status === col.key
                                ? `${col.color} ${col.bg} ${col.border}`
                                : 'text-text-3 border-border-1 hover:border-border-0'
                            }`}
                          >
                            <ColIcon className="w-3.5 h-3.5" />
                            {col.label}
                          </button>
                        );
                      })}
                    </div>
                  </div>

                  {/* Metadata */}
                  <div className="flex items-center gap-4 text-[11px] text-text-3 pt-1">
                    <span>Created {new Date(viewingTask.created_at).toLocaleDateString()}</span>
                    {viewingTask.updated_at && viewingTask.updated_at !== viewingTask.created_at && (
                      <span>Updated {new Date(viewingTask.updated_at).toLocaleDateString()}</span>
                    )}
                  </div>

                  {/* Actions */}
                  <div className="flex items-center justify-between pt-2 border-t border-border-0">
                    <button
                      onClick={() => { handleDeleteTask(viewingTask.id); }}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      Delete
                    </button>
                    <div className="flex gap-2">
                      <Button variant="ghost" size="sm" onClick={() => setViewingTask(null)}>
                        Cancel
                      </Button>
                      <Button
                        size="sm"
                        onClick={handleSaveTask}
                        loading={editSaving}
                        disabled={!editTitle.trim() || (editTitle === viewingTask.title && editDesc === viewingTask.description)}
                        icon={<Save className="w-3.5 h-3.5" />}
                      >
                        Save
                      </Button>
                    </div>
                  </div>
                </div>
              )}
            </Modal>
          </div>
        ) : activeTab === 'memory' ? (
          <div className="p-4 md:p-6 max-w-4xl">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <BookOpen className="w-4 h-4 text-accent-primary" />
                <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3">Long-term Memory</h3>
              </div>
              {memoryStats && (
                <div className="flex items-center gap-3 text-[11px] text-text-3">
                  <span>{memoryStats.total_active} active</span>
                  {memoryStats.total_archived > 0 && <span>{memoryStats.total_archived} archived</span>}
                </div>
              )}
            </div>

            {memoryStats && Object.keys(memoryStats.categories).length > 0 && (
              <div className="flex flex-wrap gap-1.5 mb-3">
                {Object.entries(memoryStats.categories).map(([cat, count]) => (
                  <span key={cat} className="text-[10px] px-2 py-0.5 rounded-full bg-surface-2 text-text-2 border border-border-1">
                    {cat} ({count})
                  </span>
                ))}
              </div>
            )}

            {memories.length === 0 ? (
              <div className="flex items-center justify-center py-16 text-sm text-text-3">
                No memories yet. The agent will save memories automatically during conversations.
              </div>
            ) : (
              <div className="space-y-2">
                {memories.map(mem => (
                  <div key={mem.id} className="rounded-lg border border-border-1 bg-surface-1 overflow-hidden">
                    <button
                      onClick={() => setExpandedMemory(expandedMemory === mem.id ? null : mem.id)}
                      className="w-full flex items-center gap-3 px-3 py-2.5 text-left hover:bg-surface-2/50 transition-colors cursor-pointer"
                    >
                      <div className="flex-1 min-w-0">
                        <p className="text-xs text-text-1 truncate">{mem.summary || mem.content.slice(0, 100)}</p>
                        <div className="flex items-center gap-2 mt-1">
                          {mem.category && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-accent-muted text-accent-text">{mem.category}</span>
                          )}
                          <span className="text-[10px] text-text-3">{new Date(mem.created_at).toLocaleDateString()}</span>
                          {mem.importance >= 8 && <span className="text-[10px] text-yellow-500">high</span>}
                        </div>
                      </div>
                      <span className="text-[10px] text-text-3 flex-shrink-0">{expandedMemory === mem.id ? '\u25B2' : '\u25BC'}</span>
                    </button>
                    {expandedMemory === mem.id && (
                      <div className="px-3 pb-3 border-t border-border-0">
                        <pre className="text-xs text-text-2 font-mono whitespace-pre-wrap mt-2 mb-2 leading-relaxed">{mem.content}</pre>
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 text-[10px] text-text-3">
                            {mem.source && <span>source: {mem.source}</span>}
                            {mem.tags && <span>tags: {mem.tags}</span>}
                            <span>importance: {mem.importance}/10</span>
                            <span>accessed: {mem.access_count}x</span>
                          </div>
                          <button
                            onClick={() => handleMemoryDelete(mem.id)}
                            className="p-1.5 rounded-lg text-text-3 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
                            title="Delete memory"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : activeTab === 'skills' ? (
          <div className="p-4 md:p-6 max-w-4xl">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <Sparkles className="w-4 h-4 text-accent-primary" />
                <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3">Installed Skills</h3>
              </div>
              <Button size="sm" onClick={() => setShowSkillPicker(!showSkillPicker)} icon={<Plus className="w-3.5 h-3.5" />}>
                Add Skill
              </Button>
            </div>

            {showSkillPicker && availableGlobalSkills.length > 0 && (
              <div className="mb-4 p-3 rounded-lg border border-border-1 bg-surface-1">
                <p className="text-xs text-text-3 mb-2">Available global skills:</p>
                <div className="flex flex-wrap gap-2">
                  {availableGlobalSkills.map(skill => (
                    <button
                      key={skill.name}
                      onClick={() => handleAddSkill(skill.name)}
                      className="px-3 py-1.5 text-xs rounded-lg bg-surface-2 text-text-1 hover:bg-accent-muted hover:text-accent-text transition-colors cursor-pointer"
                    >
                      + {skill.name}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {showSkillPicker && availableGlobalSkills.length === 0 && (
              <div className="mb-4 p-3 rounded-lg border border-border-1 bg-surface-1">
                <p className="text-xs text-text-3">No more global skills available. Create new ones on the Skills page.</p>
              </div>
            )}

            {agentSkillList.length === 0 ? (
              <div className="flex items-center justify-center py-16 text-sm text-text-3">
                No skills installed. Add skills from the global library.
              </div>
            ) : (
              <div className="space-y-2">
                {agentSkillList.map(skill => (
                  <div key={skill.name} className="flex items-center gap-3 p-3 rounded-lg border border-border-1 bg-surface-1">
                    <div className="w-8 h-8 rounded-lg bg-accent-muted flex items-center justify-center flex-shrink-0">
                      <Sparkles className="w-4 h-4 text-accent-text" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-text-0 font-mono">{skill.name}</p>
                      <p className="text-[11px] text-text-3 truncate">{skill.description || skill.summary || 'No description'}</p>
                    </div>
                    <div className="flex items-center gap-1 flex-shrink-0">
                      <button
                        onClick={() => handlePublishSkill(skill.name)}
                        className="p-1.5 rounded-lg text-text-3 hover:text-accent-primary hover:bg-accent-muted transition-colors cursor-pointer"
                        title="Publish to global"
                      >
                        <ArrowUpFromLine className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => handleRemoveSkill(skill.name)}
                        className="p-1.5 rounded-lg text-text-3 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
                        title="Remove skill"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : activeTab === 'tools' ? (
          <div className="p-4 md:p-6 max-w-4xl">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <Wrench className="w-4 h-4 text-accent-primary" />
                <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3">Agent Tools</h3>
              </div>
              <Button size="sm" onClick={() => setShowToolPicker(!showToolPicker)} icon={<Plus className="w-3.5 h-3.5" />}>
                Grant Access
              </Button>
            </div>
            {showToolPicker && (() => {
              const agentToolIds = new Set(agentTools.map(t => t.id));
              const grantable = allTools.filter(t => !agentToolIds.has(t.id));
              return grantable.length > 0 ? (
                <div className="mb-4 p-3 rounded-lg border border-border-1 bg-surface-1">
                  <p className="text-xs text-text-3 mb-2">Grant access to a tool:</p>
                  <div className="space-y-1">
                    {grantable.map(tool => (
                      <button key={tool.id} onClick={() => handleGrantTool(tool.id)} className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-left hover:bg-surface-2 transition-colors cursor-pointer">
                        <Wrench className="w-4 h-4 text-text-3 flex-shrink-0" />
                        <div className="min-w-0">
                          <p className="text-sm text-text-1 truncate">{tool.name}</p>
                          <p className="text-[11px] text-text-3 truncate">{tool.description}</p>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              ) : (
                <div className="mb-4 p-3 rounded-lg border border-border-1 bg-surface-1">
                  <p className="text-xs text-text-3">No additional tools available to grant.</p>
                </div>
              );
            })()}
            {agentTools.length === 0 ? (
              <div className="flex items-center justify-center py-16 text-sm text-text-3">No tools assigned. Grant access to tools from the library.</div>
            ) : (
              <div className="space-y-2">
                {agentTools.map(tool => (
                  <div key={`${tool.id}-${tool.access_type}`} className="flex items-center gap-3 p-3 rounded-lg border border-border-1 bg-surface-1">
                    <div className={`w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 ${tool.access_type === 'owned' ? 'bg-accent-muted' : 'bg-surface-3'}`}>
                      <Wrench className={`w-4 h-4 ${tool.access_type === 'owned' ? 'text-accent-text' : 'text-text-3'}`} />
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="text-sm font-medium text-text-0 truncate">{tool.name}</p>
                        <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${tool.access_type === 'owned' ? 'bg-accent-muted text-accent-text' : 'bg-surface-3 text-text-3'}`}>
                          {tool.access_type === 'owned' ? 'Owner' : 'Granted'}
                        </span>
                      </div>
                      <p className="text-[11px] text-text-3 truncate">{tool.description}</p>
                    </div>
                    {tool.access_type === 'granted' && (
                      <button onClick={() => handleRevokeTool(tool.id)} className="p-1.5 rounded-lg text-text-3 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer flex-shrink-0" title="Revoke access">
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : fileTabForActive ? (
          <div className="p-4 md:p-6 flex flex-col" style={{ height: 'calc(100vh - 10rem)' }}>
            <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
              <p className="text-[11px] text-text-3">{fileTabForActive.description}</p>
              <div className="flex items-center gap-3 shrink-0">
                <span className="text-[11px] text-text-3 font-mono tabular-nums whitespace-nowrap">
                  {currentLineCount} lines · {currentFileContent.length} chars
                </span>
                <Button
                  size="sm"
                  onClick={() => handleFileSave(fileTabForActive.filename)}
                  loading={fileSaving}
                  disabled={!fileDirty[fileTabForActive.filename]}
                  icon={<Save className="w-3.5 h-3.5" />}
                >
                  Save
                </Button>
              </div>
            </div>
            <div className="relative flex-1 min-h-0">
              <textarea
                ref={promptRef}
                value={currentFileContent}
                onChange={e => handleFileChange(fileTabForActive.filename, e.target.value)}
                onKeyDown={handlePromptKeyDown}
                placeholder={`Edit ${fileTabForActive.filename}...`}
                className="absolute inset-0 w-full h-full rounded-lg border border-border-1 bg-surface-0 text-text-1 px-4 py-3 text-[13px] leading-relaxed font-mono placeholder:text-text-3/50 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors resize-none"
                spellCheck={false}
              />
            </div>
          </div>
        ) : null}
      </div>

      <Modal open={showDeleteConfirm} onClose={() => setShowDeleteConfirm(false)} title="Delete Agent" size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-2">
            Are you sure you want to delete <span className="font-medium text-text-1">"{role?.name}"</span>? This will remove the agent from all chat threads. Previous messages from this agent will be preserved.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setShowDeleteConfirm(false)}>Cancel</Button>
            <Button variant="danger" size="sm" onClick={handleDelete} loading={deleting} icon={<Trash2 className="w-3.5 h-3.5" />}>Delete</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
