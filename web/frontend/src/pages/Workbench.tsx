import { useState, useRef, useEffect, useCallback } from 'react';
import { TerminalSquare, Plus, X } from 'lucide-react';
import { Button } from '../components/Button';
import { WorkbenchProvider, useWorkbench } from '../components/workbench/WorkbenchProvider';
import { PanelContainer } from '../components/workbench/PanelContainer';

// ── Workbench tab bar (top) ──

function WorkbenchHeader() {
  const {
    workbenches,
    activeWorkbenchId,
    switchWorkbench,
    createWorkbench,
    renameWorkbench,
    deleteWorkbench,
  } = useWorkbench();

  const [editingId, setEditingId] = useState<string | null>(null);
  const [editValue, setEditValue] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editingId && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [editingId]);

  const startEdit = useCallback((id: string, name: string) => {
    setEditingId(id);
    setEditValue(name);
  }, []);

  const commitEdit = useCallback(async () => {
    if (!editingId) return;
    const trimmed = editValue.trim();
    if (trimmed) await renameWorkbench(editingId, trimmed);
    setEditingId(null);
  }, [editingId, editValue, renameWorkbench]);

  const handleEditKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') commitEdit();
      else if (e.key === 'Escape') setEditingId(null);
    },
    [commitEdit],
  );

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
    <div className="flex items-center bg-surface-1 border-b border-border-0 px-1.5 h-9 gap-0.5 overflow-x-auto shrink-0">
      {workbenches.map((wb) => {
        const isActive = wb.id === activeWorkbenchId;
        const isEditing = editingId === wb.id;

        return (
          <div
            key={wb.id}
            onClick={() => !isEditing && switchWorkbench(wb.id)}
            onDoubleClick={() => startEdit(wb.id, wb.name)}
            className={`group relative flex items-center gap-1.5 px-3 h-7 rounded-md text-xs cursor-pointer select-none transition-all shrink-0 ${
              isActive
                ? 'bg-surface-2 text-text-0 font-medium shadow-[inset_0_1px_0_0_rgba(255,255,255,0.04)]'
                : 'text-text-3 hover:bg-surface-2/50 hover:text-text-1'
            }`}
          >
            {isEditing ? (
              <input
                ref={inputRef}
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                onBlur={commitEdit}
                onKeyDown={handleEditKeyDown}
                className="bg-transparent border-none outline-none text-xs text-text-0 w-20 min-w-0 p-0 caret-accent-primary"
                onClick={(e) => e.stopPropagation()}
              />
            ) : (
              <span className="truncate max-w-28">{wb.name}</span>
            )}

            {workbenches.length > 1 && !isEditing && (
              <button
                onClick={(e) => handleDelete(wb.id, e)}
                className="opacity-0 group-hover:opacity-100 flex items-center justify-center w-4 h-4 rounded hover:bg-surface-3 hover:text-danger transition-all shrink-0 cursor-pointer"
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
        className="flex items-center justify-center w-7 h-7 rounded-md text-text-3 hover:text-text-1 hover:bg-surface-2/50 transition-colors shrink-0 cursor-pointer"
        title="New workspace"
      >
        <Plus className="w-3.5 h-3.5" />
      </button>
    </div>
  );
}

// ── Main content area ──

function WorkbenchContent() {
  const { sessions, rootPanel, createSession, loading } = useWorkbench();

  if (loading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="animate-spin w-6 h-6 border-2 border-accent-primary border-t-transparent rounded-full" />
      </div>
    );
  }

  if (!rootPanel || sessions.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center gap-4 text-text-2">
        <TerminalSquare className="w-16 h-16 text-text-3" />
        <h2 className="text-xl font-semibold text-text-1">No terminals open</h2>
        <p className="text-sm">Create a terminal to get started.</p>
        <Button
          onClick={() => createSession()}
          icon={<Plus className="w-4 h-4" />}
        >
          New Terminal
        </Button>
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
  return (
    <WorkbenchProvider>
      <div className="flex flex-col h-full">
        <WorkbenchHeader />
        <WorkbenchContent />
      </div>
    </WorkbenchProvider>
  );
}
