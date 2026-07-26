/**
 * TaskComposer — simple and advanced modes for adding a task.
 *
 * Simple is the old one-line field, unchanged, because most items really are
 * one line. Advanced exists because a task is increasingly a prompt an agent
 * will pick up: it gets a body, the material that prompt refers to, and an
 * Enhance pass that rewrites it into something actionable.
 *
 * Attachments store a real on-disk path rather than a browser URL — the point
 * is that the agent can open them, and a /api/v1/... URL is not something an
 * agent has any way to read.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Plus, Loader2, X, Paperclip, FolderOpen, FileText,
  ImageIcon, Film, Music, Wand2,
} from 'lucide-react';
import { Button } from '../Button';
import { useToast } from '../Toast';
import { api, contextApi, type ContextFile, type ContextTree } from '../../lib/api';
import { mediaApi } from '../../lib/api-helpers';
import type { MediaItem, TodoAttachment } from '../../lib/types';

type Mode = 'simple' | 'advanced';

const MODE_KEY = 'openpaw_task_composer_mode';

interface Props {
  onAdd: (draft: { title: string; notes: string; attachments: TodoAttachment[] }) => Promise<void>;
}

/** Icon per attachment kind — the row is narrow, so the glyph does the work. */
function kindIcon(kind: string) {
  switch (kind) {
    case 'image': return ImageIcon;
    case 'directory': return FolderOpen;
    case 'media': return Film;
    default: return FileText;
  }
}

