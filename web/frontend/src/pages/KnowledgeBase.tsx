import { useState, useEffect, useCallback } from 'react';
import { useSearchParams } from 'react-router';
import { BookOpen, ImageIcon, UserPen, FolderTree, Folder, FolderOpen, File, ChevronRight, ChevronDown, FolderPlus, Trash2 } from 'lucide-react';
import { Header } from '../components/Header';
import { EmptyState } from '../components/EmptyState';
import { FileEditorModal } from '../components/FileEditorModal';
import { ContextPanel } from './Context';
import { MediaLibraryPanel } from './MediaLibrary';
import { api } from '../lib/api';
import { workspaces } from '../lib/api-helpers';
import type { WorkspaceFileNode, WorkspaceDirectory } from '../lib/types';

type KnowledgeTab = 'context' | 'directory' | 'media' | 'about';

const knowledgeTabs: { key: KnowledgeTab; label: string; icon: typeof BookOpen }[] = [
  { key: 'context', label: 'Files', icon: BookOpen },
  { key: 'directory', label: 'Directory', icon: FolderTree },
  { key: 'media', label: 'Media', icon: ImageIcon },
  { key: 'about', label: 'About You', icon: UserPen },
];

function initialTabFrom(param: string | null): KnowledgeTab {
  if (param === 'media') return 'media';
  if (param === 'about') return 'about';
  if (param === 'directory') return 'directory';
  return 'context';
}

