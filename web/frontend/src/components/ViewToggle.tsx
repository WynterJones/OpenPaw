import { LayoutGrid, List } from 'lucide-react';
import { useState, useEffect } from 'react';

export type ViewMode = 'grid' | 'list';

// Persists a page's grid/list choice across navigation (the pages unmount on
// route change). Keyed per page so each list remembers its own preference.
export function usePersistentViewMode(key: string, fallback: ViewMode): [ViewMode, (v: ViewMode) => void] {
  const [view, setView] = useState<ViewMode>(() => {
    const saved = localStorage.getItem(`openpaw_view_${key}`);
    return saved === 'grid' || saved === 'list' ? saved : fallback;
  });
  useEffect(() => {
    localStorage.setItem(`openpaw_view_${key}`, view);
  }, [key, view]);
  return [view, setView];
}

interface ViewToggleProps {
  view: ViewMode;
  onViewChange: (view: ViewMode) => void;
}

export function ViewToggle({ view, onViewChange }: ViewToggleProps) {
  return (
    <div className="hidden sm:flex items-center border border-border-1 rounded-lg overflow-hidden" role="group" aria-label="View mode">
      <button
        onClick={() => onViewChange('grid')}
        aria-label="Grid view"
        aria-pressed={view === 'grid'}
        className={`px-2.5 py-2.5 transition-colors cursor-pointer ${view === 'grid' ? 'bg-surface-2 text-text-1' : 'text-text-3'}`}
      >
        <LayoutGrid className="w-4 h-4" aria-hidden="true" />
      </button>
      <button
        onClick={() => onViewChange('list')}
        aria-label="List view"
        aria-pressed={view === 'list'}
        className={`px-2.5 py-2.5 transition-colors cursor-pointer ${view === 'list' ? 'bg-surface-2 text-text-1' : 'text-text-3'}`}
      >
        <List className="w-4 h-4" aria-hidden="true" />
      </button>
    </div>
  );
}
