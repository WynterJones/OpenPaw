import { useState, type ReactNode } from 'react';
import { ChevronRight, ChevronDown } from 'lucide-react';

interface FolderSectionProps {
  name: string;
  count: number;
  children: ReactNode;
  defaultOpen?: boolean;
}

export function FolderSection({ name, count, children, defaultOpen = true }: FolderSectionProps) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div>
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 w-full py-2 px-1 cursor-pointer group"
      >
        {open ? (
          <ChevronDown className="w-3.5 h-3.5 text-text-3" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5 text-text-3" />
        )}
        <span className="text-[11px] font-semibold uppercase tracking-wider text-text-3 group-hover:text-text-2 transition-colors">
          {name || 'Unfiled'}
        </span>
        <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-surface-2 text-text-3">
          {count}
        </span>
      </button>
      {open && <div className="mt-1">{children}</div>}
    </div>
  );
}
