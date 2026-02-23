import { useState, useRef, useEffect } from 'react';
import { FolderOpen, Plus } from 'lucide-react';

interface FolderFilterProps {
  folders: string[];
  folderCounts: Map<string, number>;
  unfiledCount: number;
  totalCount: number;
  selectedFolder: string | null;
  onSelect: (folder: string | null) => void;
  onAddFolder?: (name: string) => void;
}

export function FolderFilter({
  folders,
  folderCounts,
  unfiledCount,
  totalCount,
  selectedFolder,
  onSelect,
  onAddFolder,
}: FolderFilterProps) {
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (adding) inputRef.current?.focus();
  }, [adding]);

  const handleSubmit = () => {
    const trimmed = newName.trim();
    if (trimmed && !folders.includes(trimmed)) {
      onAddFolder?.(trimmed);
    }
    setNewName('');
    setAdding(false);
  };

  if (folders.length === 0 && unfiledCount === totalCount && !onAddFolder) return null;

  const pill = (
    label: string,
    count: number,
    value: string | null,
    key: string,
  ) => {
    const active = selectedFolder === value;
    return (
      <button
        key={key}
        onClick={() => onSelect(value)}
        className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-medium whitespace-nowrap transition-colors cursor-pointer ${
          active
            ? 'bg-accent-primary/15 text-accent-primary border border-accent-primary/30'
            : 'bg-surface-2 text-text-2 border border-border-1 hover:border-border-0 hover:text-text-1'
        }`}
      >
        {label}
        <span
          className={`text-[10px] px-1.5 py-0.5 rounded-full ${
            active ? 'bg-accent-primary/20 text-accent-primary' : 'bg-surface-3 text-text-3'
          }`}
        >
          {count}
        </span>
      </button>
    );
  };

  return (
    <div className="flex items-center gap-2 overflow-x-auto pb-1 mb-4 scrollbar-none">
      <FolderOpen className="w-4 h-4 text-text-3 flex-shrink-0" />
      {pill('All', totalCount, null, '_all')}
      {folders.map((f) => pill(f, folderCounts.get(f) || 0, f, f))}
      {unfiledCount > 0 && pill('Unfiled', unfiledCount, '', '_unfiled')}
      {onAddFolder && (
        adding ? (
          <input
            ref={inputRef}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleSubmit();
              if (e.key === 'Escape') { setNewName(''); setAdding(false); }
            }}
            onBlur={handleSubmit}
            placeholder="Folder name..."
            className="px-2.5 py-1 rounded-full text-xs bg-surface-2 border border-accent-primary/30 text-text-0 placeholder:text-text-3/50 focus:outline-none focus:ring-1 focus:ring-accent-primary w-32"
          />
        ) : (
          <button
            onClick={() => setAdding(true)}
            className="inline-flex items-center gap-1 px-3 py-1.5 rounded-full text-xs font-medium whitespace-nowrap transition-colors cursor-pointer bg-surface-2 text-text-3 border border-dashed border-border-1 hover:border-accent-primary/30 hover:text-accent-primary"
          >
            <Plus className="w-3 h-3" />
            Add Folder
          </button>
        )
      )}
    </div>
  );
}
