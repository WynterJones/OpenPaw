import { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import {
  ArrowRight,
  File,
  Folder,
  FolderSearch,
  MessageSquarePlus,
  Search,
  Settings2,
} from 'lucide-react';
import { APP_NAV_ITEMS, hotkeyLabel, navigationHotkeyLabel } from '../lib/app-navigation';
import { workspaces } from '../lib/api-helpers';
import type { Workspace, WorkspaceSearchResult } from '../lib/types';
import { getPathInsertionTarget, type PathInsertionTarget } from '../lib/path-insertion';
import { useHotkeys } from '../contexts/hotkeys';
import { useToast } from './Toast';

type PaletteItem =
  | { kind: 'nav'; id: string; label: string; description: string; group: string; to: string; icon: typeof Search; keyCode?: string; keywords: string }
  | { kind: 'action'; id: string; label: string; description: string; group: string; action: 'new-chat'; icon: typeof Search; keyCode?: string; keywords: string }
  | { kind: 'file'; id: string; result: WorkspaceSearchResult };

const settingsLinks: PaletteItem[] = [
  ['keyboard', 'Keyboard shortcuts', 'Change the super key, bindings, and badges'],
  ['models', 'AI Models', 'Configure language and media models'],
  ['design', 'Design', 'Theme, appearance, and background'],
  ['notifications', 'Notifications', 'Sounds and browser notifications'],
].map(([tab, label, description]) => ({
  kind: 'nav' as const,
  id: `settings-${tab}`,
  label,
  description,
  group: 'Settings',
  to: `/settings?tab=${tab}`,
  icon: Settings2,
  keywords: `${label} ${description}`.toLowerCase(),
}));

function Highlight({ text, query }: { text: string; query: string }) {
  const q = query.trim().toLowerCase();
  if (!q) return <>{text}</>;
  const index = text.toLowerCase().indexOf(q);
  if (index < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, index)}
      <mark className="rounded-sm bg-accent-primary/25 text-inherit">{text.slice(index, index + q.length)}</mark>
      {text.slice(index + q.length)}
    </>
  );
}

