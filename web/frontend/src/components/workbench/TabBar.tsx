import { useState, useRef, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { X, Plus, Columns2, Rows2, Pencil, Loader2 } from 'lucide-react';
import { useWorkbench, type PanelNode } from './WorkbenchProvider';

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

interface TabBarProps {
  node: PanelNode;
}

export function TabBar({ node }: TabBarProps) {
  const {
    sessions,
    activeSessionId,
    activateTab,
    closeSession,
    createSession,
    splitPanel,
    updateSession,
    busySessions,
  } = useWorkbench();

  const tabs = node.tabs || [];

  return (
    <div className="flex items-center bg-surface-1 border-b border-border-0 px-1.5 gap-0.5 h-9 overflow-x-auto shrink-0">
      {tabs.map((sessionId) => {
        const session = sessions.find((s) => s.id === sessionId);
        const title = session?.title || 'Terminal';
        const color = session?.color || '';
        const isActive = sessionId === node.activeTab;
        const isBusy = busySessions.has(sessionId);

        return (
          <Tab
            key={sessionId}
            sessionId={sessionId}
            title={title}
            color={color}
            isActive={isActive}
            isGloballyActive={sessionId === activeSessionId}
            isBusy={isBusy}
            panelId={node.id}
            onActivate={activateTab}
            onClose={closeSession}
            onUpdate={updateSession}
          />
        );
      })}

      <button
        onClick={() => createSession(node.id)}
        className="flex items-center justify-center w-7 h-7 rounded-md text-text-3 hover:text-text-1 hover:bg-surface-2/50 transition-colors shrink-0 cursor-pointer"
        title="New terminal"
      >
        <Plus className="w-3.5 h-3.5" />
      </button>

      <div className="flex-1" />

      <button
        onClick={() => splitPanel(node.id, 'horizontal')}
        className="flex items-center justify-center w-7 h-7 rounded-md text-text-3 hover:text-text-1 hover:bg-surface-2/50 transition-colors shrink-0 cursor-pointer"
        title="Split horizontally"
      >
        <Columns2 className="w-3.5 h-3.5" />
      </button>
      <button
        onClick={() => splitPanel(node.id, 'vertical')}
        className="flex items-center justify-center w-7 h-7 rounded-md text-text-3 hover:text-text-1 hover:bg-surface-2/50 transition-colors shrink-0 cursor-pointer"
        title="Split vertically"
      >
        <Rows2 className="w-3.5 h-3.5" />
      </button>
    </div>
  );
}

// ── Individual Tab ──

interface TabProps {
  sessionId: string;
  title: string;
  color: string;
  isActive: boolean;
  isGloballyActive: boolean;
  isBusy: boolean;
  panelId: string;
  onActivate: (panelId: string, sessionId: string) => void;
  onClose: (sessionId: string) => Promise<void>;
  onUpdate: (sessionId: string, data: { title?: string; color?: string }) => Promise<void>;
}

function Tab({
  sessionId,
  title,
  color,
  isActive,
  isBusy,
  panelId,
  onActivate,
  onClose,
  onUpdate,
}: TabProps) {
  const [showDropdown, setShowDropdown] = useState(false);
  const [dropdownPos, setDropdownPos] = useState({ top: 0, left: 0 });
  const editBtnRef = useRef<HTMLButtonElement>(null);

  const handleCloseClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onClose(sessionId);
    },
    [sessionId, onClose],
  );

  const openDropdown = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    if (editBtnRef.current) {
      const rect = editBtnRef.current.getBoundingClientRect();
      setDropdownPos({ top: rect.bottom + 4, left: rect.left });
    }
    setShowDropdown(true);
  }, []);

  return (
    <>
      <div
        className={`group relative flex items-center gap-1 h-7 rounded-md text-xs cursor-pointer select-none transition-all shrink-0 max-w-48 border ${
          isActive
            ? 'border-border-1 bg-surface-2 text-text-0 shadow-sm'
            : 'border-transparent text-text-3 hover:border-border-0 hover:bg-surface-2/50 hover:text-text-1'
        }`}
        onClick={() => onActivate(panelId, sessionId)}
        style={color ? {
          borderColor: isActive ? color : undefined,
          background: isActive
            ? `linear-gradient(135deg, ${color}15 0%, transparent 60%)`
            : undefined,
        } : undefined}
      >
        {/* Color accent bar */}
        {color && (
          <div
            className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-3.5 rounded-full"
            style={{ backgroundColor: color }}
          />
        )}

        {/* Busy indicator */}
        {isBusy && (
          <Loader2 className={`w-3 h-3 animate-spin shrink-0 ${color ? 'ml-2.5' : 'ml-2'}`} style={color ? { color } : { color: 'var(--op-accent-primary)' }} />
        )}

        <span className={`truncate ${isBusy ? '' : color ? 'pl-2.5' : 'pl-2'}`}>{title}</span>

        {/* Edit button */}
        <button
          ref={editBtnRef}
          onClick={openDropdown}
          className="opacity-0 group-hover:opacity-100 flex items-center justify-center w-4 h-4 rounded hover:bg-surface-3 transition-all shrink-0 cursor-pointer"
          title="Edit terminal"
        >
          <Pencil className="w-2.5 h-2.5" />
        </button>

        <button
          onClick={handleCloseClick}
          className="flex items-center justify-center w-4 h-4 rounded opacity-0 group-hover:opacity-100 hover:bg-surface-3 transition-all shrink-0 cursor-pointer mr-1"
          title="Close terminal"
        >
          <X className="w-3 h-3" />
        </button>
      </div>

      {showDropdown && (
        <TabEditDropdown
          pos={dropdownPos}
          title={title}
          color={color}
          onRename={(newTitle) => onUpdate(sessionId, { title: newTitle })}
          onColorChange={(newColor) => onUpdate(sessionId, { color: newColor })}
          onClose={() => setShowDropdown(false)}
        />
      )}
    </>
  );
}

// ── Edit dropdown for terminal tabs ──

function TabEditDropdown({
  pos,
  title,
  color,
  onRename,
  onColorChange,
  onClose,
}: {
  pos: { top: number; left: number };
  title: string;
  color: string;
  onRename: (name: string) => void;
  onColorChange: (color: string) => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [editValue, setEditValue] = useState(title);

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
    if (trimmed && trimmed !== title) onRename(trimmed);
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
        className="bg-surface-1 border border-border-0 rounded-md text-xs text-text-0 px-2 py-1.5 outline-none focus:border-accent-primary caret-accent-primary"
        placeholder="Terminal name"
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
