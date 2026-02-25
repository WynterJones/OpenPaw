import { useState, useEffect, useCallback, useRef } from 'react';
import { ListTodo, Plus, Trash2, Check, X, Eye, EyeOff, ArrowLeft, Pencil, GripVertical } from 'lucide-react';
import { todoApi } from '../lib/api-helpers';
import type { TodoList, TodoItem } from '../lib/types';
import { EmptyState } from '../components/EmptyState';
import { Header } from '../components/Header';
import { Modal } from '../components/Modal';
import { Button } from '../components/Button';
import { useToast } from '../components/Toast';

const colorPresets = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899'];

function ListFormModal({
  open,
  onClose,
  onSubmit,
  initial,
}: {
  open: boolean;
  onClose: () => void;
  onSubmit: (data: { name: string; description: string; color: string }) => void;
  initial?: { name: string; description: string; color: string };
}) {
  const [name, setName] = useState(initial?.name ?? '');
  const [description, setDescription] = useState(initial?.description ?? '');
  const [color, setColor] = useState(initial?.color ?? colorPresets[5]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    onSubmit({ name: name.trim(), description: description.trim(), color });
  };

  return (
    <Modal open={open} onClose={onClose} title={initial ? 'Edit List' : 'New List'} size="sm">
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-text-1 mb-1">Name</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full px-3 py-2 rounded-lg bg-surface-2 border border-border-0 text-text-1 text-sm focus:outline-none focus:ring-2 focus:ring-accent-primary"
            placeholder="My list..."
            autoFocus
            required
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-text-1 mb-1">Description</label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            className="w-full px-3 py-2 rounded-lg bg-surface-2 border border-border-0 text-text-1 text-sm focus:outline-none focus:ring-2 focus:ring-accent-primary resize-none"
            placeholder="Optional description..."
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-text-1 mb-2">Color</label>
          <div className="flex gap-2 flex-wrap">
            {colorPresets.map((c) => (
              <button
                key={c}
                type="button"
                onClick={() => setColor(c)}
                className={`w-8 h-8 rounded-full border-2 transition-all cursor-pointer ${
                  color === c ? 'border-text-1 scale-110' : 'border-transparent hover:scale-105'
                }`}
                style={{ backgroundColor: c }}
                aria-label={`Select color ${c}`}
              />
            ))}
          </div>
        </div>
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" size="sm" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" size="sm" type="submit" disabled={!name.trim()}>
            {initial ? 'Save' : 'Create'}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function InlineEdit({
  value,
  onSave,
  onCancel,
}: {
  value: string;
  onSave: (v: string) => void;
  onCancel: () => void;
}) {
  const [text, setText] = useState(value);
  const ref = useRef<HTMLInputElement>(null);

  useEffect(() => {
    ref.current?.focus();
    ref.current?.select();
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (text.trim()) onSave(text.trim());
    } else if (e.key === 'Escape') {
      onCancel();
    }
  };

  return (
    <input
      ref={ref}
      type="text"
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => { if (text.trim()) onSave(text.trim()); else onCancel(); }}
      onKeyDown={handleKeyDown}
      className="flex-1 px-2 py-0.5 rounded bg-surface-2 border border-border-0 text-text-1 text-sm focus:outline-none focus:ring-2 focus:ring-accent-primary"
    />
  );
}

export function TodoLists() {
  const { toast } = useToast();

  const [lists, setLists] = useState<TodoList[]>([]);
  const [selectedListId, setSelectedListId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const [items, setItems] = useState<TodoItem[]>([]);
  const [itemsLoading, setItemsLoading] = useState(false);
  const [showCompleted, setShowCompleted] = useState(true);
  const [newItemTitle, setNewItemTitle] = useState('');
  const [editingItemId, setEditingItemId] = useState<string | null>(null);

  const [listModalOpen, setListModalOpen] = useState(false);
  const [editingList, setEditingList] = useState<TodoList | null>(null);

  // Drag state
  const [dragItemId, setDragItemId] = useState<string | null>(null);
  const [dragOverItemId, setDragOverItemId] = useState<string | null>(null);

  const selectedList = lists.find((l) => l.id === selectedListId) ?? null;

  const fetchLists = useCallback(async () => {
    try {
      const data = await todoApi.list();
      setLists(data);
    } catch {
      toast('error', 'Failed to load todo lists');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => { fetchLists(); }, [fetchLists]);

  const fetchItems = useCallback(async (listId: string) => {
    setItemsLoading(true);
    try {
      const data = await todoApi.get(listId);
      setItems(data.items ?? []);
      setLists((prev) => prev.map((l) => (l.id === listId ? { ...l, ...data.list } : l)));
    } catch {
      toast('error', 'Failed to load items');
    } finally {
      setItemsLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    if (selectedListId) {
      fetchItems(selectedListId);
    } else {
      setItems([]);
    }
  }, [selectedListId, fetchItems]);

  // --- List CRUD ---
  const handleCreateList = async (data: { name: string; description: string; color: string }) => {
    try {
      const created = await todoApi.create(data);
      setLists((prev) => [...prev, { ...created, total_items: created.total_items ?? 0, completed_items: created.completed_items ?? 0 }]);
      setSelectedListId(created.id);
      setListModalOpen(false);
      setEditingList(null);
    } catch {
      toast('error', 'Failed to create list');
    }
  };

  const handleUpdateList = async (data: { name: string; description: string; color: string }) => {
    if (!editingList) return;
    try {
      const updated = await todoApi.update(editingList.id, data);
      setLists((prev) => prev.map((l) => (l.id === editingList.id ? { ...l, ...updated } : l)));
      setListModalOpen(false);
      setEditingList(null);
    } catch {
      toast('error', 'Failed to update list');
    }
  };

  const handleDeleteList = async (id: string) => {
    try {
      await todoApi.delete(id);
      setLists((prev) => prev.filter((l) => l.id !== id));
      if (selectedListId === id) {
        setSelectedListId(null);
        setItems([]);
      }
    } catch {
      toast('error', 'Failed to delete list');
    }
  };

  // --- Item CRUD ---
  const handleAddItem = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newItemTitle.trim() || !selectedListId) return;
    try {
      const item = await todoApi.createItem(selectedListId, { title: newItemTitle.trim() });
      setItems((prev) => [...prev, item]);
      setNewItemTitle('');
      setLists((prev) =>
        prev.map((l) =>
          l.id === selectedListId ? { ...l, total_items: (l.total_items ?? 0) + 1 } : l
        )
      );
    } catch {
      toast('error', 'Failed to add item');
    }
  };

  const handleToggleItem = async (item: TodoItem) => {
    if (!selectedListId) return;
    const wasCompleted = item.completed;
    setItems((prev) =>
      prev.map((i) =>
        i.id === item.id ? { ...i, completed: !i.completed, completed_at: !i.completed ? new Date().toISOString() : null } : i
      )
    );
    setLists((prev) =>
      prev.map((l) =>
        l.id === selectedListId
          ? { ...l, completed_items: (l.completed_items ?? 0) + (wasCompleted ? -1 : 1) }
          : l
      )
    );
    try {
      const updated = await todoApi.toggleItem(selectedListId, item.id);
      setItems((prev) => prev.map((i) => (i.id === item.id ? updated : i)));
    } catch {
      toast('error', 'Failed to toggle item');
      setItems((prev) => prev.map((i) => (i.id === item.id ? item : i)));
      setLists((prev) =>
        prev.map((l) =>
          l.id === selectedListId
            ? { ...l, completed_items: (l.completed_items ?? 0) + (wasCompleted ? 1 : -1) }
            : l
        )
      );
    }
  };

  const handleUpdateItemTitle = async (item: TodoItem, newTitle: string) => {
    if (!selectedListId || newTitle === item.title) {
      setEditingItemId(null);
      return;
    }
    try {
      const updated = await todoApi.updateItem(selectedListId, item.id, { title: newTitle });
      setItems((prev) => prev.map((i) => (i.id === item.id ? updated : i)));
    } catch {
      toast('error', 'Failed to update item');
    }
    setEditingItemId(null);
  };

  const handleDeleteItem = async (itemId: string) => {
    if (!selectedListId) return;
    const item = items.find((i) => i.id === itemId);
    setItems((prev) => prev.filter((i) => i.id !== itemId));
    if (item) {
      setLists((prev) =>
        prev.map((l) =>
          l.id === selectedListId
            ? {
                ...l,
                total_items: (l.total_items ?? 0) - 1,
                completed_items: item.completed ? (l.completed_items ?? 0) - 1 : (l.completed_items ?? 0),
              }
            : l
        )
      );
    }
    try {
      await todoApi.deleteItem(selectedListId, itemId);
    } catch {
      toast('error', 'Failed to delete item');
      if (selectedListId) fetchItems(selectedListId);
    }
  };

  // --- Drag and drop ---
  const handleDragStart = (e: React.DragEvent, itemId: string) => {
    setDragItemId(itemId);
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', itemId);
    // Make the drag image slightly transparent
    if (e.currentTarget instanceof HTMLElement) {
      e.currentTarget.style.opacity = '0.5';
    }
  };

  const handleDragEnd = (e: React.DragEvent) => {
    if (e.currentTarget instanceof HTMLElement) {
      e.currentTarget.style.opacity = '1';
    }
    setDragItemId(null);
    setDragOverItemId(null);
  };

  const handleDragOver = (e: React.DragEvent, itemId: string) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (dragItemId && itemId !== dragItemId) {
      setDragOverItemId(itemId);
    }
  };

  const handleDrop = async (e: React.DragEvent, targetId: string) => {
    e.preventDefault();
    if (!dragItemId || !selectedListId || dragItemId === targetId) {
      setDragItemId(null);
      setDragOverItemId(null);
      return;
    }

    // Only allow reorder within incomplete items
    const dragItem = incompleteItems.find((i) => i.id === dragItemId);
    const targetItem = incompleteItems.find((i) => i.id === targetId);
    if (!dragItem || !targetItem) {
      setDragItemId(null);
      setDragOverItemId(null);
      return;
    }

    // Reorder incomplete items locally
    const oldList = [...incompleteItems];
    const dragIdx = oldList.findIndex((i) => i.id === dragItemId);
    const targetIdx = oldList.findIndex((i) => i.id === targetId);
    const [moved] = oldList.splice(dragIdx, 1);
    oldList.splice(targetIdx, 0, moved);

    // Apply new sort_order values and update items state
    const reordered = oldList.map((item, idx) => ({ ...item, sort_order: idx }));
    setItems((prev) => {
      const completedItems = prev.filter((i) => i.completed);
      return [...reordered, ...completedItems];
    });

    setDragItemId(null);
    setDragOverItemId(null);

    try {
      await todoApi.reorderItems(
        selectedListId,
        reordered.map((item, idx) => ({ id: item.id, sort_order: idx }))
      );
    } catch {
      toast('error', 'Failed to reorder items');
      if (selectedListId) fetchItems(selectedListId);
    }
  };

  // --- Sort items ---
  const incompleteItems = [...items]
    .filter((i) => !i.completed)
    .sort((a, b) => a.sort_order - b.sort_order);
  const completedItems = [...items]
    .filter((i) => i.completed)
    .sort((a, b) => a.sort_order - b.sort_order);
  const completedCount = completedItems.length;

  // --- Loading ---
  if (loading) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Todo Lists" />
        <div className="flex-1 flex items-center justify-center">
          <div className="text-text-3 text-sm">Loading...</div>
        </div>
      </div>
    );
  }

  // --- Empty state ---
  if (lists.length === 0) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Todo Lists" />
        <div className="flex-1 flex items-center justify-center">
          <EmptyState
            icon={<ListTodo className="w-8 h-8" />}
            title="No Todo Lists"
            description="Create your first todo list to start tracking tasks."
            action={
              <Button variant="primary" size="sm" icon={<Plus className="w-4 h-4" />} onClick={() => setListModalOpen(true)}>
                New List
              </Button>
            }
          />
        </div>
        <ListFormModal
          key={listModalOpen ? 'create-open' : 'create-closed'}
          open={listModalOpen}
          onClose={() => { setListModalOpen(false); setEditingList(null); }}
          onSubmit={handleCreateList}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header title="Todo Lists" />

      <div className="flex flex-1 min-h-0">
        {/* Left panel: list of lists */}
        <div
          className={`w-full md:w-64 border-r border-border-0 flex flex-col bg-surface-0 ${
            selectedListId ? 'hidden md:flex' : 'flex'
          }`}
        >
          <div className="flex items-center justify-between px-3 py-3 border-b border-border-0">
            <span className="text-xs font-semibold text-text-3 uppercase tracking-wider">Lists</span>
            <button
              onClick={() => { setEditingList(null); setListModalOpen(true); }}
              className="p-1 rounded-md text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
              aria-label="New list"
              title="New list"
            >
              <Plus className="w-4 h-4" />
            </button>
          </div>

          <div className="flex-1 overflow-y-auto py-1">
            {lists.map((list) => {
              const total = list.total_items ?? 0;
              const completed = list.completed_items ?? 0;
              return (
                <div key={list.id} className="group relative">
                  <button
                    onClick={() => setSelectedListId(list.id)}
                    className={`w-full flex items-center gap-2.5 px-3 py-2.5 text-left transition-colors cursor-pointer ${
                      selectedListId === list.id
                        ? 'bg-accent-muted text-accent-text'
                        : 'text-text-1 hover:bg-surface-2'
                    }`}
                  >
                    <span
                      className="w-3 h-3 rounded-full flex-shrink-0"
                      style={{ backgroundColor: list.color || colorPresets[5] }}
                    />
                    <span className="flex-1 text-sm font-medium truncate">{list.name}</span>
                    <span className="text-xs text-text-3 tabular-nums group-hover:opacity-0 transition-opacity">
                      {completed}/{total}
                    </span>
                  </button>
                  {/* Hover action badges on dark rounded bg */}
                  <div className="absolute right-2 top-1/2 -translate-y-1/2 hidden group-hover:flex items-center gap-0.5 bg-surface-3 rounded-lg px-1 py-0.5 shadow-lg border border-border-0">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setEditingList(list);
                        setListModalOpen(true);
                      }}
                      className="p-1 rounded-md text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                      aria-label="Edit list"
                      title="Edit list"
                    >
                      <Pencil className="w-3 h-3" />
                    </button>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteList(list.id);
                      }}
                      className="p-1 rounded-md text-text-3 hover:text-red-400 hover:bg-surface-2 transition-colors cursor-pointer"
                      aria-label="Delete list"
                      title="Delete list"
                    >
                      <Trash2 className="w-3 h-3" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {/* Right panel: items */}
        <div
          className={`flex-1 flex flex-col bg-surface-0 ${
            selectedListId ? 'flex' : 'hidden md:flex'
          }`}
        >
          {selectedList ? (
            <>
              {/* List header */}
              <div className="flex items-center gap-3 px-4 md:px-6 py-3 border-b border-border-0">
                <button
                  onClick={() => setSelectedListId(null)}
                  className="md:hidden p-1 rounded-md text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                  aria-label="Back to lists"
                >
                  <ArrowLeft className="w-5 h-5" />
                </button>
                <span
                  className="w-3 h-3 rounded-full flex-shrink-0"
                  style={{ backgroundColor: selectedList.color || colorPresets[5] }}
                />
                <div className="flex-1 min-w-0">
                  <h2 className="text-base font-semibold text-text-1 truncate">
                    {selectedList.name}
                  </h2>
                  {selectedList.description && (
                    <p className="text-xs text-text-3 truncate">{selectedList.description}</p>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-xs text-text-3 tabular-nums">
                    {selectedList.completed_items ?? 0}/{selectedList.total_items ?? 0}
                  </span>
                  <button
                    onClick={() => setShowCompleted(!showCompleted)}
                    className="flex items-center gap-1.5 px-2 py-1 rounded-md text-xs text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                    title={showCompleted ? 'Hide completed' : 'Show completed'}
                  >
                    {showCompleted ? <Eye className="w-3.5 h-3.5" /> : <EyeOff className="w-3.5 h-3.5" />}
                    <span className="hidden sm:inline">{completedCount} done</span>
                  </button>
                </div>
              </div>

              {/* Quick add */}
              <form onSubmit={handleAddItem} className="flex items-center gap-2 px-4 md:px-6 py-3 border-b border-border-0">
                <input
                  type="text"
                  value={newItemTitle}
                  onChange={(e) => setNewItemTitle(e.target.value)}
                  placeholder="Add a new item..."
                  className="flex-1 px-3 py-2 rounded-lg bg-surface-2 border border-border-0 text-text-1 text-sm placeholder:text-text-3 focus:outline-none focus:ring-2 focus:ring-accent-primary"
                />
                <Button variant="primary" size="sm" type="submit" disabled={!newItemTitle.trim()} icon={<Plus className="w-4 h-4" />}>
                  Add
                </Button>
              </form>

              {/* Items list */}
              <div className="flex-1 overflow-y-auto">
                {itemsLoading ? (
                  <div className="flex items-center justify-center py-12">
                    <span className="text-sm text-text-3">Loading items...</span>
                  </div>
                ) : items.length === 0 ? (
                  <div className="flex items-center justify-center py-12">
                    <span className="text-sm text-text-3">No items yet. Add one above.</span>
                  </div>
                ) : (
                  <div>
                    {/* Incomplete items - draggable */}
                    {incompleteItems.length > 0 && (
                      <ul>
                        {incompleteItems.map((item) => (
                          <li
                            key={item.id}
                            draggable
                            onDragStart={(e) => handleDragStart(e, item.id)}
                            onDragEnd={handleDragEnd}
                            onDragOver={(e) => handleDragOver(e, item.id)}
                            onDrop={(e) => handleDrop(e, item.id)}
                            className={`group flex items-center gap-2 px-4 md:px-6 py-2.5 border-b border-border-0/50 transition-colors ${
                              dragOverItemId === item.id && dragItemId !== item.id
                                ? 'border-t-2 border-t-accent-primary'
                                : ''
                            } ${dragItemId === item.id ? 'opacity-50' : ''} hover:bg-surface-1/50`}
                          >
                            {/* Drag handle */}
                            <span className="flex-shrink-0 cursor-grab active:cursor-grabbing text-text-3 opacity-0 group-hover:opacity-60 transition-opacity">
                              <GripVertical className="w-4 h-4" />
                            </span>

                            {/* Checkbox */}
                            <button
                              onClick={() => handleToggleItem(item)}
                              className="flex-shrink-0 w-5 h-5 rounded-md border-2 border-border-1 hover:border-accent-primary flex items-center justify-center transition-colors cursor-pointer"
                              aria-label="Mark complete"
                            >
                              {/* empty */}
                            </button>

                            {/* Title */}
                            <div className="flex-1 min-w-0 flex items-center gap-2">
                              {editingItemId === item.id ? (
                                <InlineEdit
                                  value={item.title}
                                  onSave={(v) => handleUpdateItemTitle(item, v)}
                                  onCancel={() => setEditingItemId(null)}
                                />
                              ) : (
                                <span
                                  className="text-sm text-text-1 cursor-pointer hover:text-accent-text transition-colors truncate"
                                  onClick={() => setEditingItemId(item.id)}
                                  title="Click to edit"
                                >
                                  {item.title}
                                </span>
                              )}

                              {item.last_actor_agent_slug && item.last_actor_avatar && (
                                <span
                                  className="inline-flex items-center flex-shrink-0"
                                  title={`${item.last_actor_agent_name ?? item.last_actor_agent_slug}: ${item.last_actor_note}`}
                                >
                                  <img
                                    src={`/api/v1/uploads/avatars/${item.last_actor_avatar}`}
                                    alt=""
                                    className="w-4 h-4 rounded-full"
                                    onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }}
                                  />
                                </span>
                              )}
                            </div>

                            {item.due_date && (
                              <span className="text-[10px] text-text-3 flex-shrink-0 tabular-nums">
                                {new Date(item.due_date).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                              </span>
                            )}

                            {/* Delete */}
                            <button
                              onClick={() => handleDeleteItem(item.id)}
                              className="flex-shrink-0 p-1 rounded text-text-3 opacity-0 group-hover:opacity-100 hover:text-red-400 hover:bg-surface-2 transition-all cursor-pointer"
                              aria-label="Delete item"
                              title="Delete item"
                            >
                              <X className="w-3.5 h-3.5" />
                            </button>
                          </li>
                        ))}
                      </ul>
                    )}

                    {/* Completed section */}
                    {completedCount > 0 && showCompleted && (
                      <div>
                        <div className="px-4 md:px-6 py-2 bg-surface-1/30">
                          <span className="text-xs font-medium text-text-3 uppercase tracking-wider">
                            Completed ({completedCount})
                          </span>
                        </div>
                        <ul>
                          {completedItems.map((item) => (
                            <li
                              key={item.id}
                              className="group flex items-center gap-2 px-4 md:px-6 py-2 border-b border-border-0/30 hover:bg-surface-1/30 transition-colors"
                            >
                              {/* Spacer for drag handle alignment */}
                              <span className="flex-shrink-0 w-4" />

                              {/* Checkbox - filled */}
                              <button
                                onClick={() => handleToggleItem(item)}
                                className="flex-shrink-0 w-5 h-5 rounded-md bg-accent-primary/80 border-2 border-accent-primary/80 flex items-center justify-center transition-colors cursor-pointer hover:bg-accent-primary"
                                aria-label="Mark incomplete"
                              >
                                <Check className="w-3 h-3 text-white" />
                              </button>

                              {/* Title - struck through but visible */}
                              <div className="flex-1 min-w-0 flex items-center gap-2">
                                <span className="text-sm text-text-3 line-through decoration-text-3/50 truncate">
                                  {item.title}
                                </span>

                                {item.last_actor_agent_slug && item.last_actor_avatar && (
                                  <span
                                    className="inline-flex items-center flex-shrink-0"
                                    title={`${item.last_actor_agent_name ?? item.last_actor_agent_slug}: ${item.last_actor_note}`}
                                  >
                                    <img
                                      src={`/api/v1/uploads/avatars/${item.last_actor_avatar}`}
                                      alt=""
                                      className="w-4 h-4 rounded-full opacity-60"
                                      onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; }}
                                    />
                                  </span>
                                )}
                              </div>

                              {/* Delete */}
                              <button
                                onClick={() => handleDeleteItem(item.id)}
                                className="flex-shrink-0 p-1 rounded text-text-3 opacity-0 group-hover:opacity-100 hover:text-red-400 hover:bg-surface-2 transition-all cursor-pointer"
                                aria-label="Delete item"
                                title="Delete item"
                              >
                                <X className="w-3.5 h-3.5" />
                              </button>
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}

                    {/* Hidden completed indicator */}
                    {completedCount > 0 && !showCompleted && (
                      <div className="px-4 md:px-6 py-3">
                        <button
                          onClick={() => setShowCompleted(true)}
                          className="text-xs text-text-3 hover:text-text-2 transition-colors cursor-pointer"
                        >
                          + {completedCount} completed {completedCount === 1 ? 'item' : 'items'}
                        </button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </>
          ) : (
            <div className="flex-1 flex items-center justify-center">
              <div className="text-center text-text-3">
                <ListTodo className="w-10 h-10 mx-auto mb-3 opacity-40" />
                <p className="text-sm">Select a list to view its items</p>
              </div>
            </div>
          )}
        </div>
      </div>

      <ListFormModal
        key={editingList ? `edit-${editingList.id}` : listModalOpen ? 'create-open' : 'create-closed'}
        open={listModalOpen}
        onClose={() => { setListModalOpen(false); setEditingList(null); }}
        onSubmit={editingList ? handleUpdateList : handleCreateList}
        initial={editingList ? { name: editingList.name, description: editingList.description, color: editingList.color } : undefined}
      />
    </div>
  );
}