export function CommandPalette() {
  const navigate = useNavigate();
  const { toast } = useToast();
  const {
    paletteOpen,
    setPaletteOpen,
    modifier,
    bindings,
    runNewChat,
  } = useHotkeys();
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [files, setFiles] = useState<WorkspaceSearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const insertionTargetRef = useRef<PathInsertionTarget | null>(null);
  const fileMode = query.startsWith('!');
  const fileQuery = fileMode ? query.slice(1).trim() : '';

  useEffect(() => {
    if (!paletteOpen) return;
    insertionTargetRef.current = getPathInsertionTarget();
    workspaces.getActive().then(setWorkspace).catch(() => setWorkspace(null));
    const frame = requestAnimationFrame(() => {
      setQuery('');
      setFiles([]);
      setActiveIndex(0);
      inputRef.current?.focus();
    });
    return () => cancelAnimationFrame(frame);
  }, [paletteOpen]);

  useEffect(() => {
    if (!paletteOpen || !fileMode || !workspace) return;
    let cancelled = false;
    const timer = window.setTimeout(() => {
      setSearching(true);
      workspaces.search(workspace.id, fileQuery)
        .then(results => { if (!cancelled) setFiles(results); })
        .catch(() => { if (!cancelled) setFiles([]); })
        .finally(() => { if (!cancelled) setSearching(false); });
    }, fileQuery ? 120 : 0);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [fileMode, fileQuery, paletteOpen, workspace]);

  const navigationItems = useMemo<PaletteItem[]>(() => {
    const main: PaletteItem[] = APP_NAV_ITEMS.map(item => ({
      kind: 'nav',
      id: item.id,
      label: item.label,
      description: item.description,
      group: item.group,
      to: item.to,
      icon: item.icon,
      keyCode: navigationHotkeyLabel(modifier, bindings[item.id] || item.defaultKey),
      keywords: `${item.label} ${item.description} ${(item.keywords || []).join(' ')}`.toLowerCase(),
    }));
    main.unshift({
      kind: 'action',
      id: 'new-chat',
      label: 'New chat',
      description: 'Start a fresh conversation',
      group: 'Actions',
      action: 'new-chat',
      icon: MessageSquarePlus,
      keyCode: hotkeyLabel(modifier, 'N'),
      keywords: 'new chat conversation',
    });
    return [...main, ...settingsLinks];
  }, [bindings, modifier]);

  const items = useMemo<PaletteItem[]>(() => {
    if (fileMode) {
      return files.map(result => ({
        kind: 'file',
        id: `${result.dir_id}:${result.path}`,
        result,
      }));
    }
    const q = query.trim().toLowerCase();
    if (!q) return navigationItems;
    return navigationItems.filter(item => item.kind !== 'file' && item.keywords.includes(q));
  }, [fileMode, files, navigationItems, query]);

  if (!paletteOpen) return null;

  const close = () => setPaletteOpen(false);
  const run = (item: PaletteItem, insert = false) => {
    if (item.kind === 'action') {
      close();
      runNewChat();
      return;
    }
    if (item.kind === 'nav') {
      close();
      navigate(item.to);
      return;
    }
    const result = item.result;
    if (insert) {
      const target = insertionTargetRef.current;
      if (!target) {
        toast('error', 'Focus a chat, terminal, or editor before opening the palette');
        return;
      }
      close();
      target.insert(result.absolute_path);
      toast('success', `Path inserted into ${target.label}`);
      return;
    }
    close();
    const params = new URLSearchParams({ tab: 'directory', dir: result.dir_id });
    if (result.is_dir) params.set('focus', result.path);
    else params.set('file', result.path);
    navigate(`/knowledge-base?${params.toString()}`);
  };

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault();
      close();
      return;
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setActiveIndex(index => Math.min(items.length - 1, index + 1));
      return;
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault();
      setActiveIndex(index => Math.max(0, index - 1));
      return;
    }
    if (event.key === 'Enter' && items[activeIndex]) {
      event.preventDefault();
      run(items[activeIndex], event.shiftKey && items[activeIndex].kind === 'file');
    }
  };

  return createPortal(
    <div
      className="fixed inset-0 z-[120] flex items-start justify-center bg-black/60 px-3 pt-[8vh] md:pt-[12vh] backdrop-blur-sm"
      onMouseDown={event => { if (event.target === event.currentTarget) close(); }}
      role="dialog"
      aria-modal="true"
      aria-label="Command palette"
    >
      <div className="w-full max-w-2xl overflow-hidden rounded-2xl border border-border-1 bg-surface-1 shadow-2xl">
        <div className="flex items-center gap-3 border-b border-border-0 px-4">
          {fileMode ? <FolderSearch className="h-5 w-5 flex-shrink-0 text-accent-primary" /> : <Search className="h-5 w-5 flex-shrink-0 text-text-3" />}
          <input
            ref={inputRef}
            value={query}
            onChange={event => {
              setQuery(event.target.value);
              setActiveIndex(0);
            }}
            onKeyDown={onKeyDown}
            placeholder={fileMode ? `Search ${workspace?.name || 'workspace'} files…` : 'Go somewhere or type ! to search files…'}
            className="h-14 min-w-0 flex-1 border-0 bg-transparent text-base text-text-0 outline-none placeholder:text-text-3 focus:ring-0"
            aria-controls="command-palette-results"
            aria-activedescendant={items[activeIndex] ? `command-${items[activeIndex].id}` : undefined}
          />
          <button
            onClick={() => {
              setQuery(fileMode ? '' : '!');
              setActiveIndex(0);
            }}
            className={`inline-flex h-8 items-center gap-1.5 rounded-lg border px-2.5 text-xs font-semibold transition-colors cursor-pointer ${
              fileMode ? 'border-accent-primary/40 bg-accent-muted text-accent-text' : 'border-border-1 bg-surface-2 text-text-2 hover:text-text-0'
            }`}
            aria-pressed={fileMode}
            title="Search this workspace's directory"
          >
            <span className="font-mono">!</span>
            Files
          </button>
        </div>

        <div id="command-palette-results" role="listbox" className="max-h-[min(60vh,520px)] overflow-y-auto p-2">
          {items.map((item, index) => {
            const group = item.kind === 'file' ? item.result.source : item.group;
            const previous = index > 0 ? items[index - 1] : null;
            const previousGroup = previous
              ? (previous.kind === 'file' ? previous.result.source : previous.group)
              : '';
            const showGroup = group !== previousGroup;
            const Icon = item.kind === 'file' ? (item.result.is_dir ? Folder : File) : item.icon;
            const label = item.kind === 'file' ? item.result.name : item.label;
            const description = item.kind === 'file' ? item.result.path : item.description;
            return (
              <div key={item.id}>
                {showGroup && (
                  <p className="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-text-3">
                    {group}
                  </p>
                )}
                <button
                  id={`command-${item.id}`}
                  role="option"
                  aria-selected={index === activeIndex}
                  onMouseMove={() => setActiveIndex(index)}
                  onClick={() => run(item)}
                  className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors cursor-pointer ${
                    index === activeIndex ? 'bg-accent-muted' : 'hover:bg-surface-2'
                  }`}
                >
                  <span className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg ${
                    index === activeIndex ? 'bg-accent-primary/15 text-accent-text' : 'bg-surface-2 text-text-2'
                  }`}>
                    <Icon className="h-4 w-4" aria-hidden="true" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium text-text-0">
                      <Highlight text={label} query={fileMode ? fileQuery : query} />
                    </span>
                    <span className="block truncate text-xs text-text-3">
                      <Highlight text={description} query={fileMode ? fileQuery : query} />
                    </span>
                  </span>
                  {item.kind !== 'file' && item.keyCode && (
                    <kbd className="flex-shrink-0 rounded-md border border-border-1 bg-surface-2 px-1.5 py-1 text-[10px] font-medium text-text-3">
                      {item.keyCode}
                    </kbd>
                  )}
                  {item.kind === 'file' && index === activeIndex && <ArrowRight className="h-4 w-4 flex-shrink-0 text-text-3" />}
                </button>
              </div>
            );
          })}
          {items.length === 0 && (
            <div className="px-6 py-12 text-center">
              <FolderSearch className="mx-auto mb-3 h-8 w-8 text-text-3" aria-hidden="true" />
              <p className="text-sm font-medium text-text-1">{searching ? 'Searching…' : 'No matches in this workspace'}</p>
              <p className="mt-1 text-xs text-text-3">Try part of a filename or folder path.</p>
            </div>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border-0 bg-surface-0/50 px-4 py-2 text-[11px] text-text-3">
          <span className="truncate">{fileMode ? `Only ${workspace?.name || 'the active workspace'} and its attached directories` : 'Quick links are scoped to the active workspace'}</span>
          <span className="flex items-center gap-2">
            <span><kbd className="font-mono">↑↓</kbd> choose</span>
            <span><kbd className="font-mono">Enter</kbd> open</span>
            {fileMode && <span><kbd className="font-mono">⇧ Enter</kbd> insert path</span>}
            <span><kbd className="font-mono">Esc</kbd> close</span>
          </span>
        </div>
      </div>
    </div>,
    document.body,
  );
}
