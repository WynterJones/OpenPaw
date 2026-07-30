import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import {
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ArrowDown,
  ArrowUp,
  ChevronsUpDown,
  Database as DatabaseIcon,
  Download,
  ExternalLink,
  Hash,
  Link as LinkIcon,
  Mail,
  Plus,
  Search,
  Table2,
  Text,
  ToggleLeft,
  CalendarDays,
  List,
  Pencil,
  Trash2,
  Type,
  Upload,
  ZoomIn,
  ZoomOut,
} from 'lucide-react';
import { Header } from '../components/Header';
import { Button } from '../components/Button';
import { Modal } from '../components/Modal';
import { EmptyState } from '../components/EmptyState';
import { useToast } from '../components/Toast';
import { api, getCSRFToken, type UserDatabase, type UserDatabaseColumn, type UserDatabaseColumnType, type UserDatabaseRow, type UserDatabaseRowPage, type UserDatabaseTable, type WSMessage } from '../lib/api';
import { downloadFile } from '../lib/download';
import { handleExternalLinkClick } from '../lib/openExternal';
import { useWebSocket } from '../lib/useWebSocket';

const LAST_DATABASE_KEY = 'openpaw-last-database';
const PAGE_SIZE_KEY = 'openpaw-database-page-size';
const TEXT_SIZE_KEY = 'openpaw-database-text-size';
const DEFAULT_PAGE_SIZE = 50;
const PAGE_SIZE_OPTIONS = [10, 50, 100];
const MIN_PAGE_SIZE = 1;
const MAX_PAGE_SIZE = 500;
const DEFAULT_TEXT_SIZE = 14;
const MIN_TEXT_SIZE = 11;
const MAX_TEXT_SIZE = 18;
const DEFAULT_COLUMN_WIDTH = 240;
const MIN_COLUMN_WIDTH = 140;
const MAX_COLUMN_WIDTH = 640;

type TableSort = {
  columnId: string;
  direction: 'asc' | 'desc';
};

const COLUMN_TYPES: { value: UserDatabaseColumnType; label: string; icon: typeof Type }[] = [
  { value: 'text', label: 'Text', icon: Type },
  { value: 'long_text', label: 'Long text', icon: Text },
  { value: 'number', label: 'Number', icon: Hash },
  { value: 'checkbox', label: 'Checkbox', icon: ToggleLeft },
  { value: 'date', label: 'Date', icon: CalendarDays },
  { value: 'url', label: 'URL', icon: LinkIcon },
  { value: 'email', label: 'Email', icon: Mail },
  { value: 'select', label: 'Single select', icon: List },
];

function ColumnTypeIcon({ type }: { type: UserDatabaseColumnType }) {
  const Icon = COLUMN_TYPES.find(item => item.value === type)?.icon || Type;
  return <Icon className="w-3.5 h-3.5 text-text-3 flex-shrink-0" aria-hidden="true" />;
}

function normalizeCellValue(column: UserDatabaseColumn, raw: string): unknown {
  if (column.type === 'number') {
    if (raw.trim() === '') return null;
    const parsed = Number(raw);
    return Number.isFinite(parsed) ? parsed : raw;
  }
  return raw.trim() === '' ? null : raw;
}

