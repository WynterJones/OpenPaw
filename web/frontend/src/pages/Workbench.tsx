import { useState, useRef, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { Plus, X, Pencil } from 'lucide-react';
import { WorkbenchProvider, useWorkbench } from '../components/workbench/WorkbenchProvider';
import { PanelContainer } from '../components/workbench/PanelContainer';
import { ProjectsButton } from '../components/workbench/ProjectsPanel';
import { useDragReorder } from '../hooks/useDragReorder';
import type { Workbench as WorkbenchType } from '../lib/api';
import { NewTerminalScreen } from '../components/workbench/NewTerminalScreen';
import { MobileWorkspaceSwitcher } from '../components/MobileWorkspaceSwitcher';
import { useDesign } from '../contexts/DesignContext';

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

// ── Edit dropdown for workspace tabs ──

function WorkbenchEditDropdown({
  pos,
  name,
  color,
  onRename,
  onColorChange,
  onClose,
}: {
  pos: { top: number; left: number };
  name: string;
  color: string;
  onRename: (name: string) => void;
  onColorChange: (color: string) => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [editValue, setEditValue] = useState(name);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [onClose]);

  const commitRename = () => {
    const trimmed = editValue.trim();
    if (trimmed && trimmed !== name) onRename(trimmed);
    onClose();
  };

  return createPortal(
    <div
      ref={ref}
      className="fixed z-[9999] bg-surface-2 border border-border-1 rounded-lg shadow-xl p-2 flex flex-col gap-2 min-w-44"
      style={{ top: pos.top, left: pos.left }}
    >
      <input
        ref={inputRef}
        value={editValue}
        onChange={(e) => setEditValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') commitRename();
          if (e.key === 'Escape') onClose();
        }}
        className="bg-surface-1 border border-border-0 rounded-md text-xs text-text-0 px-2 py-1.5 outline-none focus:border-border-1 caret-accent-primary"
        placeholder="Workspace name"
      />
      <div className="flex gap-1.5 justify-center">
        {TAB_COLORS.map((c) => (
          <button
            key={c || 'none'}
            onClick={() => onColorChange(c)}
            className="w-5 h-5 rounded-full border-2 hover:scale-125 transition-transform cursor-pointer"
            style={{
              backgroundColor: c || 'var(--op-surface-3)',
              borderColor: color === c ? 'var(--op-text-0)' : 'var(--op-border-1)',
            }}
            title={c || 'None'}
          />
        ))}
      </div>
    </div>,
    document.body,
  );
}

// ── Workbench tab bar (top) ──

