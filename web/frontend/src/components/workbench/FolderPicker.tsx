/**
 * FolderPicker
 *
 * Chooses a directory to open a terminal in.
 *
 * Starts from the workspace's attached context directories, since those are the
 * folders the user has already told OpenPaw they care about — for most launches
 * that is the whole journey. From there any subfolder can be browsed into, and
 * "Choose on computer" escapes to the native dialog for anywhere else.
 */

import { useCallback, useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { Folder, FolderOpen, ChevronRight, X, HardDrive, Loader2, CornerLeftUp } from 'lucide-react';
import { Button } from '../Button';
import { api } from '../../lib/api';
import { workspaces } from '../../lib/api-helpers';
import type { WorkspaceDirectory } from '../../lib/types';

interface PathInfo {
  path: string;
  name: string;
  parent: string;
  is_dir: boolean;
  children: { name: string; path: string }[];
}

interface FolderPickerProps {
  onPick: (path: string) => void;
  onClose: () => void;
}

export function FolderPicker({ onPick, onClose }: FolderPickerProps) {
  const [dirs, setDirs] = useState<WorkspaceDirectory[]>([]);
  const [current, setCurrent] = useState<PathInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Root view: the workspace's attached directories.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const active = await workspaces.getActive();
        const list = await workspaces.listDirectories(active.id);
        if (!cancelled) setDirs((list || []).filter(d => !d.missing));
      } catch {
        if (!cancelled) setDirs([]);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const browse = useCallback(async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      setCurrent(await api.get<PathInfo>(`/system/path-info?path=${encodeURIComponent(path)}`));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not open that folder');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  const chooseOnComputer = async () => {
    try {
      const { path } = await api.post<{ path: string }>('/system/pick-folder', {});
      if (path) onPick(path);
    } catch {
      /* dialog cancelled — leave the picker open */
    }
  };

  const atRoot = current === null;

  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm"
      onClick={e => { if (e.target === e.currentTarget) onClose(); }}
      role="dialog"
      aria-modal="true"
      aria-label="Choose a folder"
    >
      <div className="w-full max-w-lg max-h-[70vh] flex flex-col rounded-2xl border border-border-0 bg-surface-1 shadow-2xl overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-3 border-b border-border-0 flex-shrink-0">
          <FolderOpen className="w-4 h-4 text-accent-primary flex-shrink-0" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-text-0">Open terminal in folder</p>
            <p className="text-[11px] text-text-3 truncate" title={current?.path}>
              {current ? current.path : 'Your workspace directories'}
            </p>
          </div>
          <button
            onClick={onClose}
            aria-label="Close"
            className="p-1.5 rounded-lg text-text-2 hover:text-text-0 hover:bg-surface-2 transition-colors cursor-pointer flex-shrink-0"
          >
            <X className="w-4 h-4" aria-hidden="true" />
          </button>
        </div>

        {error && (
          <p className="px-4 py-2 text-xs text-danger bg-danger/10 border-b border-danger/20">{error}</p>
        )}

        <div className="flex-1 overflow-y-auto min-h-[200px]">
          {loading ? (
            <div className="h-full flex items-center justify-center py-10">
              <Loader2 className="w-5 h-5 text-text-3 animate-spin" aria-hidden="true" />
            </div>
          ) : atRoot ? (
            dirs.length === 0 ? (
              <p className="px-4 py-8 text-center text-xs text-text-3 leading-relaxed">
                No directories are attached to this workspace yet.
                <br />
                Use “Choose on computer” to pick any folder.
              </p>
            ) : (
              dirs.map(d => (
                <button
                  key={d.id}
                  onClick={() => browse(d.path)}
                  className="w-full flex items-center gap-2.5 px-4 py-2.5 text-left border-b border-border-0 last:border-0 hover:bg-surface-2 transition-colors cursor-pointer"
                >
                  <Folder className="w-4 h-4 text-accent-primary flex-shrink-0" aria-hidden="true" />
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm text-text-1 truncate">{d.label}</span>
                    <span className="block text-[11px] text-text-3 truncate">{d.path}</span>
                  </span>
                  <ChevronRight className="w-4 h-4 text-text-3 flex-shrink-0" aria-hidden="true" />
                </button>
              ))
            )
          ) : (
            <>
              <button
                onClick={() => (current.parent ? browse(current.parent) : setCurrent(null))}
                className="w-full flex items-center gap-2.5 px-4 py-2 text-left border-b border-border-0 text-text-2 hover:bg-surface-2 hover:text-text-1 transition-colors cursor-pointer"
              >
                <CornerLeftUp className="w-4 h-4 flex-shrink-0" aria-hidden="true" />
                <span className="text-xs">Up a level</span>
              </button>
              {current.children.length === 0 ? (
                <p className="px-4 py-8 text-center text-xs text-text-3">No subfolders here.</p>
              ) : (
                current.children.map(c => (
                  <button
                    key={c.path}
                    onClick={() => browse(c.path)}
                    className="w-full flex items-center gap-2.5 px-4 py-2.5 text-left border-b border-border-0 last:border-0 hover:bg-surface-2 transition-colors cursor-pointer"
                  >
                    <Folder className="w-4 h-4 text-text-3 flex-shrink-0" aria-hidden="true" />
                    <span className="flex-1 min-w-0 text-sm text-text-1 truncate">{c.name}</span>
                    <ChevronRight className="w-4 h-4 text-text-3 flex-shrink-0" aria-hidden="true" />
                  </button>
                ))
              )}
            </>
          )}
        </div>

        <div className="flex items-center gap-2 px-4 py-3 border-t border-border-0 flex-shrink-0">
          <Button
            variant="secondary"
            size="sm"
            icon={<HardDrive className="w-4 h-4" />}
            onClick={chooseOnComputer}
          >
            Choose on computer
          </Button>
          <Button
            size="sm"
            className="ml-auto"
            disabled={!current}
            onClick={() => current && onPick(current.path)}
          >
            Open here
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