function cellLink(column: UserDatabaseColumn, value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  if (column.type === 'email') return `mailto:${trimmed}`;
  if (column.type !== 'url') return null;
  if (/^https?:\/\//i.test(trimmed)) return trimmed;
  if (/^(www\.)?[a-z0-9][a-z0-9.-]+\.[a-z]{2,}(?:[/?#].*)?$/i.test(trimmed)) {
    return `https://${trimmed}`;
  }
  return null;
}

function CellEditor({
  column,
  value,
  onSave,
}: {
  column: UserDatabaseColumn;
  value: unknown;
  onSave: (value: unknown) => void;
}) {
  const shown = value === null || value === undefined ? '' : String(value);
  const [draft, setDraft] = useState(shown);
  const [editingLink, setEditingLink] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewPosition, setPreviewPosition] = useState<{
    left: number;
    width: number;
    maxHeight: number;
    top?: number;
    bottom?: number;
  } | null>(null);
  const previewAnchorRef = useRef<HTMLDivElement>(null);
  const previewOpenTimer = useRef<number | null>(null);
  const previewCloseTimer = useRef<number | null>(null);
  const previewId = useId();
  const showPreview = shown.length > 48 || shown.includes('\n');

  const positionPreview = useCallback(() => {
    const anchor = previewAnchorRef.current;
    if (!anchor) return;

    const rect = anchor.getBoundingClientRect();
    const gutter = 12;
    const gap = 8;
    const width = Math.min(
      520,
      Math.max(240, rect.width),
      Math.max(240, window.innerWidth - gutter * 2),
    );
    const left = Math.min(
      Math.max(gutter, rect.left),
      Math.max(gutter, window.innerWidth - width - gutter),
    );
    const spaceBelow = window.innerHeight - rect.bottom - gutter - gap;
    const spaceAbove = rect.top - gutter - gap;
    const placeBelow = spaceBelow >= 160 || spaceBelow >= spaceAbove;
    const availableHeight = Math.max(88, placeBelow ? spaceBelow : spaceAbove);

    setPreviewPosition({
      left,
      width,
      maxHeight: Math.min(288, availableHeight),
      ...(placeBelow
        ? { top: rect.bottom + gap }
        : { bottom: window.innerHeight - rect.top + gap }),
    });
  }, []);

  const cancelPreviewClose = () => {
    if (previewCloseTimer.current !== null) {
      window.clearTimeout(previewCloseTimer.current);
      previewCloseTimer.current = null;
    }
  };

  const openPreview = (delay = 180) => {
    if (!showPreview) return;
    cancelPreviewClose();
    if (previewOpenTimer.current !== null) window.clearTimeout(previewOpenTimer.current);
    previewOpenTimer.current = window.setTimeout(() => {
      positionPreview();
      setPreviewOpen(true);
      previewOpenTimer.current = null;
    }, delay);
  };

  const closePreview = () => {
    if (previewOpenTimer.current !== null) {
      window.clearTimeout(previewOpenTimer.current);
      previewOpenTimer.current = null;
    }
    cancelPreviewClose();
    previewCloseTimer.current = window.setTimeout(() => {
      setPreviewOpen(false);
      previewCloseTimer.current = null;
    }, 120);
  };

  useEffect(() => {
    if (!previewOpen) return;
    const reposition = () => positionPreview();
    window.addEventListener('resize', reposition);
    document.addEventListener('scroll', reposition, true);
    return () => {
      window.removeEventListener('resize', reposition);
      document.removeEventListener('scroll', reposition, true);
    };
  }, [positionPreview, previewOpen]);

  useEffect(() => () => {
    if (previewOpenTimer.current !== null) window.clearTimeout(previewOpenTimer.current);
    if (previewCloseTimer.current !== null) window.clearTimeout(previewCloseTimer.current);
  }, []);

  if (column.type === 'checkbox') {
    return (
      <label className="flex items-center justify-center w-full h-full min-h-9 cursor-pointer">
        <input
          type="checkbox"
          checked={value === true}
          onChange={event => onSave(event.target.checked)}
          className="rounded border-border-1 bg-surface-2 text-accent-primary focus:ring-accent-primary/40 cursor-pointer"
          aria-label={column.name}
        />
      </label>
    );
  }

  if (column.type === 'select') {
    const choices = Array.isArray(column.options?.choices) ? column.options.choices : [];
    return (
      <select
        value={shown}
        onChange={event => onSave(event.target.value || null)}
        className="w-full h-9 border-0 px-2 bg-transparent text-[inherit] text-text-1 outline-none ring-0 focus:border-0 focus:ring-0 cursor-pointer"
        aria-label={column.name}
      >
        <option value="">—</option>
        {choices.map(choice => <option key={choice} value={choice}>{choice}</option>)}
      </select>
    );
  }

  const commit = () => {
    const next = normalizeCellValue(column, draft);
    if (next !== value && String(next ?? '') !== shown) onSave(next);
    setEditingLink(false);
  };

  const href = cellLink(column, shown);
  const preview = showPreview && previewOpen && previewPosition && createPortal(
    <div
      id={previewId}
      role="tooltip"
      style={previewPosition}
      className="fixed z-[100] overflow-y-auto overscroll-contain rounded-xl border border-border-1/70 bg-surface-2/95 p-3 text-text-1 shadow-[0_16px_48px_rgba(0,0,0,0.45)] backdrop-blur-md"
      onMouseEnter={cancelPreviewClose}
      onMouseLeave={closePreview}
    >
      <div className="sticky -top-3 z-[1] -mx-3 -mt-3 mb-2 flex items-center gap-2 border-b border-border-0 bg-surface-2/95 px-3 py-2 backdrop-blur-md">
        <ColumnTypeIcon type={column.type} />
        <span className="min-w-0 truncate text-xs font-medium text-text-2">{column.name}</span>
        <span className="ml-auto whitespace-nowrap rounded-md bg-surface-3 px-1.5 py-0.5 text-[10px] font-medium text-text-3">
          Full value
        </span>
      </div>
      <div className="whitespace-pre-wrap break-words text-[13px] leading-5">{shown}</div>
    </div>,
    document.body,
  );

  if (href && !editingLink) {
    const external = column.type === 'url';
    return (
      <div
        ref={previewAnchorRef}
        className="group/cell relative flex items-center gap-1 min-h-9 px-2.5"
        onMouseEnter={() => openPreview()}
        onMouseLeave={closePreview}
        onFocusCapture={() => openPreview(0)}
        onBlurCapture={closePreview}
      >
        <a
          href={href}
          target={external ? '_blank' : undefined}
          rel={external ? 'noopener noreferrer' : undefined}
          onClick={event => {
            if (external) handleExternalLinkClick(event, href);
          }}
          className="min-w-0 flex-1 inline-flex items-center gap-1.5 text-accent-text hover:underline focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent-primary rounded"
          aria-describedby={previewOpen ? previewId : undefined}
        >
          <span className="truncate">{shown}</span>
          {external && <ExternalLink className="w-3 h-3 flex-shrink-0" aria-hidden="true" />}
        </a>
        <button
          type="button"
          onClick={() => setEditingLink(true)}
          className="p-1 rounded text-text-3 opacity-0 group-hover/cell:opacity-100 group-focus-within/cell:opacity-100 hover:text-text-1 hover:bg-surface-2 transition-opacity cursor-pointer"
          title={`Edit ${column.name}`}
          aria-label={`Edit ${column.name}`}
        >
          <Pencil className="w-3 h-3" aria-hidden="true" />
        </button>
        {preview}
      </div>
    );
  }

  return (
    <div
      ref={previewAnchorRef}
      className="group/cell relative min-h-9"
      onMouseEnter={() => openPreview()}
      onMouseLeave={closePreview}
      onFocusCapture={() => openPreview(0)}
      onBlurCapture={closePreview}
    >
      <input
        autoFocus={editingLink}
        type={column.type === 'date' ? 'date' : column.type === 'number' ? 'number' : column.type === 'email' ? 'email' : column.type === 'url' ? 'url' : 'text'}
        value={draft}
        onChange={event => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={event => {
          if (event.key === 'Enter') event.currentTarget.blur();
          if (event.key === 'Escape') {
            setDraft(shown);
            setEditingLink(false);
            event.currentTarget.blur();
          }
        }}
        className="w-full h-9 border-0 px-2.5 bg-transparent text-[inherit] text-text-1 outline-none ring-0 focus:border-0 focus:bg-accent-primary/5 focus:ring-0"
        aria-label={column.name}
        aria-describedby={previewOpen ? previewId : undefined}
      />
      {preview}
    </div>
  );
}

export function Databases() {
  const { toast } = useToast();
  const [databases, setDatabases] = useState<UserDatabase[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [database, setDatabase] = useState<UserDatabase | null>(null);
  const [tableId, setTableId] = useState<string | null>(null);
  const [rows, setRows] = useState<UserDatabaseRow[]>([]);
  const [rowTotal, setRowTotal] = useState(0);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [rowsLoading, setRowsLoading] = useState(false);
  const [switcherOpen, setSwitcherOpen] = useState(false);
  const switcherRef = useRef<HTMLDivElement>(null);
  const importInputRef = useRef<HTMLInputElement>(null);
  const [importing, setImporting] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(() => {
    const stored = Number(localStorage.getItem(PAGE_SIZE_KEY));
    return Number.isInteger(stored) && stored >= MIN_PAGE_SIZE && stored <= MAX_PAGE_SIZE
      ? stored
      : DEFAULT_PAGE_SIZE;
  });
  const [pageSizeModal, setPageSizeModal] = useState(false);
  const [customPageSize, setCustomPageSize] = useState(String(pageSize));
  const [textSize, setTextSize] = useState(() => {
    const stored = Number(localStorage.getItem(TEXT_SIZE_KEY));
    return Number.isInteger(stored) && stored >= MIN_TEXT_SIZE && stored <= MAX_TEXT_SIZE
      ? stored
      : DEFAULT_TEXT_SIZE;
  });

  const [databaseModal, setDatabaseModal] = useState<'create' | 'edit' | null>(null);
  const [databaseName, setDatabaseName] = useState('');
  const [databaseDescription, setDatabaseDescription] = useState('');
  const [tableModal, setTableModal] = useState<'create' | 'edit' | null>(null);
  const [tableName, setTableName] = useState('');
  const [columnModal, setColumnModal] = useState<'create' | 'edit' | null>(null);
  const [editingColumn, setEditingColumn] = useState<UserDatabaseColumn | null>(null);
  const [columnName, setColumnName] = useState('');
  const [columnType, setColumnType] = useState<UserDatabaseColumnType>('text');
  const [columnChoices, setColumnChoices] = useState('');
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<'database' | 'table' | null>(null);
  const [columnWidths, setColumnWidths] = useState<Record<string, number>>({});
  const [tableSorts, setTableSorts] = useState<Record<string, TableSort>>({});

  const selectedTable = useMemo(
    () => database?.tables?.find(table => table.id === tableId) ?? null,
    [database, tableId],
  );
  const activeSort = tableId ? tableSorts[tableId] ?? null : null;
  const totalPages = Math.max(1, Math.ceil(rowTotal / pageSize));
  const firstVisibleRow = rowTotal === 0 ? 0 : (page - 1) * pageSize + 1;
  const lastVisibleRow = Math.min(rowTotal, (page - 1) * pageSize + rows.length);

  useEffect(() => {
    if (!selectedTable) {
      setColumnWidths({});
      return;
    }
    const key = `openpaw-database-column-widths:${selectedTable.id}`;
    try {
      const stored = JSON.parse(localStorage.getItem(key) || '{}') as Record<string, number>;
      setColumnWidths(Object.fromEntries(
        selectedTable.columns.map(column => [
          column.id,
          Math.min(MAX_COLUMN_WIDTH, Math.max(MIN_COLUMN_WIDTH, Number(stored[column.id]) || DEFAULT_COLUMN_WIDTH)),
        ]),
      ));
    } catch {
      setColumnWidths(Object.fromEntries(selectedTable.columns.map(column => [column.id, DEFAULT_COLUMN_WIDTH])));
    }
  }, [selectedTable]);

  useEffect(() => {
    if (!selectedTable || Object.keys(columnWidths).length === 0) return;
    localStorage.setItem(`openpaw-database-column-widths:${selectedTable.id}`, JSON.stringify(columnWidths));
  }, [columnWidths, selectedTable]);

  useEffect(() => {
    localStorage.setItem(PAGE_SIZE_KEY, String(pageSize));
  }, [pageSize]);

  useEffect(() => {
    localStorage.setItem(TEXT_SIZE_KEY, String(textSize));
  }, [textSize]);

  const startColumnResize = (event: React.PointerEvent, columnId: string) => {
    event.preventDefault();
    event.stopPropagation();
    const startX = event.clientX;
    const startWidth = columnWidths[columnId] || DEFAULT_COLUMN_WIDTH;
    const move = (moveEvent: PointerEvent) => {
      const next = Math.min(MAX_COLUMN_WIDTH, Math.max(MIN_COLUMN_WIDTH, startWidth + moveEvent.clientX - startX));
      setColumnWidths(current => ({ ...current, [columnId]: next }));
    };
    const stop = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', stop);
      window.removeEventListener('pointercancel', stop);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', stop, { once: true });
    window.addEventListener('pointercancel', stop, { once: true });
  };

  const loadDatabases = useCallback(async (preferredId?: string | null) => {
    const items = await api.get<UserDatabase[]>('/databases');
    const list = Array.isArray(items) ? items : [];
    setDatabases(list);
    const preferred = preferredId || selectedId || localStorage.getItem(LAST_DATABASE_KEY);
    const next = list.find(item => item.id === preferred)?.id || list[0]?.id || null;
    setSelectedId(next);
    if (next) localStorage.setItem(LAST_DATABASE_KEY, next);
    else localStorage.removeItem(LAST_DATABASE_KEY);
    return next;
  }, [selectedId]);

  const loadDatabase = useCallback(async (id: string, preferredTableId?: string | null) => {
    const item = await api.get<UserDatabase>(`/databases/${id}`);
    setDatabase(item);
    const nextTable = item.tables?.find(table => table.id === (preferredTableId || tableId))?.id
      || item.tables?.[0]?.id
      || null;
    setTableId(nextTable);
    return nextTable;
  }, [tableId]);

  const loadRows = useCallback(async (
    activeTableId: string,
    query = search,
    pageNumber = page,
    sortOverride?: TableSort | null,
  ) => {
    setRowsLoading(true);
    try {
      const offset = Math.max(0, pageNumber - 1) * pageSize;
      const params = new URLSearchParams({ limit: String(pageSize), offset: String(offset) });
      if (query.trim()) params.set('search', query.trim());
      const tableSort = sortOverride === undefined ? tableSorts[activeTableId] : sortOverride;
      if (tableSort) {
        params.set('sort_column', tableSort.columnId);
        params.set('sort_direction', tableSort.direction);
      }
      const result = await api.get<UserDatabaseRowPage>(`/databases/tables/${activeTableId}/rows?${params}`);
      setRows(Array.isArray(result.rows) ? result.rows : []);
      setRowTotal(result.total || 0);
    } finally {
      setRowsLoading(false);
    }
  }, [page, pageSize, search, tableSorts]);

  const toggleColumnSort = (columnId: string) => {
    if (!selectedTable) return;
    setPage(1);
    setTableSorts(current => {
      const existing = current[selectedTable.id];
      if (!existing || existing.columnId !== columnId) {
        return { ...current, [selectedTable.id]: { columnId, direction: 'asc' } };
      }
      if (existing.direction === 'asc') {
        return { ...current, [selectedTable.id]: { columnId, direction: 'desc' } };
      }
      const next = { ...current };
      delete next[selectedTable.id];
      return next;
    });
  };

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const next = await loadDatabases();
        if (next && !cancelled) await loadDatabase(next);
      } catch {
        if (!cancelled) setDatabases([]);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!tableId) {
      setRows([]);
      setRowTotal(0);
      return;
    }
    const timer = window.setTimeout(() => loadRows(tableId, search, page), 180);
    return () => window.clearTimeout(timer);
  }, [tableId, search, page, loadRows]);

  useEffect(() => {
    if (page > totalPages) setPage(totalPages);
  }, [page, totalPages]);

  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (switcherRef.current && !switcherRef.current.contains(event.target as Node)) setSwitcherOpen(false);
    };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, []);

  const reloadCurrent = useCallback(async (sortOverride?: TableSort | null, pageOverride = page) => {
    if (!selectedId) return;
    const activeTable = await loadDatabase(selectedId, tableId);
    await loadDatabases(selectedId);
    if (activeTable) await loadRows(activeTable, search, pageOverride, sortOverride);
  }, [selectedId, tableId, loadDatabase, loadDatabases, loadRows, page, search]);

  useWebSocket({
    onMessage: useCallback((message: WSMessage) => {
      if (message.type === 'database_updated') reloadCurrent().catch(() => {});
    }, [reloadCurrent]),
  });

  const selectDatabase = async (id: string) => {
    setSwitcherOpen(false);
    setSelectedId(id);
    setSearch('');
    setPage(1);
    localStorage.setItem(LAST_DATABASE_KEY, id);
    await loadDatabase(id, null);
  };

  const importCSV = async (file: File) => {
    setImporting(true);
    try {
      const formData = new FormData();
      formData.append('file', file);
      const headers: Record<string, string> = {};
      const csrf = getCSRFToken();
      if (csrf) headers['X-CSRF-Token'] = csrf;
      const response = await fetch('/api/v1/databases/import', {
        method: 'POST',
        headers,
        body: formData,
        credentials: 'same-origin',
      });
      if (!response.ok) {
        const body = await response.text();
        let message = `Import failed: ${response.status}`;
        try {
          const parsed = JSON.parse(body) as { error?: string; message?: string };
          message = parsed.error || parsed.message || message;
        } catch {
          if (body) message = body;
        }
        throw new Error(message);
      }
      const result = await response.json() as { database: UserDatabase; imported_rows: number };
      setSearch('');
      setPage(1);
      setSelectedId(result.database.id);
      localStorage.setItem(LAST_DATABASE_KEY, result.database.id);
      await loadDatabases(result.database.id);
      await loadDatabase(result.database.id, result.database.tables?.[0]?.id);
      toast('success', `Imported ${result.imported_rows} ${result.imported_rows === 1 ? 'row' : 'rows'} from CSV`);
    } catch (error) {
      toast('error', error instanceof Error ? error.message : 'Could not import CSV');
    } finally {
      setImporting(false);
      if (importInputRef.current) importInputRef.current.value = '';
    }
  };

  const applyPageSize = (next: number) => {
    if (!Number.isFinite(next)) return;
    const bounded = Math.min(MAX_PAGE_SIZE, Math.max(MIN_PAGE_SIZE, Math.round(next)));
    setPageSize(bounded);
    setPage(1);
    setCustomPageSize(String(bounded));
    setPageSizeModal(false);
  };

  const changeTextSize = (delta: number) => {
    setTextSize(current => Math.min(MAX_TEXT_SIZE, Math.max(MIN_TEXT_SIZE, current + delta)));
  };

  const saveDatabase = async () => {
    if (!databaseName.trim()) return;
    setSaving(true);
    try {
      if (databaseModal === 'create') {
        const created = await api.post<UserDatabase>('/databases', {
          name: databaseName.trim(),
          description: databaseDescription.trim(),
        });
        await loadDatabases(created.id);
        setSelectedId(created.id);
        setPage(1);
        await loadDatabase(created.id);
        toast('success', 'Database created');
      } else if (database) {
        await api.put(`/databases/${database.id}`, {
          name: databaseName.trim(),
          description: databaseDescription.trim(),
        });
        await reloadCurrent();
        toast('success', 'Database updated');
      }
      setDatabaseModal(null);
    } catch (error) {
      toast('error', error instanceof Error ? error.message : 'Could not save database');
    } finally {
      setSaving(false);
    }
  };

  const saveTable = async () => {
    if (saving || !tableName.trim() || !database) return;
    setSaving(true);
    try {
      if (tableModal === 'create') {
        const created = await api.post<UserDatabaseTable>(`/databases/${database.id}/tables`, { name: tableName.trim() });
        setPage(1);
        await loadDatabase(database.id, created.id);
        setTableId(created.id);
        toast('success', 'Table created');
      } else if (selectedTable) {
        const updated = await api.put<UserDatabaseTable>(`/databases/tables/${selectedTable.id}`, { name: tableName.trim() });
        setDatabase(current => current ? {
          ...current,
          tables: (current.tables || []).map(table => table.id === updated.id ? { ...table, ...updated } : table),
        } : current);
        toast('success', 'Table renamed');
      }
      setTableModal(null);
    } catch (error) {
      toast('error', error instanceof Error ? error.message : 'Could not save table');
    } finally {
      setSaving(false);
    }
  };

  const openColumnModal = (column?: UserDatabaseColumn) => {
    setEditingColumn(column || null);
    setColumnName(column?.name || '');
    setColumnType(column?.type || 'text');
    setColumnChoices(Array.isArray(column?.options?.choices) ? column.options.choices.join(', ') : '');
    setColumnModal(column ? 'edit' : 'create');
  };

  const saveColumn = async () => {
    if (!columnName.trim() || !selectedTable) return;
    setSaving(true);
    const options = columnType === 'select'
      ? { choices: columnChoices.split(',').map(choice => choice.trim()).filter(Boolean) }
      : {};
    try {
      if (columnModal === 'create') {
        await api.post(`/databases/tables/${selectedTable.id}/columns`, {
          name: columnName.trim(),
          type: columnType,
          options,
        });
        toast('success', 'Column added');
      } else if (editingColumn) {
        await api.put(`/databases/columns/${editingColumn.id}`, {
          name: columnName.trim(),
          type: columnType,
          options,
        });
        toast('success', 'Column updated');
      }
      setColumnModal(null);
      await reloadCurrent();
    } catch (error) {
      toast('error', error instanceof Error ? error.message : 'Could not save column');
    } finally {
      setSaving(false);
    }
  };

  const deleteColumn = async (column: UserDatabaseColumn) => {
    if (!window.confirm(`Delete column "${column.name}" and its values?`)) return;
    try {
      await api.delete(`/databases/columns/${column.id}`);
      const clearedActiveSort = Boolean(
        selectedTable && tableSorts[selectedTable.id]?.columnId === column.id,
      );
      if (clearedActiveSort && selectedTable) {
        setTableSorts(current => {
          const next = { ...current };
          delete next[selectedTable.id];
          return next;
        });
      }
      await reloadCurrent(clearedActiveSort ? null : undefined);
      toast('success', 'Column deleted');
    } catch (error) {
      toast('error', error instanceof Error ? error.message : 'Could not delete column');
    }
  };

  const addRow = async () => {
    if (!selectedTable) return;
    try {
      await api.post<UserDatabaseRow>(`/databases/tables/${selectedTable.id}/rows`, { values: {} });
      const targetPage = !search.trim() && !activeSort
        ? Math.max(1, Math.ceil((rowTotal + 1) / pageSize))
        : page;
      if (targetPage !== page) setPage(targetPage);
      else await loadRows(selectedTable.id, search, targetPage);
    } catch (error) {
      toast('error', error instanceof Error ? error.message : 'Could not add row');
    }
  };

  const updateCell = async (rowId: string, columnId: string, value: unknown) => {
    const previous = rows;
    setRows(current => current.map(row => row.id === rowId
      ? { ...row, values: { ...row.values, [columnId]: value } }
      : row));
    try {
      await api.put(`/databases/rows/${rowId}`, { values: { [columnId]: value } });
    } catch (error) {
      setRows(previous);
      toast('error', error instanceof Error ? error.message : 'Could not update cell');
      return;
    }
    if (selectedTable && tableSorts[selectedTable.id]?.columnId === columnId) {
      try {
        await loadRows(selectedTable.id, search, page);
      } catch {
        toast('error', 'Cell saved, but sorted rows could not be refreshed');
      }
    }
  };

  const deleteRow = async (rowId: string) => {
    try {
      await api.delete(`/databases/rows/${rowId}`);
      const nextTotal = Math.max(0, rowTotal - 1);
      const targetPage = Math.min(page, Math.max(1, Math.ceil(nextTotal / pageSize)));
      setRowTotal(nextTotal);
      if (targetPage !== page) setPage(targetPage);
      else if (selectedTable) await loadRows(selectedTable.id, search, targetPage);
    } catch (error) {
      toast('error', error instanceof Error ? error.message : 'Could not delete row');
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setSaving(true);
    try {
      if (deleteTarget === 'database' && database) {
        await api.delete(`/databases/${database.id}`);
        setDatabase(null);
        setPage(1);
        const next = await loadDatabases(null);
        if (next) await loadDatabase(next);
        toast('success', 'Database deleted');
      } else if (deleteTarget === 'table' && selectedTable && database) {
        await api.delete(`/databases/tables/${selectedTable.id}`);
        setPage(1);
        await loadDatabase(database.id, null);
        toast('success', 'Table deleted');
      }
      setDeleteTarget(null);
    } catch (error) {
      toast('error', error instanceof Error ? error.message : 'Could not delete');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Databases" />
        <div className="flex-1 flex items-center justify-center">
          <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full min-h-0">
      <Header
        title="Databases"
        hideTitleOnMobile
        actions={
          <div className="flex items-center gap-2">
            <input
              ref={importInputRef}
              type="file"
              accept=".csv,text/csv"
              className="hidden"
              onChange={event => {
                const file = event.target.files?.[0];
                if (file) importCSV(file);
              }}
            />
            <div ref={switcherRef} className="relative">
              <button
                onClick={() => setSwitcherOpen(open => !open)}
                className="flex items-center gap-2 min-w-[150px] max-w-[240px] px-3 py-1.5 rounded-lg border border-border-0 bg-surface-2 text-sm text-text-1 hover:border-border-1 transition-colors cursor-pointer"
                aria-haspopup="listbox"
                aria-expanded={switcherOpen}
              >
                <DatabaseIcon className="w-4 h-4 text-accent-primary flex-shrink-0" />
                <span className="flex-1 truncate text-left">{database?.name || 'Choose database'}</span>
                <ChevronDown className={`w-4 h-4 text-text-3 transition-transform ${switcherOpen ? 'rotate-180' : ''}`} />
              </button>
              {switcherOpen && (
                <div className="absolute z-40 top-full left-0 mt-1 w-64 rounded-xl border border-border-1 bg-surface-1 shadow-xl overflow-hidden">
                  <div className="max-h-72 overflow-y-auto py-1">
                    {databases.map(item => (
                      <button
                        key={item.id}
                        onClick={() => selectDatabase(item.id)}
                        className={`w-full flex items-center gap-2 px-3 py-2 text-sm text-left cursor-pointer ${
                          item.id === selectedId ? 'bg-accent-muted text-accent-text' : 'text-text-1 hover:bg-surface-2'
                        }`}
                      >
                        <span className="flex-1 truncate">{item.name}</span>
                        <span className="text-[10px] text-text-3">{item.row_count} rows</span>
                        {item.id === selectedId && <Check className="w-3.5 h-3.5" />}
                      </button>
                    ))}
                  </div>
                  {database && (
                    <div className="grid grid-cols-2 border-t border-border-0 sm:hidden">
                      <button
                        onClick={() => {
                          setSwitcherOpen(false);
                          setDatabaseName(database.name);
                          setDatabaseDescription(database.description);
                          setDatabaseModal('edit');
                        }}
                        className="flex items-center justify-center gap-1.5 px-3 py-2 text-xs text-text-2 hover:bg-surface-2 hover:text-text-0 cursor-pointer"
                      >
                        <Pencil className="w-3.5 h-3.5" />
                        Edit
                      </button>
                      <button
                        onClick={() => {
                          setSwitcherOpen(false);
                          setDeleteTarget('database');
                        }}
                        className="flex items-center justify-center gap-1.5 border-l border-border-0 px-3 py-2 text-xs text-text-2 hover:bg-danger/10 hover:text-danger cursor-pointer"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                        Delete
                      </button>
                    </div>
                  )}
                  <button
                    onClick={() => {
                      setSwitcherOpen(false);
                      setDatabaseName('');
                      setDatabaseDescription('');
                      setDatabaseModal('create');
                    }}
                    className="w-full flex items-center gap-2 px-3 py-2 border-t border-border-0 text-sm text-accent-text hover:bg-surface-2 cursor-pointer"
                  >
                    <Plus className="w-4 h-4" /> New database
                  </button>
                </div>
              )}
            </div>
            {database && (
              <>
                <button
                  onClick={() => {
                    setDatabaseName(database.name);
                    setDatabaseDescription(database.description);
                    setDatabaseModal('edit');
                  }}
                  className="hidden sm:inline-flex p-2 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2 cursor-pointer"
                  title="Edit database"
                  aria-label="Edit database"
                >
                  <Pencil className="w-4 h-4" />
                </button>
                <button
                  onClick={() => setDeleteTarget('database')}
                  className="hidden sm:inline-flex p-2 rounded-lg text-text-2 hover:text-danger hover:bg-danger/10 cursor-pointer"
                  title="Delete database"
                  aria-label="Delete database"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </>
            )}
            <Button
              size="sm"
              variant="secondary"
              icon={<Upload className="w-4 h-4" />}
              loading={importing}
              onClick={() => importInputRef.current?.click()}
            >
              <span className="hidden lg:inline">Import CSV</span>
            </Button>
            <Button
              size="sm"
              icon={<Plus className="w-4 h-4" />}
              onClick={() => {
                setDatabaseName('');
                setDatabaseDescription('');
                setDatabaseModal('create');
              }}
            >
              <span className="hidden sm:inline">New database</span>
            </Button>
          </div>
        }
      />

      {!database ? (
        <div className="flex-1 flex items-center justify-center p-6">
          <EmptyState
            icon={<DatabaseIcon className="w-9 h-9" />}
            title="No databases yet"
            description="Create a structured home for projects, research, favorite links, reports, or anything your agents should be able to search and update."
            action={
              <Button
                icon={<Plus className="w-4 h-4" />}
                onClick={() => {
                  setDatabaseName('');
                  setDatabaseDescription('');
                  setDatabaseModal('create');
                }}
              >
                Create database
              </Button>
            }
          />
        </div>
      ) : (
        <>
          <div className="flex-shrink-0 border-b border-border-0 bg-surface-1/70">
            <div className="flex items-end gap-1 px-3 pt-2 overflow-x-auto">
              {(database.tables || []).map(table => (
                <button
                  key={table.id}
                  type="button"
                  onClick={() => { setTableId(table.id); setSearch(''); setPage(1); }}
                  onDoubleClick={() => {
                    setTableName(table.name);
                    setTableModal('edit');
                  }}
                  className={`flex items-center gap-2 px-3 py-2 rounded-t-lg border border-b-0 text-sm whitespace-nowrap cursor-pointer transition-colors ${
                    table.id === tableId
                      ? 'bg-surface-0 border-border-1 text-text-0 font-medium'
                      : 'border-transparent text-text-2 hover:text-text-1 hover:bg-surface-2'
                  }`}
                >
                  <Table2 className="w-3.5 h-3.5" />
                  {table.name}
                  <span className="text-[10px] text-text-3">{table.row_count}</span>
                </button>
              ))}
              <button
                onClick={() => { setTableName(''); setTableModal('create'); }}
                className="p-2 mb-0.5 rounded-lg text-text-3 hover:text-accent-text hover:bg-surface-2 cursor-pointer"
                title="Add table"
                aria-label="Add table"
              >
                <Plus className="w-4 h-4" />
              </button>
            </div>
          </div>

          {selectedTable ? (
            <div className="flex flex-col flex-1 min-h-0">
              <div className="flex-shrink-0 flex flex-wrap items-center gap-2 px-3 py-2 border-b border-border-0 bg-surface-0/60">
                <div className="relative flex-1 min-w-[180px] max-w-sm">
                  <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-text-3" />
                  <input
                    value={search}
                    onChange={event => { setSearch(event.target.value); setPage(1); }}
                    placeholder={`Search ${selectedTable.name}…`}
                    className="w-full h-8 pl-8 pr-3 rounded-lg border border-border-0 bg-surface-1 text-sm text-text-1 placeholder:text-text-3 outline-none focus:border-accent-primary"
                  />
                </div>
                <span className="text-xs text-text-3">
                  {rowTotal} {rowTotal === 1 ? 'row' : 'rows'}
                </span>
                <div className="ml-auto flex flex-wrap items-center justify-end gap-1.5">
                  <div className="flex h-8 items-center rounded-lg border border-border-0 bg-surface-1" aria-label="Table text size">
                    <button
                      type="button"
                      onClick={() => changeTextSize(-1)}
                      disabled={textSize <= MIN_TEXT_SIZE}
                      className="flex h-full w-8 items-center justify-center rounded-l-lg text-text-2 hover:bg-surface-2 hover:text-text-0 disabled:cursor-not-allowed disabled:opacity-35"
                      title="Decrease table text size"
                      aria-label="Decrease table text size"
                    >
                      <ZoomOut className="h-3.5 w-3.5" aria-hidden="true" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setTextSize(DEFAULT_TEXT_SIZE)}
                      className="h-full min-w-12 border-x border-border-0 px-1.5 text-[11px] font-medium text-text-2 hover:bg-surface-2 hover:text-text-0"
                      title="Reset table text size"
                      aria-label={`Table text size ${Math.round(textSize / DEFAULT_TEXT_SIZE * 100)} percent. Reset`}
                    >
                      {Math.round(textSize / DEFAULT_TEXT_SIZE * 100)}%
                    </button>
                    <button
                      type="button"
                      onClick={() => changeTextSize(1)}
                      disabled={textSize >= MAX_TEXT_SIZE}
                      className="flex h-full w-8 items-center justify-center rounded-r-lg text-text-2 hover:bg-surface-2 hover:text-text-0 disabled:cursor-not-allowed disabled:opacity-35"
                      title="Increase table text size"
                      aria-label="Increase table text size"
                    >
                      <ZoomIn className="h-3.5 w-3.5" aria-hidden="true" />
                    </button>
                  </div>
                  <Button
                    size="sm"
                    variant="secondary"
                    icon={<Pencil className="w-3.5 h-3.5" />}
                    onClick={() => { setTableName(selectedTable.name); setTableModal('edit'); }}
                  >
                    <span className="hidden xl:inline">Rename</span>
                  </Button>
                  <Button
                    size="sm"
                    variant="secondary"
                    icon={<Download className="w-3.5 h-3.5" />}
                    onClick={() => downloadFile(
                      `/api/v1/databases/tables/${selectedTable.id}/export`,
                      `${selectedTable.name}.csv`,
                    )}
                  >
                    <span className="hidden xl:inline">Export CSV</span>
                  </Button>
                  <button
                    type="button"
                    onClick={() => setDeleteTarget('table')}
                    className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border-0 bg-surface-1 text-text-3 hover:border-danger/40 hover:bg-danger/10 hover:text-danger cursor-pointer"
                    title="Delete table"
                    aria-label="Delete table"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                  <Button size="sm" variant="secondary" icon={<Plus className="w-3.5 h-3.5" />} onClick={addRow}>
                    Add row
                  </Button>
                </div>
              </div>

              <div className="relative flex-1 min-h-0 overflow-auto bg-surface-0">
                <table
                  className="border-separate border-spacing-0 min-w-full w-max"
                  style={{ fontSize: `${textSize}px` }}
                >
                  <thead className="sticky top-0 z-20">
                    <tr>
                      <th className="sticky left-0 z-30 w-12 min-w-12 h-10 border-r border-b border-border-0/45 bg-surface-2 text-[10px] font-medium text-text-3 text-center">
                        #
                      </th>
                      {selectedTable.columns.map(column => (
                        <th
                          key={column.id}
                          aria-sort={activeSort?.columnId === column.id
                            ? activeSort.direction === 'asc' ? 'ascending' : 'descending'
                            : 'none'}
                          style={{
                            width: columnWidths[column.id] || DEFAULT_COLUMN_WIDTH,
                            minWidth: columnWidths[column.id] || DEFAULT_COLUMN_WIDTH,
                            maxWidth: columnWidths[column.id] || DEFAULT_COLUMN_WIDTH,
                          }}
                          className="group/header relative h-10 border-r border-b border-border-0/45 bg-surface-2 text-left font-medium text-text-1"
                        >
                          <div className="flex items-center gap-0.5 px-1">
                            <button
                              type="button"
                              onClick={() => toggleColumnSort(column.id)}
                              className={`flex h-8 min-w-0 flex-1 items-center gap-2 rounded-md px-1.5 text-left transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-primary/70 ${
                                activeSort?.columnId === column.id
                                  ? 'text-accent-text bg-accent-muted/60'
                                  : 'text-text-1 hover:bg-surface-3'
                              }`}
                              title={activeSort?.columnId === column.id
                                ? activeSort.direction === 'asc'
                                  ? `Sort ${column.name} descending`
                                  : `Clear ${column.name} sort`
                                : `Sort ${column.name} ascending`}
                              aria-label={activeSort?.columnId === column.id
                                ? activeSort.direction === 'asc'
                                  ? `Sort ${column.name} descending`
                                  : `Clear ${column.name} sort`
                                : `Sort ${column.name} ascending`}
                            >
                              <ColumnTypeIcon type={column.type} />
                              <span className="min-w-0 flex-1 truncate">{column.name}</span>
                              {activeSort?.columnId === column.id
                                ? activeSort.direction === 'asc'
                                  ? <ArrowUp className="h-3.5 w-3.5 flex-shrink-0" aria-hidden="true" />
                                  : <ArrowDown className="h-3.5 w-3.5 flex-shrink-0" aria-hidden="true" />
                                : <ChevronsUpDown className="h-3.5 w-3.5 flex-shrink-0 text-text-3" aria-hidden="true" />}
                            </button>
                            <span className="opacity-0 group-hover/header:opacity-100 group-focus-within/header:opacity-100 flex items-center gap-0.5">
                              <button
                                onClick={() => openColumnModal(column)}
                                className="p-1 rounded text-text-3 hover:text-text-1 hover:bg-surface-3 cursor-pointer"
                                title={`Edit ${column.name}`}
                                aria-label={`Edit ${column.name}`}
                              >
                                <Pencil className="w-3 h-3" />
                              </button>
                              <button
                                onClick={() => deleteColumn(column)}
                                className="p-1 rounded text-text-3 hover:text-danger hover:bg-danger/10 cursor-pointer"
                                title={`Delete ${column.name}`}
                                aria-label={`Delete ${column.name}`}
                              >
                                <Trash2 className="w-3 h-3" />
                              </button>
                            </span>
                          </div>
                          <button
                            type="button"
                            onPointerDown={event => startColumnResize(event, column.id)}
                            className="absolute right-0 inset-y-0 z-10 w-1.5 translate-x-1/2 cursor-col-resize touch-none hover:bg-accent-primary/50 focus-visible:bg-accent-primary/50 focus-visible:outline-none"
                            title={`Resize ${column.name}`}
                            aria-label={`Resize ${column.name} column`}
                          />
                        </th>
                      ))}
                      <th className="sticky right-0 z-30 w-12 min-w-12 h-10 border-b border-border-0/45 bg-surface-2">
                        <button
                          onClick={() => openColumnModal()}
                          className="w-full h-full flex items-center justify-center text-text-3 hover:text-accent-text hover:bg-surface-3 cursor-pointer"
                          title="Add column"
                          aria-label="Add column"
                        >
                          <Plus className="w-4 h-4" />
                        </button>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row, rowIndex) => (
                      <tr key={row.id} className="database-grid-row group/row">
                        <td className="database-grid-cell sticky left-0 z-10 h-10 text-[11px] text-text-3 text-center">
                          <span className="group-hover/row:hidden">{(page - 1) * pageSize + rowIndex + 1}</span>
                          <button
                            onClick={() => deleteRow(row.id)}
                            className="hidden group-hover/row:inline-flex p-1 rounded text-text-3 hover:text-danger hover:bg-danger/10 cursor-pointer"
                            title="Delete row"
                            aria-label={`Delete row ${(page - 1) * pageSize + rowIndex + 1}`}
                          >
                            <Trash2 className="w-3 h-3" />
                          </button>
                        </td>
                        {selectedTable.columns.map(column => (
                          <td
                            key={column.id}
                            style={{
                              width: columnWidths[column.id] || DEFAULT_COLUMN_WIDTH,
                              minWidth: columnWidths[column.id] || DEFAULT_COLUMN_WIDTH,
                              maxWidth: columnWidths[column.id] || DEFAULT_COLUMN_WIDTH,
                            }}
                            className="database-grid-cell h-10 focus-within:relative focus-within:z-[1] focus-within:ring-1 focus-within:ring-inset focus-within:ring-accent-primary/70"
                          >
                            <CellEditor
                              key={`${row.id}:${column.id}:${String(row.values[column.id] ?? '')}`}
                              column={column}
                              value={row.values[column.id]}
                              onSave={value => updateCell(row.id, column.id, value)}
                            />
                          </td>
                        ))}
                        <td className="database-grid-cell sticky right-0" />
                      </tr>
                    ))}
                    <tr>
                      <td className="database-grid-cell sticky left-0 z-10 h-10 border-r border-border-0/50 bg-surface-1 text-center">
                        <Plus className="w-3.5 h-3.5 text-text-3 mx-auto" />
                      </td>
                      <td colSpan={Math.max(1, selectedTable.columns.length + 1)} className="database-grid-cell h-10">
                        <button
                          onClick={addRow}
                          className="w-full h-full px-3 text-left text-xs text-text-3 hover:text-accent-text hover:bg-surface-1/60 cursor-pointer"
                        >
                          Add a row
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
                {rowsLoading && (
                  <div className="absolute inset-0 flex items-center justify-center bg-surface-0/40 pointer-events-none">
                    <div className="w-6 h-6 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
                  </div>
                )}
                {!rowsLoading && rows.length === 0 && search && (
                  <div className="p-8 text-center text-sm text-text-3">No rows match “{search}”.</div>
                )}
              </div>
              <div className="flex flex-shrink-0 flex-wrap items-center justify-between gap-2 border-t border-border-0 bg-surface-1/85 px-3 py-2 text-xs text-text-2 backdrop-blur-md">
                <span className="tabular-nums text-text-3">
                  {rowTotal === 0 ? 'No rows' : `${firstVisibleRow}–${lastVisibleRow} of ${rowTotal}`}
                </span>
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <label className="flex items-center gap-2">
                    <span className="hidden sm:inline text-text-3">Rows per page</span>
                    <select
                      value={PAGE_SIZE_OPTIONS.includes(pageSize) ? String(pageSize) : 'custom'}
                      onChange={event => {
                        if (event.target.value === 'custom') {
                          setCustomPageSize(String(pageSize));
                          setPageSizeModal(true);
                        } else {
                          applyPageSize(Number(event.target.value));
                        }
                      }}
                      className="h-8 rounded-lg border border-border-0 bg-surface-2 px-2 text-xs text-text-1 outline-none focus:border-accent-primary"
                      aria-label="Rows per page"
                    >
                      {PAGE_SIZE_OPTIONS.map(option => <option key={option} value={option}>{option}</option>)}
                      <option value="custom">Custom…</option>
                    </select>
                  </label>
                  <span className="min-w-20 text-center tabular-nums text-text-2">
                    Page {page} of {totalPages}
                  </span>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => setPage(current => Math.max(1, current - 1))}
                      disabled={page <= 1 || rowsLoading}
                      className="flex h-8 w-8 items-center justify-center rounded-lg border border-border-0 bg-surface-2 text-text-2 hover:border-border-1 hover:text-text-0 disabled:cursor-not-allowed disabled:opacity-35"
                      title="Previous page"
                      aria-label="Previous page"
                    >
                      <ChevronLeft className="h-4 w-4" aria-hidden="true" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setPage(current => Math.min(totalPages, current + 1))}
                      disabled={page >= totalPages || rowsLoading}
                      className="flex h-8 w-8 items-center justify-center rounded-lg border border-border-0 bg-surface-2 text-text-2 hover:border-border-1 hover:text-text-0 disabled:cursor-not-allowed disabled:opacity-35"
                      title="Next page"
                      aria-label="Next page"
                    >
                      <ChevronRight className="h-4 w-4" aria-hidden="true" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <div className="flex-1 flex items-center justify-center p-6">
              <EmptyState
                icon={<Table2 className="w-8 h-8" />}
                title="No tables"
                description="Add a table to start organizing records."
                action={<Button icon={<Plus className="w-4 h-4" />} onClick={() => { setTableName(''); setTableModal('create'); }}>Add table</Button>}
              />
            </div>
          )}
        </>
      )}

      <Modal open={databaseModal !== null} onClose={() => setDatabaseModal(null)} title={databaseModal === 'create' ? 'New Database' : 'Edit Database'} size="sm">
        <div className="space-y-4">
          <label className="block">
            <span className="block text-xs font-medium text-text-2 mb-1.5">Name</span>
            <input autoFocus value={databaseName} onChange={event => setDatabaseName(event.target.value)} className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-2 text-sm text-text-0 outline-none focus:border-accent-primary" placeholder="Projects" />
          </label>
          <label className="block">
            <span className="block text-xs font-medium text-text-2 mb-1.5">Description</span>
            <textarea value={databaseDescription} onChange={event => setDatabaseDescription(event.target.value)} rows={3} className="w-full resize-none px-3 py-2 rounded-lg border border-border-1 bg-surface-2 text-sm text-text-0 outline-none focus:border-accent-primary" placeholder="What belongs in this database?" />
          </label>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setDatabaseModal(null)}>Cancel</Button>
            <Button onClick={saveDatabase} loading={saving} disabled={!databaseName.trim()}>{databaseModal === 'create' ? 'Create' : 'Save'}</Button>
          </div>
        </div>
      </Modal>

      <Modal open={tableModal !== null} onClose={() => setTableModal(null)} title={tableModal === 'create' ? 'New Table' : 'Rename Table'} size="sm">
        <div className="space-y-4">
          <label className="block">
            <span className="block text-xs font-medium text-text-2 mb-1.5">Table name</span>
            <input
              autoFocus
              value={tableName}
              onChange={event => setTableName(event.target.value)}
              onKeyDown={event => {
                if (event.key === 'Enter' && !event.nativeEvent.isComposing && !saving) saveTable();
              }}
              className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-2 text-sm text-text-0 outline-none focus:border-accent-primary"
              placeholder="Projects"
            />
          </label>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setTableModal(null)} disabled={saving}>Cancel</Button>
            <Button onClick={saveTable} loading={saving} disabled={!tableName.trim()}>{tableModal === 'create' ? 'Create' : 'Save'}</Button>
          </div>
        </div>
      </Modal>

      <Modal open={columnModal !== null} onClose={() => setColumnModal(null)} title={columnModal === 'create' ? 'Add Column' : 'Edit Column'} size="sm">
        <div className="space-y-4">
          <label className="block">
            <span className="block text-xs font-medium text-text-2 mb-1.5">Column name</span>
            <input autoFocus value={columnName} onChange={event => setColumnName(event.target.value)} className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-2 text-sm text-text-0 outline-none focus:border-accent-primary" placeholder="Status" />
          </label>
          <label className="block">
            <span className="block text-xs font-medium text-text-2 mb-1.5">Type</span>
            <select value={columnType} onChange={event => setColumnType(event.target.value as UserDatabaseColumnType)} className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-2 text-sm text-text-0 outline-none focus:border-accent-primary cursor-pointer">
              {COLUMN_TYPES.map(type => <option key={type.value} value={type.value}>{type.label}</option>)}
            </select>
          </label>
          {columnType === 'select' && (
            <label className="block">
              <span className="block text-xs font-medium text-text-2 mb-1.5">Choices</span>
              <input value={columnChoices} onChange={event => setColumnChoices(event.target.value)} className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-2 text-sm text-text-0 outline-none focus:border-accent-primary" placeholder="Idea, Active, Done" />
              <span className="block text-[11px] text-text-3 mt-1">Separate choices with commas.</span>
            </label>
          )}
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setColumnModal(null)}>Cancel</Button>
            <Button onClick={saveColumn} loading={saving} disabled={!columnName.trim()}>{columnModal === 'create' ? 'Add column' : 'Save'}</Button>
          </div>
        </div>
      </Modal>

      <Modal open={pageSizeModal} onClose={() => setPageSizeModal(false)} title="Rows Per Page" size="sm">
        <div className="space-y-4">
          <label className="block">
            <span className="block text-xs font-medium text-text-2 mb-1.5">
              Number of rows
            </span>
            <input
              autoFocus
              type="number"
              min={MIN_PAGE_SIZE}
              max={MAX_PAGE_SIZE}
              value={customPageSize}
              onChange={event => setCustomPageSize(event.target.value)}
              onKeyDown={event => {
                if (event.key === 'Enter') applyPageSize(Number(customPageSize));
              }}
              className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-2 text-sm text-text-0 outline-none focus:border-accent-primary"
            />
            <span className="mt-1.5 block text-[11px] text-text-3">
              Choose between {MIN_PAGE_SIZE} and {MAX_PAGE_SIZE} rows.
            </span>
          </label>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setPageSizeModal(false)}>Cancel</Button>
            <Button
              onClick={() => applyPageSize(Number(customPageSize))}
              disabled={!Number.isFinite(Number(customPageSize)) || Number(customPageSize) < MIN_PAGE_SIZE || Number(customPageSize) > MAX_PAGE_SIZE}
            >
              Apply
            </Button>
          </div>
        </div>
      </Modal>

      <Modal open={deleteTarget !== null} onClose={() => setDeleteTarget(null)} title={`Delete ${deleteTarget === 'database' ? 'Database' : 'Table'}`} size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-1">
            Delete <strong className="text-text-0">{deleteTarget === 'database' ? database?.name : selectedTable?.name}</strong>?
            {deleteTarget === 'database' ? ' Every table and row in it will be removed.' : ' Every row and column in this table will be removed.'}
            {' '}This cannot be undone.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setDeleteTarget(null)}>Cancel</Button>
            <Button variant="danger" onClick={confirmDelete} loading={saving}>Delete</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
