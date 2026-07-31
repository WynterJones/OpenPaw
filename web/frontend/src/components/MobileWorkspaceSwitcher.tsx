/**
 * MobileWorkspaceSwitcher
 *
 * The full workspace switcher lives in the sidebar, which is hidden below `md`
 * — leaving no way to change workspace on a phone at all. This is the compact
 * stand-in: an avatar-sized badge at the left of the header, matching the user
 * avatar on the right, opening a plain list of workspaces plus "New workspace".
 *
 * Renaming, deleting and workspace artwork stay in the sidebar. They are rare,
 * fiddly operations and a phone is the wrong place to do them.
 */

import { useEffect, useRef, useState } from 'react';
import { Check, Plus } from 'lucide-react';
import { workspaces } from '../lib/api-helpers';
import type { Workspace } from '../lib/types';

export function MobileWorkspaceSwitcher() {
  const [open, setOpen] = useState(false);
  const [list, setList] = useState<Workspace[]>([]);
  const [active, setActive] = useState<Workspace | null>(null);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.all([workspaces.list(), workspaces.getActive()])
      .then(([all, act]) => {
        if (cancelled) return;
        setList(Array.isArray(all) ? all : []);
        setActive(act ?? null);
      })
      .catch(() => {
        /* leave the switcher in its default state */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false);
    }
    document.addEventListener('mousedown', onClick);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onClick);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  // This switcher is rendered inside the global terminal screen. Update the
  // active workspace without reloading and destroying the live xterm canvases.
  const switchTo = async (id: string) => {
    setOpen(false);
    if (active && id === active.id) return;
    try {
      await workspaces.setActive(id);
      const next = list.find((ws) => ws.id === id) ?? null;
      setActive(next);
      window.dispatchEvent(new CustomEvent('openpaw:workspace-changed', { detail: next }));
    } catch {
      /* ignore */
    }
  };

  const submitCreate = async () => {
    const name = newName.trim();
    if (!name) return;
    try {
      const ws = await workspaces.create(name);
      await workspaces.setActive(ws.id);
      setList((prev) => [...prev, ws]);
      setActive(ws);
      setCreating(false);
      setNewName('');
      window.dispatchEvent(new CustomEvent('openpaw:workspace-changed', { detail: ws }));
    } catch {
      /* ignore */
    }
  };

  const activeName = active?.name ?? 'Workspace';

  const badge = (ws: Workspace | null, size: string) =>
    ws?.image_url ? (
      <img src={ws.image_url} alt="" className={`flex-shrink-0 ${size} rounded-md object-cover`} />
    ) : (
      <span
        className={`flex-shrink-0 ${size} rounded-md bg-accent-primary/15 text-accent-text text-xs font-bold flex items-center justify-center`}
      >
        {(ws?.name ?? activeName).charAt(0).toUpperCase() || 'W'}
      </span>
    );

  return (
    <div className="relative md:hidden flex-shrink-0" ref={ref}>
      <button
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-haspopup="menu"
        aria-label={`Switch workspace (current: ${activeName})`}
        title={activeName}
        className="w-8 h-8 rounded-md border border-border-1 overflow-hidden flex items-center justify-center bg-surface-2 cursor-pointer"
      >
        {badge(active, 'w-8 h-8')}
      </button>

      {open && (
        <div
          className="absolute left-0 top-full mt-1 w-56 max-w-[calc(100vw-2rem)] rounded-lg border border-border-0 bg-surface-1 shadow-xl py-1 z-50"
          role="menu"
        >
          <p className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-text-3">
            Workspaces
          </p>
          <div className="max-h-[50vh] overflow-y-auto">
            {list.length === 0 ? (
              <p className="px-3 py-1.5 text-xs text-text-3">No workspaces</p>
            ) : (
              list.map((ws) => (
                <button
                  key={ws.id}
                  role="menuitem"
                  onClick={() => switchTo(ws.id)}
                  className="w-full flex items-center gap-2.5 px-3 py-2.5 text-sm text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                >
                  {badge(ws, 'w-6 h-6')}
                  <span className="flex-1 min-w-0 text-left truncate">{ws.name}</span>
                  {active?.id === ws.id && (
                    <Check className="w-4 h-4 text-accent-text flex-shrink-0" aria-hidden="true" />
                  )}
                </button>
              ))
            )}
          </div>

          <div className="my-1 border-t border-border-0" />

          {creating ? (
            <div className="px-2 pb-1.5 space-y-1.5">
              <input
                autoFocus
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') submitCreate();
                  if (e.key === 'Escape') {
                    setCreating(false);
                    setNewName('');
                  }
                }}
                placeholder="Workspace name"
                className="w-full rounded-md border border-border-1 bg-surface-0 text-text-1 px-2.5 py-2 text-sm outline-none focus:border-accent-primary"
              />
              <div className="flex gap-1.5">
                <button
                  onClick={submitCreate}
                  disabled={!newName.trim()}
                  className="flex-1 rounded-md bg-accent-primary px-2 py-1.5 text-xs font-semibold text-white disabled:opacity-40 cursor-pointer"
                >
                  Create
                </button>
                <button
                  onClick={() => {
                    setCreating(false);
                    setNewName('');
                  }}
                  className="rounded-md border border-border-1 px-2.5 py-1.5 text-xs font-medium text-text-2 cursor-pointer"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <button
              role="menuitem"
              onClick={() => setCreating(true)}
              className="w-full flex items-center gap-2.5 px-3 py-2.5 text-sm text-text-2 hover:bg-surface-2 hover:text-text-1 transition-colors cursor-pointer"
            >
              <Plus className="w-4 h-4 flex-shrink-0 mx-1" aria-hidden="true" />
              New workspace
            </button>
          )}
        </div>
      )}
    </div>
  );
}
