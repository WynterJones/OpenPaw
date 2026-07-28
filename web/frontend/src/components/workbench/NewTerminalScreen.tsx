/**
 * NewTerminalScreen
 *
 * The "no terminals open" screen. Shown when a workspace has no terminals at
 * all, and on demand when a panel's "+" button is pressed — "+" asks for a
 * terminal, it doesn't spawn one, so the user still gets to say *where* it
 * should open.
 *
 * Opening a terminal for a dropped folder lives here and nowhere else. While a
 * terminal is open and focused a dropped folder is only pasted into it (see
 * terminal-manager's per-terminal drop handling); it must never spawn a second
 * terminal behind the user's back.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { TerminalSquare, Plus, FolderOpen } from 'lucide-react';
import { Button } from '../Button';
import { FolderPicker } from './FolderPicker';
import { useWorkbench } from './WorkbenchProvider';
import { api } from '../../lib/api';
import { isTauri, listenFolderDrop, type DropPoint } from '../../lib/tauri';

interface NewTerminalScreenProps {
  /** Panel the new terminal should open in. Omitted for the workspace-wide empty state. */
  panelId?: string;
  /** Fired once a terminal has been opened, so a host panel can hide this screen. */
  onOpened?: () => void;
}

export function NewTerminalScreen({ panelId, onOpened }: NewTerminalScreenProps) {
  const { createSession, launchSession } = useWorkbench();
  const [pickingFolder, setPickingFolder] = useState(false);
  const [dropActive, setDropActive] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  // Tauri drag events are window-wide, so with split panes every screen that is
  // open hears them. A panel-scoped screen must only answer for its own area,
  // or one drop would open a terminal in each of them.
  const isOurs = useCallback(
    (pos?: DropPoint) => {
      if (!panelId || !pos) return true; // workspace-wide screen, or no position reported
      const el = document.elementFromPoint(pos.x, pos.y);
      return !!el && !!rootRef.current && rootRef.current.contains(el);
    },
    [panelId],
  );

  const openInFolder = useCallback(
    async (path: string) => {
      setPickingFolder(false);
      // Resolve first: a drop can land on a file as easily as a folder, and
      // "open a terminal here" should mean the containing directory then.
      let cwd = path;
      try {
        const info = await api.get<{ path: string }>(
          `/system/path-info?path=${encodeURIComponent(path)}`,
        );
        cwd = info.path;
      } catch {
        /* fall back to the raw path and let the shell complain */
      }
      await launchSession(
        { title: cwd.split('/').filter(Boolean).pop() || 'Terminal', cwd },
        panelId,
      );
      onOpened?.();
    },
    [launchSession, panelId, onOpened],
  );

  const openBlank = useCallback(async () => {
    await createSession(panelId);
    onOpened?.();
  }, [createSession, panelId, onOpened]);

  // Folder drops carry a real path only in the desktop shell; a browser hands
  // over the file contents but never the location on disk.
  useEffect(() => {
    return listenFolderDrop({
      onEnter: (pos) => setDropActive(isOurs(pos)),
      onLeave: () => setDropActive(false),
      onDrop: (paths, pos) => {
        setDropActive(false);
        if (!isOurs(pos)) return;
        if (paths.length > 0) openInFolder(paths[0]);
      },
    });
  }, [openInFolder, isOurs]);

  return (
    <div
      ref={rootRef}
      className={`h-full w-full flex flex-col items-center justify-center gap-4 text-text-2 transition-colors ${
        dropActive ? 'bg-accent-primary/5 ring-2 ring-inset ring-accent-primary/40' : ''
      }`}
    >
      <TerminalSquare className="w-16 h-16 text-text-3" />
      <h2 className="text-xl font-semibold text-text-1">No terminals open</h2>
      <p className="text-sm">Create a terminal to get started.</p>
      <div className="flex items-center gap-2">
        <Button onClick={openBlank} icon={<Plus className="w-4 h-4" />}>
          New Terminal
        </Button>
        <Button
          variant="secondary"
          onClick={() => setPickingFolder(true)}
          icon={<FolderOpen className="w-4 h-4" />}
        >
          Select Folder
        </Button>
      </div>
      {isTauri() && (
        <p className="text-xs text-text-3">…or drop a folder here to open a terminal in it.</p>
      )}
      {pickingFolder && (
        <FolderPicker onPick={openInFolder} onClose={() => setPickingFolder(false)} />
      )}
    </div>
  );
}
