import { useState, useRef, useEffect } from 'react';
import { FolderOpen, Plus, X } from 'lucide-react';

interface FolderAssignProps {
  value: string;
  folders: string[];
  onChange: (folder: string) => void;
}

export function FolderAssign({ value, folders, onChange }: FolderAssignProps) {
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState('');
  const ref = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const openDropdown = () => {
    setFilter('');
    setOpen(true);
    setTimeout(() => inputRef.current?.focus(), 0);
  };

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const filtered = folders.filter(
    (f) => f.toLowerCase().includes(filter.toLowerCase()) && f !== value,
  );
  const showCreate = filter.trim() && !folders.includes(filter.trim()) && filter.trim() !== value;

  const select = (folder: string) => {
    onChange(folder);
    setOpen(false);
  };

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => open ? setOpen(false) : openDropdown()}
        className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-sm bg-surface-2 border border-border-1 hover:border-border-0 transition-colors cursor-pointer w-full text-left"
      >
        <FolderOpen className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
        {value ? (
          <span className="text-text-1 truncate flex-1">{value}</span>
        ) : (
          <span className="text-text-3 italic flex-1">Unfiled</span>
        )}
        {value && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              onChange('');
              setOpen(false);
            }}
            className="p-0.5 rounded text-text-3 hover:text-text-1 hover:bg-surface-3 transition-colors cursor-pointer"
            aria-label="Clear folder"
          >
            <X className="w-3 h-3" />
          </button>
        )}
      </button>

      {open && (
        <div className="absolute z-50 top-full left-0 mt-1 w-full min-w-[200px] rounded-lg bg-surface-1 border border-border-1 shadow-lg overflow-hidden">
          <div className="p-2">
            <input
              ref={inputRef}
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && showCreate) select(filter.trim());
                if (e.key === 'Escape') setOpen(false);
              }}
              placeholder="Search or create..."
              className="w-full px-2.5 py-1.5 rounded-md text-sm bg-surface-2 border border-border-1 text-text-0 placeholder:text-text-3/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
            />
          </div>
          <div className="max-h-48 overflow-y-auto">
            {value && (
              <button
                onClick={() => select('')}
                className="w-full flex items-center gap-2 px-3 py-2 text-sm text-text-3 hover:bg-surface-2 transition-colors cursor-pointer"
              >
                <X className="w-3.5 h-3.5" />
                Unfiled
              </button>
            )}
            {filtered.map((f) => (
              <button
                key={f}
                onClick={() => select(f)}
                className="w-full flex items-center gap-2 px-3 py-2 text-sm text-text-1 hover:bg-surface-2 transition-colors cursor-pointer text-left"
              >
                <FolderOpen className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
                {f}
              </button>
            ))}
            {showCreate && (
              <button
                onClick={() => select(filter.trim())}
                className="w-full flex items-center gap-2 px-3 py-2 text-sm text-accent-primary hover:bg-surface-2 transition-colors cursor-pointer"
              >
                <Plus className="w-3.5 h-3.5" />
                Create &ldquo;{filter.trim()}&rdquo;
              </button>
            )}
            {!showCreate && filtered.length === 0 && !value && (
              <p className="px-3 py-2 text-xs text-text-3">Type to create a folder</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
