import { useState, useRef } from 'react';
import type { ChatThread } from '../lib/api';

export function useThreadList() {
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [threadSearch, setThreadSearch] = useState('');
  const [threadPage, setThreadPage] = useState(1);
  const [editingThread, setEditingThread] = useState<string | null>(null);
  const [editTitle, setEditTitle] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<ChatThread | null>(null);
  const editInputRef = useRef<HTMLInputElement>(null);

  return {
    threads, setThreads,
    threadSearch, setThreadSearch,
    threadPage, setThreadPage,
    editingThread, setEditingThread,
    editTitle, setEditTitle,
    deleteTarget, setDeleteTarget,
    editInputRef,
  };
}