export function TaskComposer({ onAdd }: Props) {
  const { toast } = useToast();

  const [mode, setMode] = useState<Mode>(
    () => (localStorage.getItem(MODE_KEY) === 'advanced' ? 'advanced' : 'simple'),
  );
  useEffect(() => { localStorage.setItem(MODE_KEY, mode); }, [mode]);

  const [title, setTitle] = useState('');
  const [notes, setNotes] = useState('');
  const [attachments, setAttachments] = useState<TodoAttachment[]>([]);
  const [saving, setSaving] = useState(false);
  const [enhancing, setEnhancing] = useState(false);
  const [pasting, setPasting] = useState(false);
  const [picker, setPicker] = useState<null | 'context' | 'media'>(null);

  const [contextTree, setContextTree] = useState<ContextTree | null>(null);
  const [media, setMedia] = useState<MediaItem[]>([]);
  const pickerRef = useRef<HTMLDivElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (pickerRef.current && !pickerRef.current.contains(e.target as Node)) setPicker(null);
    }
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, []);

  // Loaded on first open rather than on mount — the advanced panel is opt-in
  // and most sessions never touch it.
  const openPicker = useCallback(async (which: 'context' | 'media') => {
    setPicker(p => (p === which ? null : which));
    if (which === 'context' && !contextTree) {
      contextApi.tree().then(setContextTree).catch(() => {});
    }
    if (which === 'media' && media.length === 0) {
      mediaApi.list().then(d => setMedia(d?.items ?? [])).catch(() => {});
    }
  }, [contextTree, media.length]);

  // Keyed on ref-or-path: a context file has no path until the server
  // resolves it, so comparing paths alone would let it be added twice.
  const keyOf = (a: TodoAttachment) => a.ref || a.path;

  const add = (a: TodoAttachment) => {
    setAttachments(prev =>
      prev.some(p => keyOf(p) === keyOf(a)) ? prev : [...prev, a],
    );
    setPicker(null);
  };

  const uploadImage = async (file: File | undefined) => {
    if (!file) return;
    if (!file.type.startsWith('image/')) {
      toast('error', 'That is not an image.');
      return;
    }
    setPasting(true);
    try {
      // Reuses the chat paste endpoint: it already writes the file to disk and
      // hands back the absolute path, which is exactly what an agent needs.
      const image = await contextApi.uploadPastedImage(file);
      add({ kind: 'image', path: image.path, name: image.name });
    } catch (e) {
      toast('error', e instanceof Error ? e.message : 'Could not attach that image');
    } finally {
      setPasting(false);
    }
  };

  const onPaste = (e: React.ClipboardEvent) => {
    const img = Array.from(e.clipboardData?.items ?? []).find(i => i.type.startsWith('image/'));
    if (!img) return;
    e.preventDefault();
    uploadImage(img.getAsFile() ?? undefined);
  };

  const attachDirectory = async () => {
    setPicker(null);
    try {
      const result = await api.post<{ path: string }>('/system/pick-folder', {});
      if (result.path) {
        add({ kind: 'directory', path: result.path, name: result.path.split('/').filter(Boolean).pop() || result.path });
      }
    } catch {
      /* dialog cancelled */
    }
  };

  const enhance = async () => {
    if (!title.trim() && !notes.trim()) return;
    setEnhancing(true);
    try {
      const res = await api.post<{ notes: string }>('/todo-lists/enhance', {
        title: title.trim(),
        notes: notes.trim(),
        attachments,
      });
      setNotes(res.notes);
      toast('success', 'Rewritten — edit it before adding if you like');
    } catch (e) {
      toast('error', e instanceof Error ? e.message : 'Enhance failed');
    } finally {
      setEnhancing(false);
    }
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim() || saving) return;
    setSaving(true);
    try {
      await onAdd({ title: title.trim(), notes: notes.trim(), attachments });
      setTitle('');
      setNotes('');
      setAttachments([]);
    } finally {
      setSaving(false);
    }
  };

  const contextFiles: ContextFile[] = contextTree
    ? [...contextTree.files, ...contextTree.folders.flatMap(f => collectFiles(f))]
    : [];

  return (
    <form onSubmit={submit} className="px-4 md:px-6 py-3 border-b border-border-0 space-y-2">
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={title}
          onChange={e => setTitle(e.target.value)}
          onPaste={mode === 'advanced' ? onPaste : undefined}
          placeholder="Add a new item..."
          className="flex-1 px-3 py-2 rounded-lg bg-surface-2 border border-border-0 text-text-1 text-sm placeholder:text-text-3 focus:outline-none focus:ring-2 focus:ring-accent-primary"
        />
        <Button variant="primary" size="sm" type="submit" disabled={!title.trim() || saving} icon={<Plus className="w-4 h-4" />}>
          Add
        </Button>
      </div>

      <div className="flex items-center gap-1 rounded-lg border border-border-0 bg-surface-2 p-0.5 w-fit">
        {(['simple', 'advanced'] as Mode[]).map(m => (
          <button
            key={m}
            type="button"
            onClick={() => setMode(m)}
            aria-pressed={mode === m}
            className={`px-2.5 py-1 rounded-md text-[11px] font-medium capitalize transition-colors cursor-pointer ${
              mode === m ? 'bg-accent-primary/15 text-accent-text' : 'text-text-3 hover:text-text-1'
            }`}
          >
            {m}
          </button>
        ))}
      </div>

      {mode === 'advanced' && (
        <div className="space-y-2 pt-1">
          <textarea
            value={notes}
            onChange={e => setNotes(e.target.value)}
            onPaste={onPaste}
            rows={5}
            placeholder="Detail for the agent picking this up — what done looks like, constraints, anything it should read first. Paste an image to attach it."
            className="w-full px-3 py-2 rounded-lg bg-surface-2 border border-border-0 text-text-1 text-sm placeholder:text-text-3 focus:outline-none focus:ring-2 focus:ring-accent-primary resize-y"
          />

          {attachments.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {attachments.map(a => {
                const Icon = kindIcon(a.kind);
                return (
                  <span
                    key={keyOf(a)}
                    title={a.path || a.name}
                    className="inline-flex items-center gap-1.5 px-2 py-1 rounded-lg bg-surface-3 text-text-2 text-[11px] max-w-[240px]"
                  >
                    <Icon className="w-3 h-3 flex-shrink-0" aria-hidden="true" />
                    <span className="truncate">{a.name}</span>
                    <button
                      type="button"
                      onClick={() => setAttachments(prev => prev.filter(p => keyOf(p) !== keyOf(a)))}
                      className="ml-0.5 hover:text-danger cursor-pointer flex-shrink-0"
                      aria-label={`Remove ${a.name}`}
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </span>
                );
              })}
            </div>
          )}

          <div ref={pickerRef} className="relative flex flex-wrap items-center gap-1.5">
            <PickerButton icon={ImageIcon} label="Image" busy={pasting} onClick={() => fileRef.current?.click()} />
            <PickerButton icon={FileText} label="Context file" onClick={() => openPicker('context')} />
            <PickerButton icon={FolderOpen} label="Directory" onClick={attachDirectory} />
            <PickerButton icon={Film} label="Studio media" onClick={() => openPicker('media')} />

            <span className="flex-1" />

            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={enhance}
              loading={enhancing}
              disabled={!title.trim() && !notes.trim()}
              icon={enhancing ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Wand2 className="w-3.5 h-3.5" />}
              title="Rewrite this as a clearer prompt using your connected model"
            >
              Enhance
            </Button>

            {picker === 'context' && (
              <PickerPanel empty="No context files yet.">
                {contextFiles.map(f => (
                  <PickerRow
                    key={f.id}
                    icon={FileText}
                    label={f.name}
                    onClick={() => add({ kind: 'file', ref: f.id, path: '', name: f.name })}
                  />
                ))}
              </PickerPanel>
            )}

            {picker === 'media' && (
              <PickerPanel empty="Nothing in Studio yet.">
                {media.map(m => (
                  <PickerRow
                    key={m.id}
                    icon={m.media_type === 'video' ? Film : m.media_type === 'audio' ? Music : ImageIcon}
                    label={m.prompt || m.id}
                    onClick={() => add({ kind: 'media', ref: m.id, path: '', name: m.prompt?.slice(0, 60) || m.filename || 'media' })}
                  />
                ))}
              </PickerPanel>
            )}
          </div>

          <p className="text-[11px] text-text-3 leading-relaxed flex items-center gap-1">
            <Paperclip className="w-3 h-3 flex-shrink-0" aria-hidden="true" />
            Agents are given the real paths, so they can open anything attached here.
          </p>
        </div>
      )}

      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={e => { uploadImage(e.target.files?.[0]); e.target.value = ''; }}
      />
    </form>
  );
}

