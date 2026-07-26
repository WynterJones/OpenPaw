import { useState, useEffect } from 'react';

export type ViewMode = 'grid' | 'list';

// Persists a page's grid/list choice across navigation (the pages unmount on
// route change). Keyed per page so each list remembers its own preference.
//
// Lives apart from ViewToggle.tsx because Fast Refresh only re-renders a module
// that exports components and nothing else — a hook alongside the component
// would force a full reload on every edit to either.
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
