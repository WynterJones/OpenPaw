import { useState, useEffect, useRef, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { FolderOpen, Plus, Play, Pencil, Trash2, X, ChevronDown, ChevronRight, FolderGit2 } from 'lucide-react';
import { api, projectsApi, type Project } from '../../lib/api';
import { useWorkbench } from './WorkbenchProvider';

const TAB_COLORS = [
  '',
  '#ef4444',
  '#f97316',
  '#eab308',
  '#22c55e',
  '#3b82f6',
  '#8b5cf6',
  '#ec4899',
];

const COMMAND_PRESETS = [
  { label: 'Claude Code', command: 'claude' },
  { label: 'Codex', command: 'codex' },
  { label: 'Gemini CLI', command: 'gemini' },
  { label: 'Custom', command: '' },
];

export function ProjectsButton() {
  const [showDropdown, setShowDropdown] = useState(false);
  const [dropdownPos, setDropdownPos] = useState({ top: 0, right: 0 });
  const btnRef = useRef<HTMLButtonElement>(null);

  const openDropdown = useCallback(() => {
    if (btnRef.current) {
      const rect = btnRef.current.getBoundingClientRect();
      setDropdownPos({ top: rect.bottom + 4, right: window.innerWidth - rect.right });
    }
    setShowDropdown(true);
  }, []);

  return (
    <>
      <button
        ref={btnRef}
        onClick={openDropdown}
        className="flex items-center gap-1.5 h-7 px-3 rounded-lg border border-border-1 text-text-2 hover:text-text-0 hover:bg-surface-2 transition-all shrink-0 cursor-pointer text-xs"
        title="Projects"
      >
        <FolderGit2 className="w-3.5 h-3.5" />
        <span>Projects</span>
      </button>

      {showDropdown && (
        <ProjectsDropdown
          pos={dropdownPos}
          onClose={() => setShowDropdown(false)}
        />
      )}
    </>
  );
}

function ProjectsDropdown({
  pos,
  onClose,
}: {
  pos: { top: number; right: number };
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const { launchSession } = useWorkbench();
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [editingProject, setEditingProject] = useState<Project | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);

  const loadProjects = useCallback(async () => {
    try {
      const data = await projectsApi.list();
      setProjects(data);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadProjects(); }, [loadProjects]);

  const modalOpenRef = useRef(false);
  modalOpenRef.current = showCreateModal || editingProject !== null;

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (modalOpenRef.current) return;
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [onClose]);

  const handleLaunch = useCallback(async (project: Project, repo: Project['repos'][0]) => {
    await launchSession({
      title: repo.name || project.name,
      cwd: repo.folder_path,
      command: repo.command,
      color: project.color,
    });
    onClose();
  }, [launchSession, onClose]);

  const handleDelete = useCallback(async (id: string) => {
    await projectsApi.delete(id);
    setProjects(prev => prev.filter(p => p.id !== id));
  }, []);

  return createPortal(
    <div
      ref={ref}
      className="fixed z-[9999] bg-surface-2 border border-border-1 rounded-xl shadow-2xl flex flex-col max-h-[70vh] overflow-hidden inset-x-2 sm:inset-x-auto sm:w-[500px]"
      style={{ top: pos.top, right: window.innerWidth >= 640 ? pos.right : undefined }}
    >
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2.5 border-b border-border-0">
        <div className="flex items-center gap-2">
          <FolderGit2 className="w-4 h-4 text-text-2" />
          <span className="text-sm font-medium text-text-0">Projects</span>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="flex items-center gap-1 text-[11px] font-medium px-2.5 py-1 rounded-md bg-accent-primary text-white hover:bg-accent-primary/90 transition-colors cursor-pointer"
        >
          <Plus className="w-3 h-3" />
          New
        </button>
      </div>

      {/* Project list */}
      <div className="flex-1 overflow-y-auto py-1">
        {loading ? (
          <div className="flex items-center justify-center py-6">
            <div className="animate-spin w-4 h-4 border-2 border-accent-primary border-t-transparent rounded-full" />
          </div>
        ) : projects.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-6 text-text-3 text-xs">
            <FolderOpen className="w-8 h-8" />
            <span>No projects yet</span>
            <button
              onClick={() => setShowCreateModal(true)}
              className="text-accent-primary hover:text-accent-text transition-colors cursor-pointer"
            >
              Create your first project
            </button>
          </div>
        ) : (
          projects.map((project) => {
            const isExpanded = expandedId === project.id;
            return (
              <div key={project.id}>
                <div
                  className="group flex items-center gap-2 px-3 py-2 hover:bg-surface-3/50 cursor-pointer transition-colors"
                  onClick={() => setExpandedId(isExpanded ? null : project.id)}
                >
                  {/* Color indicator */}
                  {project.color ? (
                    <div className="w-[3px] h-5 rounded-full shrink-0" style={{ backgroundColor: project.color }} />
                  ) : (
                    <div className="w-[3px] h-5 shrink-0" />
                  )}

                  {isExpanded ? (
                    <ChevronDown className="w-3.5 h-3.5 text-text-3 shrink-0" />
                  ) : (
                    <ChevronRight className="w-3.5 h-3.5 text-text-3 shrink-0" />
                  )}

                  <span className="text-xs text-text-0 font-medium truncate flex-1">{project.name}</span>
                  <span className="text-[10px] text-text-3">{project.repos.length} repo{project.repos.length !== 1 ? 's' : ''}</span>

                  <div className="opacity-0 group-hover:opacity-100 flex items-center gap-0.5 transition-opacity">
                    <button
                      onClick={(e) => { e.stopPropagation(); setEditingProject(project); }}
                      className="w-5 h-5 flex items-center justify-center rounded hover:bg-surface-3 text-text-3 hover:text-text-1 cursor-pointer"
                    >
                      <Pencil className="w-3 h-3" />
                    </button>
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDelete(project.id); }}
                      className="w-5 h-5 flex items-center justify-center rounded hover:bg-surface-3 text-text-3 hover:text-danger cursor-pointer"
                    >
                      <Trash2 className="w-3 h-3" />
                    </button>
                  </div>
                </div>

                {isExpanded && (
                  <div className="pl-5 pr-2 pb-1">
                    {project.repos.length === 0 ? (
                      <div className="text-[11px] text-text-3 py-1.5 pl-4">No repos configured</div>
                    ) : (
                      project.repos.map((repo) => (
                        <button
                          key={repo.id}
                          onClick={() => handleLaunch(project, repo)}
                          className="w-full flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-surface-3/50 text-left transition-colors group/repo cursor-pointer"
                        >
                          <Play className="w-3 h-3 text-text-3 group-hover/repo:text-accent-primary shrink-0 transition-colors" />
                          <div className="flex-1 min-w-0">
                            <div className="text-xs text-text-1 truncate">{repo.name}</div>
                            <div className="text-[10px] text-text-3 truncate">{repo.folder_path}</div>
                          </div>
                          {repo.command && (
                            <code className="text-[10px] text-text-3 bg-surface-1 px-1.5 py-0.5 rounded shrink-0 max-w-24 truncate">
                              {repo.command}
                            </code>
                          )}
                        </button>
                      ))
                    )}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>

      {/* Create/Edit modal */}
      {(showCreateModal || editingProject) && (
        <ProjectModal
          project={editingProject}
          onClose={() => { setShowCreateModal(false); setEditingProject(null); }}
          onSave={() => { setShowCreateModal(false); setEditingProject(null); loadProjects(); }}
        />
      )}
    </div>,
    document.body,
  );
}

// ── Project Create/Edit Modal ──

function ProjectModal({
  project,
  onClose,
  onSave,
}: {
  project: Project | null;
  onClose: () => void;
  onSave: () => void;
}) {
  const [name, setName] = useState(project?.name || '');
  const [color, setColor] = useState(project?.color || '');
  const [repos, setRepos] = useState<{ name: string; folder_path: string; command: string }[]>(
    project?.repos.map(r => ({ name: r.name, folder_path: r.folder_path, command: r.command })) || [{ name: '', folder_path: '', command: '' }],
  );
  const [saving, setSaving] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);

  useEffect(() => { nameRef.current?.focus(); }, []);

  const handleSave = async () => {
    if (!name.trim()) return;
    setSaving(true);
    try {
      const validRepos = repos.filter(r => r.folder_path.trim());
      if (project) {
        await projectsApi.update(project.id, { name: name.trim(), color, repos: validRepos });
      } else {
        await projectsApi.create({ name: name.trim(), color, repos: validRepos });
      }
      onSave();
    } catch {
      // error
    } finally {
      setSaving(false);
    }
  };

  const addRepo = () => {
    setRepos(prev => [...prev, { name: '', folder_path: '', command: '' }]);
  };

  const removeRepo = (index: number) => {
    setRepos(prev => prev.filter((_, i) => i !== index));
  };

  const updateRepo = (index: number, field: string, value: string) => {
    setRepos(prev => prev.map((r, i) => i === index ? { ...r, [field]: value } : r));
  };

  return createPortal(
    <div className="fixed inset-0 z-[10000] flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="bg-surface-1 border border-border-1 rounded-2xl shadow-2xl w-full max-w-lg mx-4 overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Modal header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-border-0">
          <h3 className="text-sm font-semibold text-text-0">
            {project ? 'Edit Project' : 'New Project'}
          </h3>
          <button onClick={onClose} className="text-text-3 hover:text-text-1 cursor-pointer">
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Modal body */}
        <div className="px-5 py-4 flex flex-col gap-4 max-h-[60vh] overflow-y-auto">
          {/* Project name */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-text-2">Project Name</label>
            <input
              ref={nameRef}
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') handleSave(); }}
              className="bg-surface-0 border border-border-0 rounded-lg text-sm text-text-0 px-3 py-2 outline-none focus:border-border-1 caret-accent-primary"
              placeholder="My Project"
            />
          </div>

          {/* Color */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-text-2">Color</label>
            <div className="flex gap-2">
              {TAB_COLORS.map((c) => (
                <button
                  key={c || 'none'}
                  onClick={() => setColor(c)}
                  className="w-6 h-6 rounded-full border-2 hover:scale-110 transition-transform cursor-pointer"
                  style={{
                    backgroundColor: c || 'var(--op-surface-3)',
                    borderColor: color === c ? 'var(--op-text-0)' : 'var(--op-border-1)',
                  }}
                />
              ))}
            </div>
          </div>

          {/* Repos */}
          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <label className="text-xs font-medium text-text-2">Repos</label>
              <button
                onClick={addRepo}
                className="flex items-center gap-1 text-xs text-accent-primary hover:text-accent-text transition-colors cursor-pointer"
              >
                <Plus className="w-3 h-3" />
                Add Repo
              </button>
            </div>

            {repos.map((repo, i) => (
              <div key={i} className="bg-surface-0 border border-border-0 rounded-lg p-3 flex flex-col gap-2.5 relative">
                {repos.length > 1 && (
                  <button
                    onClick={() => removeRepo(i)}
                    className="absolute top-2 right-2 w-5 h-5 flex items-center justify-center rounded hover:bg-surface-2 text-text-3 hover:text-danger cursor-pointer"
                  >
                    <X className="w-3 h-3" />
                  </button>
                )}

                <div className="flex gap-2">
                  <div className="flex-1 flex flex-col gap-1">
                    <label className="text-[11px] text-text-3">Name</label>
                    <input
                      value={repo.name}
                      onChange={(e) => updateRepo(i, 'name', e.target.value)}
                      className="bg-surface-1 border border-border-0 rounded-md text-xs text-text-0 px-2.5 py-1.5 outline-none focus:border-border-1 caret-accent-primary"
                      placeholder="frontend"
                    />
                  </div>
                </div>

                <div className="flex flex-col gap-1">
                  <label className="text-[11px] text-text-3">Folder Path</label>
                  <div className="flex gap-1.5">
                    <input
                      value={repo.folder_path}
                      onChange={(e) => updateRepo(i, 'folder_path', e.target.value)}
                      className="flex-1 bg-surface-1 border border-border-0 rounded-md text-xs text-text-0 px-2.5 py-1.5 outline-none focus:border-border-1 caret-accent-primary font-mono"
                      placeholder="/Users/you/projects/my-app"
                    />
                    <button
                      type="button"
                      onClick={async () => {
                        try {
                          const result = await api.post<{ path: string }>('/system/pick-folder', {});
                          if (result.path) {
                            updateRepo(i, 'folder_path', result.path);
                            if (!repo.name) {
                              const folderName = result.path.split('/').filter(Boolean).pop() || '';
                              updateRepo(i, 'name', folderName);
                            }
                          }
                        } catch { /* cancelled */ }
                      }}
                      className="flex items-center justify-center w-8 h-8 rounded-md border border-border-0 bg-surface-1 text-text-3 hover:text-text-1 hover:border-border-1 transition-colors cursor-pointer shrink-0"
                      title="Browse folder"
                    >
                      <FolderOpen className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div className="flex flex-col gap-1">
                  <label className="text-[11px] text-text-3">Command</label>
                  <div className="flex gap-1.5 flex-wrap mb-1">
                    {COMMAND_PRESETS.map((preset) => (
                      <button
                        key={preset.label}
                        onClick={() => { if (preset.command) updateRepo(i, 'command', preset.command); }}
                        className={`text-[10px] px-2 py-0.5 rounded-md border transition-colors cursor-pointer ${
                          repo.command === preset.command && preset.command
                            ? 'border-accent-primary bg-accent-primary/10 text-accent-text'
                            : 'border-border-0 text-text-3 hover:border-border-1 hover:text-text-1'
                        }`}
                      >
                        {preset.label}
                      </button>
                    ))}
                  </div>
                  <input
                    value={repo.command}
                    onChange={(e) => updateRepo(i, 'command', e.target.value)}
                    className="bg-surface-1 border border-border-0 rounded-md text-xs text-text-0 px-2.5 py-1.5 outline-none focus:border-border-1 caret-accent-primary font-mono"
                    placeholder="claude, codex, npm start, etc."
                  />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Modal footer */}
        <div className="flex items-center justify-end gap-2 px-5 py-3.5 border-t border-border-0">
          <button
            onClick={onClose}
            className="px-4 py-2 text-xs text-text-2 hover:text-text-0 rounded-lg border border-border-0 hover:border-border-1 transition-colors cursor-pointer"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !name.trim()}
            className="px-4 py-2 text-xs font-medium text-white bg-accent-primary hover:bg-accent-primary/90 rounded-lg transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? 'Saving...' : project ? 'Save Changes' : 'Create Project'}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