function PickerButton({
  icon: Icon, label, onClick, busy,
}: { icon: typeof FileText; label: string; onClick: () => void; busy?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center gap-1.5 rounded-lg border border-border-1 bg-surface-2 px-2.5 py-1.5 text-[11px] text-text-2 hover:text-text-1 hover:border-border-0 transition-colors cursor-pointer"
    >
      {busy ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Icon className="w-3.5 h-3.5" aria-hidden="true" />}
      {label}
    </button>
  );
}

function PickerPanel({ children, empty }: { children: React.ReactNode; empty: string }) {
  const list = Array.isArray(children) ? children : [children];
  const isEmpty = list.filter(Boolean).length === 0;
  return (
    <div className="absolute top-full left-0 mt-1 w-72 max-h-64 overflow-y-auto rounded-xl border border-border-0 bg-surface-1 shadow-2xl py-1 z-50">
      {isEmpty ? <p className="px-3 py-2 text-[11px] text-text-3">{empty}</p> : children}
    </div>
  );
}

function PickerRow({
  icon: Icon, label, onClick,
}: { icon: typeof FileText; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={label}
      className="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-surface-2 transition-colors cursor-pointer"
    >
      <Icon className="w-3.5 h-3.5 text-text-3 flex-shrink-0" aria-hidden="true" />
      <span className="text-xs text-text-1 truncate">{label}</span>
    </button>
  );
}

/** Context folders nest; the picker wants one flat list of files. */
function collectFiles(node: { files: ContextFile[]; children: { files: ContextFile[]; children: unknown[] }[] }): ContextFile[] {
  const out = [...node.files];
  for (const child of node.children) {
    out.push(...collectFiles(child as Parameters<typeof collectFiles>[0]));
  }
  return out;
}
