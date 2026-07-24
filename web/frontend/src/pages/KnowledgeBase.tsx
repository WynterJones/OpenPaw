import { useState, useEffect } from 'react';
import { useSearchParams } from 'react-router';
import { BookOpen, ImageIcon, UserPen, FolderTree, Folder, FolderOpen, File, ChevronRight, ChevronDown } from 'lucide-react';
import { Header } from '../components/Header';
import { EmptyState } from '../components/EmptyState';
import { ContextPanel } from './Context';
import { MediaLibraryPanel } from './MediaLibrary';
import { workspaces } from '../lib/api-helpers';
import type { WorkspaceFileNode } from '../lib/types';

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

function DirTreeNode({ node, level }: { node: WorkspaceFileNode; level: number }) {
  const [open, setOpen] = useState(true);
  const pad = { paddingLeft: `${8 + level * 16}px` };

  if (node.is_dir) {
    return (
      <div>
        <button
          onClick={() => setOpen((o) => !o)}
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
        </button>
        {open && node.children && node.children.length > 0 && (
          <div>
            {node.children.map((child) => (
              <DirTreeNode key={child.path} node={child} level={level + 1} />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div style={pad} className="flex items-center gap-1.5 rounded-lg pr-2 py-1.5 text-sm text-text-2">
      <span className="w-3.5 flex-shrink-0" aria-hidden="true" />
      <File className="w-4 h-4 text-text-3 flex-shrink-0" aria-hidden="true" />
      <span className="flex-1 min-w-0 truncate">{node.name}</span>
      {node.size > 0 && (
        <span className="text-[11px] text-text-3 flex-shrink-0">{formatSize(node.size)}</span>
      )}
    </div>
  );
}

function WorkspaceDirectoryPanel() {
  const [tree, setTree] = useState<WorkspaceFileNode[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const active = await workspaces.getActive();
        const res = await workspaces.listFiles(active.id);
        if (!cancelled) setTree(Array.isArray(res?.files) ? res.files : []);
      } catch {
        if (!cancelled) setTree([]);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (tree.length === 0) {
    return (
      <div className="h-full flex items-center justify-center">
        <EmptyState
          icon={<FolderTree className="w-8 h-8" />}
          title="No files yet"
          description="No files yet in this workspace."
        />
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto px-2 md:px-4 py-3">
      <div className="max-w-2xl mx-auto">
        {tree.map((node) => (
          <DirTreeNode key={node.path} node={node} level={0} />
        ))}
      </div>
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
