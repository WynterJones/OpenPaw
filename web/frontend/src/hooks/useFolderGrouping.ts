import { useMemo, useState } from 'react';

interface FolderGroupingResult<T> {
  folders: string[];
  folderCounts: Map<string, number>;
  unfiledCount: number;
  totalCount: number;
  selectedFolder: string | null;
  setSelectedFolder: (folder: string | null) => void;
  grouped: Map<string, T[]>;
  filtered: T[];
}

export function useFolderGrouping<T>(
  items: T[],
  getFolder: (item: T) => string,
): FolderGroupingResult<T> {
  const [selectedFolder, setSelectedFolder] = useState<string | null>(null);

  const { folders, folderCounts, unfiledCount, grouped } = useMemo(() => {
    const counts = new Map<string, number>();
    const groups = new Map<string, T[]>();
    let unfiled = 0;

    for (const item of items) {
      const folder = getFolder(item);
      if (!folder) {
        unfiled++;
        const arr = groups.get('') || [];
        arr.push(item);
        groups.set('', arr);
      } else {
        counts.set(folder, (counts.get(folder) || 0) + 1);
        const arr = groups.get(folder) || [];
        arr.push(item);
        groups.set(folder, arr);
      }
    }

    const sortedFolders = [...counts.keys()].sort((a, b) => a.localeCompare(b));

    return {
      folders: sortedFolders,
      folderCounts: counts,
      unfiledCount: unfiled,
      grouped: groups,
    };
  }, [items, getFolder]);

  const filtered = useMemo(() => {
    if (selectedFolder === null) return items;
    if (selectedFolder === '') return grouped.get('') || [];
    return grouped.get(selectedFolder) || [];
  }, [items, selectedFolder, grouped]);

  return {
    folders,
    folderCounts,
    unfiledCount,
    totalCount: items.length,
    selectedFolder,
    setSelectedFolder,
    grouped,
    filtered,
  };
}
