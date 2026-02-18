import { LayoutGrid, List } from 'lucide-react';

export type ViewMode = 'grid' | 'list';

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
        className={`p-2 transition-colors cursor-pointer ${view === 'grid' ? 'bg-surface-2 text-text-1' : 'text-text-3'}`}
      >
        <LayoutGrid className="w-4 h-4" aria-hidden="true" />
      </button>
      <button
        onClick={() => onViewChange('list')}
        aria-label="List view"
        aria-pressed={view === 'list'}
        className={`p-2 transition-colors cursor-pointer ${view === 'list' ? 'bg-surface-2 text-text-1' : 'text-text-3'}`}
      >
        <List className="w-4 h-4" aria-hidden="true" />
      </button>
    </div>
  );
}