function WorkbenchHeader() {
  const {
    workbenches,
    activeWorkbenchId,
    switchWorkbench,
    createWorkbench,
    renameWorkbench,
    updateWorkbenchColor,
    deleteWorkbench,
    reorderWorkbenches,
  } = useWorkbench();

  const [editingId, setEditingId] = useState<string | null>(null);
  const [dropdownPos, setDropdownPos] = useState({ top: 0, left: 0 });
  const editBtnRefs = useRef<Map<string, HTMLButtonElement>>(new Map());

  const { dragId, overId, handleDragStart } = useDragReorder<WorkbenchType>({
    items: workbenches,
    getId: (w) => w.id,
    onReorder: reorderWorkbenches,
  });

  const openEdit = useCallback((id: string) => {
    const btn = editBtnRefs.current.get(id);
    if (btn) {
      const rect = btn.getBoundingClientRect();
      setDropdownPos({ top: rect.bottom + 4, left: rect.left });
    }
    setEditingId(id);
  }, []);

  const handleDelete = useCallback(
    async (id: string, e: React.MouseEvent) => {
      e.stopPropagation();
      if (workbenches.length <= 1) return;
      await deleteWorkbench(id);
    },
    [workbenches.length, deleteWorkbench],
  );

  const handleAdd = useCallback(async () => {
    const name = `Workspace ${workbenches.length + 1}`;
    await createWorkbench(name);
  }, [workbenches.length, createWorkbench]);

  return (
    <div className="flex items-center bg-surface-1 border-b border-border-0 pl-2 pr-1.5 h-12 md:h-10 gap-1 shrink-0">
      {/* The terminal screen has no app Header, so this is the only workspace
          switcher a phone can reach from here. It sits outside the scrolling
          strip below — an overflow container would clip its dropdown. */}
      <MobileWorkspaceSwitcher />

      <div className="flex items-center gap-1 op-scroll-x flex-1 min-w-0 h-full">
      {workbenches.map((wb) => {
        const isActive = wb.id === activeWorkbenchId;
        const color = wb.color || '';
        const isDragTarget = overId === wb.id && dragId !== wb.id;
        const isBeingDragged = dragId === wb.id;

        return (
          <div
            key={wb.id}
            data-drag-id={wb.id}
            onMouseDown={(e) => handleDragStart(wb.id, e)}
            onClick={() => { if (!isBeingDragged) switchWorkbench(wb.id); }}
            className={`group relative flex items-center gap-1.5 h-8 rounded-lg text-xs cursor-pointer select-none transition-all shrink-0 border ${
              isActive
                ? 'border-border-1 bg-surface-2 text-text-0 font-medium shadow-sm'
                : 'border-transparent hover:border-border-0 text-text-3 hover:bg-surface-2/50 hover:text-text-1'
            } ${isBeingDragged ? 'opacity-50' : ''} ${isDragTarget ? 'ring-2 ring-accent-primary/50' : ''}`}
            style={color ? {
              borderColor: isActive ? color : undefined,
              background: isActive
                ? `linear-gradient(135deg, ${color}12 0%, transparent 60%)`
                : undefined,
            } : undefined}
          >
            {/* Color accent bar */}
            {color && (
              <div
                className="absolute left-1 top-1/2 -translate-y-1/2 w-[3px] h-4 rounded-full"
                style={{ backgroundColor: color }}
              />
            )}

            <span className={`truncate max-w-28 ${color ? 'pl-4' : 'pl-2.5'} pr-1`}>{wb.name}</span>

            {/* Edit button */}
            <button
              ref={(el) => { if (el) editBtnRefs.current.set(wb.id, el); }}
              onClick={(e) => { e.stopPropagation(); openEdit(wb.id); }}
              className="op-touch-visible opacity-0 group-hover:opacity-100 flex items-center justify-center w-5 h-5 rounded hover:bg-surface-3 transition-all shrink-0 cursor-pointer"
              title="Edit workspace"
            >
              <Pencil className="w-2.5 h-2.5" />
            </button>

            {/* Close button */}
            {workbenches.length > 1 && (
              <button
                onClick={(e) => handleDelete(wb.id, e)}
                className="op-touch-visible opacity-0 group-hover:opacity-100 flex items-center justify-center w-5 h-5 rounded hover:bg-surface-3 hover:text-danger transition-all shrink-0 cursor-pointer pr-0.5"
                title="Close workspace"
              >
                <X className="w-2.5 h-2.5" />
              </button>
            )}
          </div>
        );
      })}

      <button
        onClick={handleAdd}
        className="flex items-center justify-center w-8 h-8 rounded-lg border border-transparent hover:border-border-0 text-text-3 hover:text-text-1 hover:bg-surface-2/50 transition-all shrink-0 cursor-pointer"
        title="New workspace"
      >
        <Plus className="w-3.5 h-3.5" />
      </button>
      </div>

      {/* Projects dropdown */}
      <ProjectsButton />

      {/* Edit dropdown */}
      {editingId && (() => {
        const wb = workbenches.find(w => w.id === editingId);
        if (!wb) return null;
        return (
          <WorkbenchEditDropdown
            pos={dropdownPos}
            name={wb.name}
            color={wb.color || ''}
            onRename={(name) => renameWorkbench(editingId, name)}
            onColorChange={(color) => updateWorkbenchColor(editingId, color)}
            onClose={() => setEditingId(null)}
          />
        );
      })()}
    </div>
  );
}

// ── Main content area ──

function WorkbenchContent() {
  const { sessions, rootPanel, loading } = useWorkbench();

  if (loading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="animate-spin w-6 h-6 border-2 border-accent-primary border-t-transparent rounded-full" />
      </div>
    );
  }

  if (!rootPanel || sessions.length === 0) {
    return (
      <div className="flex-1 min-h-0 bg-surface-0/75">
        <NewTerminalScreen />
      </div>
    );
  }

  return (
    <div className="flex-1 min-h-0">
      <PanelContainer node={rootPanel} />
    </div>
  );
}

export function Workbench() {
  const { bgImage, previewBg } = useDesign();
  const shownBackground = previewBg ?? bgImage;

  return (
    <WorkbenchProvider>
      <div className="relative h-full overflow-hidden">
        {shownBackground && (
          <div
            className="pointer-events-none absolute inset-0 bg-cover bg-center bg-no-repeat"
            style={{ backgroundImage: `url(${shownBackground})` }}
            aria-hidden="true"
          />
        )}
        <div className="relative z-[1] flex h-full flex-col">
          <WorkbenchHeader />
          <WorkbenchContent />
        </div>
      </div>
    </WorkbenchProvider>
  );
}