function formatSize(bytes: number): string {
  if (!bytes) return '';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// One tree row. Folders start COLLAPSED and load their immediate children on
// first expand (via loadChildren) — the backend lists a single level at a time,
// so opening a repo never walks node_modules/.git.
function DirTreeNode({
  node,
  level,
  loadChildren,
  onOpenFile,
}: {
  node: WorkspaceFileNode;
  level: number;
  loadChildren: (path: string) => Promise<WorkspaceFileNode[]>;
  onOpenFile: (node: WorkspaceFileNode) => void;
}) {
  const [open, setOpen] = useState(false);
  const [children, setChildren] = useState<WorkspaceFileNode[] | null>(node.children ?? null);
  const [loading, setLoading] = useState(false);
  const pad = { paddingLeft: `${8 + level * 16}px` };

  const toggle = async () => {
    const next = !open;
    setOpen(next);
    if (next && children === null && !loading) {
      setLoading(true);
      try {
        setChildren(await loadChildren(node.path));
      } catch {
        setChildren([]);
      } finally {
        setLoading(false);
      }
    }
  };

  if (node.is_dir) {
    return (
      <div>
        <button
          onClick={toggle}
          style={pad}
          className="w-full flex items-center gap-1.5 rounded-lg pr-2 py-1.5 text-sm text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
        >
          {open ? (
            <ChevronDown className="w-3.5 h-3.5 text-text-3 flex-shrink-0" aria-hidden="true" />
          ) : (
            <ChevronRight className="w-3.5 h-3.5 text-text-3 flex-shrink-0" aria-hidden="true" />
          )}
          {open ? (
            <FolderOpen className="w-4 h-4 text-accent-primary flex-shrink-0" aria-hidden="true" />
          ) : (
            <Folder className="w-4 h-4 text-text-2 flex-shrink-0" aria-hidden="true" />
          )}
          <span className="flex-1 min-w-0 truncate text-left">{node.name}</span>
          {loading && (
            <span className="w-3 h-3 border-2 border-text-3 border-t-transparent rounded-full animate-spin flex-shrink-0" aria-hidden="true" />
          )}
        </button>
        {open && children && children.length > 0 && (
          <div>
            {children.map((child) => (
              <DirTreeNode
                key={child.path}
                node={child}
                level={level + 1}
                loadChildren={loadChildren}
                onOpenFile={onOpenFile}
              />
            ))}
          </div>
        )}
        {open && !loading && children && children.length === 0 && (
          <div style={{ paddingLeft: `${8 + (level + 1) * 16}px` }} className="py-1 text-[11px] text-text-3 italic">
            empty
          </div>
        )}
      </div>
    );
  }

  return (
    <button
      onClick={() => onOpenFile(node)}
      style={pad}
      title={`Open ${node.name}`}
      className="group w-full flex items-center gap-1.5 rounded-lg pr-2 py-1.5 text-sm text-text-2 hover:bg-surface-2 hover:text-text-1 transition-colors cursor-pointer text-left"
    >
      <span className="w-3.5 flex-shrink-0" aria-hidden="true" />
      <File className="w-4 h-4 text-text-3 group-hover:text-accent-primary flex-shrink-0 transition-colors" aria-hidden="true" />
      <span className="flex-1 min-w-0 truncate">{node.name}</span>
      {node.size > 0 && (
        <span className="text-[11px] text-text-3 flex-shrink-0">{formatSize(node.size)}</span>
      )}
    </button>
  );
}

function AttachedDirectorySection({
  dir,
  onRemove,
  removing,
  loadChildren,
  onOpenFile,
}: {
  dir: WorkspaceDirectory;
  onRemove: () => void;
  removing: boolean;
  loadChildren: (path: string) => Promise<WorkspaceFileNode[]>;
  onOpenFile: (node: WorkspaceFileNode) => void;
}) {
  return (
    <div className="mb-5">
      <div className="flex items-center justify-between gap-2 px-1 mb-1.5">
        <div className="min-w-0 flex items-baseline gap-2">
          <span className="text-xs font-semibold text-text-1 truncate">{dir.label}</span>
          <span className="text-[11px] text-text-3 truncate">{dir.path}</span>
        </div>
        <button
          onClick={onRemove}
          disabled={removing}
          className="inline-flex items-center justify-center rounded-lg p-1.5 text-text-3 hover:text-red-500 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-50 flex-shrink-0"
          title="Remove this directory from the workspace"
          aria-label={`Remove ${dir.label}`}
        >
          <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
        </button>
      </div>
      {dir.missing ? (
        <p className="px-1 text-xs text-text-3 italic">Folder not found — it may have been moved or deleted.</p>
      ) : dir.files.length === 0 ? (
        <p className="px-1 text-xs text-text-3 italic">Empty folder.</p>
      ) : (
        <div>
          {dir.files.map((node) => (
            <DirTreeNode
              key={node.path}
              node={node}
              level={0}
              loadChildren={loadChildren}
              onOpenFile={onOpenFile}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function WorkspaceDirectoryPanel() {
  const [tree, setTree] = useState<WorkspaceFileNode[]>([]);
  const [dirs, setDirs] = useState<WorkspaceDirectory[]>([]);
  const [loading, setLoading] = useState(true);
  const [wsId, setWsId] = useState<string | null>(null);
  const [revealing, setRevealing] = useState(false);
  const [adding, setAdding] = useState(false);
  const [removingId, setRemovingId] = useState<string | null>(null);
  const [editing, setEditing] = useState<{ dirId: string; path: string; name: string } | null>(null);

  const refresh = useCallback(async (id: string) => {
    const [filesRes, dirsRes] = await Promise.all([
      workspaces.listFiles(id).catch(() => ({ files: [] as WorkspaceFileNode[] })),
      workspaces.listDirectories(id).catch(() => [] as WorkspaceDirectory[]),
    ]);
    setTree(Array.isArray(filesRes?.files) ? filesRes.files : []);
    setDirs(Array.isArray(dirsRes) ? dirsRes : []);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const active = await workspaces.getActive();
        if (cancelled) return;
        setWsId(active.id);
        await refresh(active.id);
      } catch {
        if (!cancelled) {
          setTree([]);
          setDirs([]);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [refresh]);

  const reveal = async () => {
    if (!wsId) return;
    setRevealing(true);
    try {
      await workspaces.reveal(wsId);
    } catch {
      /* ignore */
    } finally {
      setRevealing(false);
    }
  };

  const addDirectory = async () => {
    if (!wsId) return;
    setAdding(true);
    try {
      const result = await api.post<{ path: string }>('/system/pick-folder', {});
      if (result.path) {
        await workspaces.addDirectory(wsId, result.path);
        await refresh(wsId);
      }
    } catch {
      // dialog cancelled or failed — ignore
    } finally {
      setAdding(false);
    }
  };

  const removeDirectory = async (dirId: string) => {
    if (!wsId) return;
    setRemovingId(dirId);
    try {
      await workspaces.removeDirectory(wsId, dirId);
      await refresh(wsId);
    } catch {
      /* ignore */
    } finally {
      setRemovingId(null);
    }
  };

  const toolbar = (
    <div className="flex items-center justify-end gap-2 px-2 md:px-4 py-2 border-b border-border-0 flex-shrink-0">
      <button
        onClick={addDirectory}
        disabled={!wsId || adding}
        className="inline-flex items-center gap-1.5 rounded-lg border border-border-1 bg-surface-2 hover:bg-surface-3 text-text-1 px-2.5 py-1.5 text-xs font-medium transition-colors cursor-pointer disabled:opacity-50"
        title="Attach an external directory to this workspace"
      >
        <FolderPlus className="w-4 h-4" aria-hidden="true" />
        Add directory
      </button>
      <button
        onClick={reveal}
        disabled={!wsId || revealing}
        className="inline-flex items-center gap-1.5 rounded-lg border border-border-1 bg-surface-2 hover:bg-surface-3 text-text-1 px-2.5 py-1.5 text-xs font-medium transition-colors cursor-pointer disabled:opacity-50"
        title="Open this workspace's files folder in your file manager"
      >
        <FolderOpen className="w-4 h-4" aria-hidden="true" />
        Open in Finder
      </button>
    </div>
  );

  if (loading) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      {toolbar}
      {tree.length === 0 && dirs.length === 0 ? (
        <div className="flex-1 flex items-center justify-center">
          <EmptyState
            icon={<FolderTree className="w-8 h-8" />}
            title="No files yet"
            description="No files yet in this workspace. Add an external directory to give agents access to more of your filesystem."
          />
        </div>
      ) : (
        <div className="flex-1 overflow-y-auto px-2 md:px-4 py-3">
          <div className="max-w-2xl mx-auto">
            {tree.length > 0 && (
              <div className="mb-5">
                {dirs.length > 0 && (
                  <h3 className="px-1 mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-text-3">
                    Workspace files
                  </h3>
                )}
                {tree.map((node) => (
                  <DirTreeNode
                    key={node.path}
                    node={node}
                    level={0}
                    loadChildren={(p) => workspaces.browse(wsId!, '', p).then((r) => r.files)}
                    onOpenFile={(f) => setEditing({ dirId: '', path: f.path, name: f.name })}
                  />
                ))}
              </div>
            )}
            {dirs.map((dir) => (
              <AttachedDirectorySection
                key={dir.id}
                dir={dir}
                onRemove={() => removeDirectory(dir.id)}
                removing={removingId === dir.id}
                loadChildren={(p) => workspaces.browse(wsId!, dir.id, p).then((r) => r.files)}
                onOpenFile={(f) => setEditing({ dirId: dir.id, path: f.path, name: f.name })}
              />
            ))}
          </div>
        </div>
      )}
      {editing && wsId && (
        <FileEditorModal
          key={`${editing.dirId}:${editing.path}`}
          workspaceId={wsId}
          dirId={editing.dirId}
          path={editing.path}
          name={editing.name}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}

export function KnowledgeBase() {
  const [searchParams] = useSearchParams();
  const [tab, setTab] = useState<KnowledgeTab>(initialTabFrom(searchParams.get('tab')));

  return (
    <div className="flex flex-col h-full">
      <Header title="Context" />
      <div className="flex items-center gap-2 px-4 md:px-6 border-b border-border-0 overflow-x-auto flex-shrink-0">
        {knowledgeTabs.map(t => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`px-4 py-2.5 text-sm font-medium transition-colors relative cursor-pointer ${
              tab === t.key ? 'text-text-0' : 'text-text-3 hover:text-text-1'
            }`}
          >
            <span className="flex items-center gap-2">
              <t.icon className="w-4 h-4" />
              {t.label}
            </span>
            {tab === t.key && (
              <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-accent-primary rounded-t" />
            )}
          </button>
        ))}
      </div>

      <div className="flex-1 min-h-0">
        {tab === 'context' && <ContextPanel view="files" />}
        {tab === 'directory' && <WorkspaceDirectoryPanel />}
        {tab === 'about' && <ContextPanel view="about" />}
        {tab === 'media' && <MediaLibraryPanel />}
      </div>
    </div>
  );
}
