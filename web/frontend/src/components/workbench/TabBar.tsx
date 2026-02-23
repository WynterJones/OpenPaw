import { useState, useRef, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { X, Plus, Columns2, Rows2 } from 'lucide-react';
import { useWorkbench, type PanelNode } from './WorkbenchProvider';

const TAB_COLORS = [
  '', // none (default)
  '#ef4444', // red
  '#f97316', // orange
  '#eab308', // yellow
  '#22c55e', // green
  '#3b82f6', // blue
  '#8b5cf6', // violet
  '#ec4899', // pink
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
  } = useWorkbench();

  const tabs = node.tabs || [];
  const activeTab = node.activeTab;

  return (
    <div className="flex items-center bg-surface-1 border-b border-border-0 px-1.5 gap-0.5 h-9 overflow-x-auto shrink-0">
      {tabs.map((sessionId) => {
        const session = sessions.find((s) => s.id === sessionId);
        const title = session?.title || 'Terminal';
        const color = session?.color || '';
        const isActive = sessionId === activeTab;

        return (
          <Tab
            key={sessionId}
            sessionId={sessionId}
            title={title}
            color={color}
            isActive={isActive}
            isGloballyActive={sessionId === activeSessionId}
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
  panelId,
  onActivate,
  onClose,
  onUpdate,
}: TabProps) {
  const [editing, setEditing] = useState(false);
  const [editValue, setEditValue] = useState(title);
  const [showColorPicker, setShowColorPicker] = useState(false);
  const [pickerPos, setPickerPos] = useState({ top: 0, left: 0 });
  const inputRef = useRef<HTMLInputElement>(null);
  const dotRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (editing && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [editing]);

  const handleDoubleClick = useCallback(() => {
    setEditValue(title);
    setEditing(true);
  }, [title]);

  const commitRename = useCallback(() => {
    const trimmed = editValue.trim();
    setEditing(false);
    if (trimmed && trimmed !== title) {
      onUpdate(sessionId, { title: trimmed });
    }
  }, [editValue, title, sessionId, onUpdate]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') {
        commitRename();
      } else if (e.key === 'Escape') {
        setEditing(false);
      }
    },
    [commitRename],
  );

  const handleCloseClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onClose(sessionId);
    },
    [sessionId, onClose],
  );

  const handleColorClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      if (dotRef.current) {
        const rect = dotRef.current.getBoundingClientRect();
        setPickerPos({ top: rect.bottom + 4, left: rect.left });
      }
      setShowColorPicker((v) => !v);
    },
    [],
  );

  const handleColorSelect = useCallback(
    (c: string) => {
      setShowColorPicker(false);
      onUpdate(sessionId, { color: c });
    },
    [sessionId, onUpdate],
  );

  return (
    <>
      <div
        className={`group relative flex items-center gap-1.5 px-2.5 h-7 rounded-md text-xs cursor-pointer select-none transition-all shrink-0 max-w-48 ${
          isActive
            ? 'bg-surface-2 text-text-0 shadow-[inset_0_1px_0_0_rgba(255,255,255,0.04)]'
            : 'text-text-3 hover:bg-surface-2/50 hover:text-text-1'
        }`}
        onClick={() => onActivate(panelId, sessionId)}
        onDoubleClick={handleDoubleClick}
        style={color ? { borderBottom: `2px solid ${color}` } : undefined}
      >
        {/* Color dot */}
        <button
          ref={dotRef}
          onClick={handleColorClick}
          className="flex-none w-2.5 h-2.5 rounded-full border border-border-1 hover:scale-125 transition-transform cursor-pointer shrink-0"
          style={{ backgroundColor: color || 'var(--op-surface-3)' }}
          title="Set tab color"
        />

        {editing ? (
          <input
            ref={inputRef}
            value={editValue}
            onChange={(e) => setEditValue(e.target.value)}
            onBlur={commitRename}
            onKeyDown={handleKeyDown}
            className="bg-transparent border-none outline-none text-xs text-text-0 w-full min-w-12 p-0 caret-accent-primary"
            onClick={(e) => e.stopPropagation()}
          />
        ) : (
          <span className="truncate">{title}</span>
        )}

        <button
          onClick={handleCloseClick}
          className="flex items-center justify-center w-4 h-4 rounded opacity-0 group-hover:opacity-100 hover:bg-surface-3 transition-all shrink-0 cursor-pointer"
          title="Close terminal"
        >
          <X className="w-3 h-3" />
        </button>
      </div>

      {/* Color picker portal - renders above all content */}
      {showColorPicker && <ColorPickerPortal
        pos={pickerPos}
        currentColor={color}
        onSelect={handleColorSelect}
        onClose={() => setShowColorPicker(false)}
      />}
    </>
  );
}

// ── Color picker rendered in a portal to avoid scroll issues ──

function ColorPickerPortal({
  pos,
  currentColor,
  onSelect,
  onClose,
}: {
  pos: { top: number; left: number };
  currentColor: string;
  onSelect: (color: string) => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [onClose]);

  return createPortal(
    <div
      ref={ref}
      className="fixed z-[9999] bg-surface-2 border border-border-1 rounded-lg p-2 shadow-xl flex gap-1.5"
      style={{ top: pos.top, left: pos.left }}
    >
      {TAB_COLORS.map((c) => (
        <button
          key={c || 'none'}
          onClick={() => onSelect(c)}
          className="w-5 h-5 rounded-full border-2 hover:scale-125 transition-transform cursor-pointer"
          style={{
            backgroundColor: c || 'var(--op-surface-3)',
            borderColor: currentColor === c ? 'var(--op-text-0)' : 'var(--op-border-1)',
          }}
          title={c || 'None'}
        />
      ))}
    </div>,
    document.body,
  );
}
