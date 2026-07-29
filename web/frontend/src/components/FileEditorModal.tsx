/**
 * FileEditorModal
 *
 * Quick view/edit for a single file in the Directory tab. The Directory tree
 * could already show you that a file existed but not what was in it, so any
 * one-line fix meant leaving the app.
 *
 * Scope is deliberately "quick edit", not an IDE: a plain textarea with tab
 * support. The server refuses anything oversized or non-UTF-8, so a binary
 * can't be opened, mangled by a round trip through a textarea, and written
 * back over the original.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { X, Save, FileText, Loader2, AlertTriangle, FolderSearch } from 'lucide-react';
import { Button } from './Button';
import { workspaces } from '../lib/api-helpers';
import { activatePathInsertionTarget, clearPathInsertionTarget } from '../lib/path-insertion';
import { useHotkeys } from '../contexts/hotkeys';

interface FileEditorModalProps {
  workspaceId: string;
  /** Attached-directory id, or "" for the workspace's own files dir. */
  dirId: string;
  /** Path relative to that base. */
  path: string;
  name: string;
  onClose: () => void;
}

function errorMessage(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

export function FileEditorModal({ workspaceId, dirId, path, name, onClose }: FileEditorModalProps) {
  const { setPaletteOpen } = useHotkeys();
  const [content, setContent] = useState('');
  const [original, setOriginal] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => () => clearPathInsertionTarget(`file-editor:${dirId}:${path}`), [dirId, path]);

  const dirty = content !== original;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    workspaces
      .readFile(workspaceId, dirId, path)
      .then(res => {
        if (cancelled) return;
        setContent(res.content);
        setOriginal(res.content);
      })
      .catch(e => {
        if (!cancelled) setError(errorMessage(e, 'Failed to open this file'));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [workspaceId, dirId, path]);

  const save = useCallback(async () => {
    if (!dirty || saving) return;
    setSaving(true);
    setError(null);
    try {
      const res = await workspaces.writeFile(workspaceId, dirId, path, content);
      setOriginal(content);
      setSavedAt(new Date(res.modified_at).toLocaleTimeString());
    } catch (e) {
      setError(errorMessage(e, 'Failed to save'));
    } finally {
      setSaving(false);
    }
  }, [dirty, saving, workspaceId, dirId, path, content]);

  // Closing with unsaved edits asks first — this edits real files in the user's
  // repositories, so a stray Escape must not silently discard work.
  const requestClose = useCallback(() => {
    if (dirty && !window.confirm(`Discard unsaved changes to ${name}?`)) return;
    onClose();
  }, [dirty, name, onClose]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        requestClose();
      }
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault();
        save();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [requestClose, save]);

  // Tab indents instead of moving focus — in a code editor, losing the caret to
  // the next control on Tab is worse than trapping it.
  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key !== 'Tab') return;
    e.preventDefault();
    const el = e.currentTarget;
    const { selectionStart: start, selectionEnd: end } = el;
    const next = content.slice(0, start) + '  ' + content.slice(end);
    setContent(next);
    requestAnimationFrame(() => {
      el.selectionStart = el.selectionEnd = start + 2;
    });
  };

  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm"
      onClick={e => { if (e.target === e.currentTarget) requestClose(); }}
      role="dialog"
      aria-modal="true"
      aria-label={`Edit ${name}`}
    >
      <div className="w-full max-w-4xl h-[80vh] flex flex-col rounded-2xl border border-border-0 bg-surface-1 shadow-2xl overflow-hidden">
        <div className="flex items-center gap-3 px-4 py-3 border-b border-border-0 flex-shrink-0">
          <FileText className="w-4 h-4 text-accent-primary flex-shrink-0" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-text-0 truncate">{name}</p>
            <p className="text-[11px] text-text-3 truncate" title={path}>{path}</p>
          </div>
          {dirty && (
            <span className="text-[11px] text-amber-400 flex-shrink-0">Unsaved changes</span>
          )}
          {!dirty && savedAt && (
            <span className="text-[11px] text-text-3 flex-shrink-0">Saved {savedAt}</span>
          )}
          <button
            type="button"
            onMouseDown={event => event.preventDefault()}
            onClick={() => setPaletteOpen(true)}
            aria-label="Insert workspace path"
            title="Insert workspace path"
            className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg text-text-2 transition-colors hover:bg-surface-2 hover:text-text-0 cursor-pointer"
          >
            <FolderSearch className="h-4 w-4" aria-hidden="true" />
          </button>
          <Button
            size="sm"
            icon={saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
            onClick={save}
            disabled={!dirty || saving || loading}
          >
            {saving ? 'Saving…' : 'Save'}
          </Button>
          <button
            onClick={requestClose}
            aria-label="Close editor"
            className="p-1.5 rounded-lg text-text-2 hover:text-text-0 hover:bg-surface-2 transition-colors cursor-pointer flex-shrink-0"
          >
            <X className="w-4 h-4" aria-hidden="true" />
          </button>
        </div>

        {error && (
          <div className="flex items-start gap-2 px-4 py-2.5 bg-danger/10 border-b border-danger/20 flex-shrink-0">
            <AlertTriangle className="w-4 h-4 text-danger flex-shrink-0 mt-0.5" aria-hidden="true" />
            <p className="text-xs text-danger leading-relaxed">{error}</p>
          </div>
        )}

        <div className="flex-1 min-h-0">
          {loading ? (
            <div className="h-full flex items-center justify-center">
              <Loader2 className="w-5 h-5 text-text-3 animate-spin" aria-hidden="true" />
            </div>
          ) : error && !content ? (
            <div className="h-full flex items-center justify-center px-6">
              <p className="text-sm text-text-3 text-center max-w-sm">
                This file can't be opened in the editor. Binary files and files over 2&nbsp;MB are
                excluded.
              </p>
            </div>
          ) : (
            <textarea
              ref={textareaRef}
              data-openpaw-hotkeys="ignore"
              value={content}
              onChange={e => setContent(e.target.value)}
              onFocus={event => {
                const textarea = event.currentTarget;
                activatePathInsertionTarget({
                  id: `file-editor:${dirId}:${path}`,
                  label: name,
                  insert: insertedPath => {
                    const start = textarea.selectionStart;
                    const end = textarea.selectionEnd;
                    setContent(current => `${current.slice(0, start)}${insertedPath}${current.slice(end)}`);
                    requestAnimationFrame(() => {
                      textarea.focus();
                      textarea.selectionStart = textarea.selectionEnd = start + insertedPath.length;
                    });
                  },
                });
              }}
              onKeyDown={onKeyDown}
              spellCheck={false}
              autoComplete="off"
              autoCorrect="off"
              autoCapitalize="off"
              className="w-full h-full resize-none bg-surface-0 text-text-1 font-mono text-[13px] leading-relaxed px-4 py-3 outline-none border-0"
            />
          )}
        </div>

        <div className="flex items-center justify-between px-4 py-2 border-t border-border-0 flex-shrink-0 text-[11px] text-text-3">
          <span>{content.split('\n').length} lines</span>
          <span>
            <kbd className="px-1 py-0.5 rounded bg-surface-2 border border-border-1">⌘S</kbd> save
            {' · '}
            <kbd className="px-1 py-0.5 rounded bg-surface-2 border border-border-1">Esc</kbd> close
          </span>
        </div>
      </div>
    </div>,
    document.body,
  );
}
