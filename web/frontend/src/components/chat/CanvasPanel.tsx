/**
 * CanvasPanel
 *
 * The work pane beside the chat: either a local/remote page preview or a live
 * editor for a document stored in Context.
 *
 * Either side can drive it. The user can type a URL/path or open a Context
 * document; agents use canvas_show or canvas_show_document over the websocket.
 *
 * Local files can't be loaded as file:// from an http page, so they are served
 * back through /api/v1/canvas/fs/<abs path> — a path typed in here is rewritten
 * to that, and shown as the plain path so the address stays readable.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  RotateCw,
  ExternalLink,
  X,
  Monitor,
  Smartphone,
  MonitorPlay,
  FileText,
  Pencil,
  Eye,
  Save,
  Loader2,
  FolderOpen,
  AlertTriangle,
} from 'lucide-react';
import { openExternal } from '../../lib/openExternal';
import { contextApi } from '../../lib/api';
import { markdownLinkComponents } from '../MarkdownLink';
import { useToast } from '../Toast';

const FS_PREFIX = '/api/v1/canvas/fs/';

/** Absolute filesystem path → the URL that serves it. */
function canvasUrlForPath(path: string): string {
  const parts = path.replace(/^\//, '').split('/').map(encodeURIComponent);
  return FS_PREFIX + parts.join('/');
}

/** The inverse, for display: a served file reads as the path it came from. */
function displayUrl(url: string): string {
  if (!url.startsWith(FS_PREFIX)) return url;
  return '/' + url.slice(FS_PREFIX.length).split('/').map(decodeURIComponent).join('/');
}

/**
 * What the user typed, as something loadable. Bare hosts get a scheme —
 * "localhost:5173" is how anyone writes a dev server — and absolute paths go to
 * the file route.
 */
function normalizeInput(raw: string): string {
  const value = raw.trim();
  if (!value) return '';
  if (value.startsWith('/') && !value.startsWith('//')) {
    return value.startsWith('/api/') ? value : canvasUrlForPath(value);
  }
  if (!/^[a-z]+:\/\//i.test(value)) return `http://${value}`;
  return value;
}

export type CanvasEntry =
  | { kind: 'preview'; url: string; title?: string }
  | { kind: 'document'; documentId: string; title?: string };

interface CanvasPanelProps {
  entry?: CanvasEntry;
  onUrlChange: (url: string) => void;
  onClose: () => void;
  onOpenContext: () => void;
  documentRevision?: number;
}

export function CanvasPanel({ entry, onUrlChange, onClose, onOpenContext, documentRevision = 0 }: CanvasPanelProps) {
  if (entry?.kind === 'document') {
    return (
      <DocumentCanvas
        documentId={entry.documentId}
        fallbackTitle={entry.title}
        revision={documentRevision}
        onClose={onClose}
        onOpenContext={onOpenContext}
        onSwitchToPreview={() => onUrlChange('')}
      />
    );
  }
  return (
    <PreviewCanvas
      url={entry?.url || ''}
      title={entry?.title}
      onUrlChange={onUrlChange}
      onClose={onClose}
    />
  );
}

function DocumentCanvas({
  documentId,
  fallbackTitle,
  revision,
  onClose,
  onOpenContext,
  onSwitchToPreview,
}: {
  documentId: string;
  fallbackTitle?: string;
  revision: number;
  onClose: () => void;
  onOpenContext: () => void;
  onSwitchToPreview: () => void;
}) {
  const { toast } = useToast();
  const [title, setTitle] = useState(fallbackTitle || 'Context document');
  const [content, setContent] = useState('');
  const [savedContent, setSavedContent] = useState('');
  const [mode, setMode] = useState<'edit' | 'preview'>('edit');
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [saving, setSaving] = useState(false);
  const [externalUpdate, setExternalUpdate] = useState(false);
  const dirty = content !== savedContent;
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;
  const revisionRef = useRef(revision);

  const loadDocument = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      const result = await contextApi.getFile(documentId);
      const next = result.content ?? '';
      setTitle(result.file.name);
      setContent(next);
      setSavedContent(next);
      setExternalUpdate(false);
    } catch (error) {
      console.warn('load canvas document failed:', error);
      setLoadError('This document is unavailable. It may have been deleted or moved to another workspace.');
      toast('error', 'Could not load the Context document');
    } finally {
      setLoading(false);
    }
  }, [documentId, toast]);

  useEffect(() => {
    revisionRef.current = revision;
    void loadDocument();
    // A revision change is handled below so unsaved local edits can surface a
    // conflict instead of being overwritten. This effect only resets when the
    // canvas switches to a different document.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [documentId, loadDocument]);

  useEffect(() => {
    if (revisionRef.current === revision) return;
    revisionRef.current = revision;
    if (dirtyRef.current) setExternalUpdate(true);
    else void loadDocument();
  }, [revision, loadDocument]);

  const save = async () => {
    if (!dirty || saving) return;
    setSaving(true);
    try {
      await contextApi.updateFile(documentId, { content });
      setSavedContent(content);
      setExternalUpdate(false);
      toast('success', 'Document saved to Context');
    } catch (error) {
      console.warn('save canvas document failed:', error);
      toast('error', 'Could not save the document');
    } finally {
      setSaving(false);
    }
  };

  const close = () => {
    if (dirty && !confirm('Close the canvas and discard your unsaved document changes?')) return;
    onClose();
  };

  const switchToPreview = () => {
    if (dirty && !confirm('Switch canvases and discard your unsaved document changes?')) return;
    onSwitchToPreview();
  };

  const openContext = () => {
    if (dirty && !confirm('Open Context and discard your unsaved document changes?')) return;
    onOpenContext();
  };

  return (
    <div className="flex h-full min-w-0 flex-col bg-surface-0">
      <header className="flex min-h-14 items-center gap-2 border-b border-border-0 bg-surface-1 px-3 py-2">
        <div className="flex min-w-0 flex-1 items-center gap-2.5">
          <span className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-accent-muted text-accent-primary">
            <FileText className="h-[18px] w-[18px]" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-text-0">{title}</p>
            <p className="text-[11px] text-text-3">Context document · {dirty ? 'Unsaved changes' : 'Saved'}</p>
          </div>
        </div>

        <div className="flex items-center gap-1">
          <div className="hidden rounded-lg bg-surface-2 p-0.5 sm:flex" role="group" aria-label="Document view">
            <button
              type="button"
              onClick={() => setMode('edit')}
              aria-pressed={mode === 'edit'}
              className={`inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary ${mode === 'edit' ? 'bg-surface-0 text-text-0 shadow-sm' : 'text-text-3 hover:text-text-1'}`}
            >
              <Pencil className="h-3.5 w-3.5" aria-hidden="true" /> Edit
            </button>
            <button
              type="button"
              onClick={() => setMode('preview')}
              aria-pressed={mode === 'preview'}
              className={`inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary ${mode === 'preview' ? 'bg-surface-0 text-text-0 shadow-sm' : 'text-text-3 hover:text-text-1'}`}
            >
              <Eye className="h-3.5 w-3.5" aria-hidden="true" /> Preview
            </button>
          </div>
          <button
            type="button"
            onClick={openContext}
              className="inline-flex h-10 w-10 items-center justify-center rounded-lg text-text-3 transition-colors hover:bg-surface-2 hover:text-text-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary"
            title="Open in Context"
            aria-label="Open document in Context"
          >
            <FolderOpen className="h-4 w-4" aria-hidden="true" />
          </button>
          <button
            type="button"
            onClick={switchToPreview}
            className="inline-flex h-10 w-10 items-center justify-center rounded-lg text-text-3 transition-colors hover:bg-surface-2 hover:text-text-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary"
            title="Switch to page preview"
            aria-label="Switch canvas to page preview"
          >
            <MonitorPlay className="h-4 w-4" aria-hidden="true" />
          </button>
          <button
            type="button"
            onClick={() => void save()}
            disabled={!dirty || saving}
            className="inline-flex h-10 items-center gap-1.5 rounded-lg bg-accent-primary px-3 text-xs font-semibold text-white transition-colors hover:bg-accent-hover disabled:cursor-not-allowed disabled:opacity-35 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface-1"
            title="Save document"
          >
            {saving ? <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> : <Save className="h-4 w-4" aria-hidden="true" />}
            <span className="hidden md:inline">Save</span>
          </button>
          <button
            type="button"
            onClick={close}
            className="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-border-1 bg-surface-2 text-text-1 shadow-sm transition-colors hover:border-red-400/50 hover:bg-red-500/10 hover:text-red-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary"
            title="Close canvas"
            aria-label="Close canvas"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </header>

      <div className="flex border-b border-border-0 bg-surface-1 px-3 py-1 sm:hidden" role="group" aria-label="Document view">
        <button type="button" onClick={() => setMode('edit')} className={`h-10 flex-1 rounded-md text-xs font-medium ${mode === 'edit' ? 'bg-surface-2 text-text-0' : 'text-text-3'}`}>Edit</button>
        <button type="button" onClick={() => setMode('preview')} className={`h-10 flex-1 rounded-md text-xs font-medium ${mode === 'preview' ? 'bg-surface-2 text-text-0' : 'text-text-3'}`}>Preview</button>
      </div>

      {externalUpdate && (
        <div className="flex items-center gap-2 border-b border-amber-400/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
          <AlertTriangle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
          <span className="min-w-0 flex-1">An agent updated this document while you have unsaved edits.</span>
          <button type="button" onClick={() => void loadDocument()} className="rounded-md px-2 py-1 font-semibold hover:bg-amber-500/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-300">Load latest</button>
        </div>
      )}

      <main className="min-h-0 flex-1 overflow-hidden">
        {loading ? (
          <div className="flex h-full items-center justify-center gap-2 text-sm text-text-3">
            <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> Loading document…
          </div>
        ) : loadError ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 px-8 text-center">
            <AlertTriangle className="h-6 w-6 text-amber-300" aria-hidden="true" />
            <p className="max-w-sm text-sm leading-6 text-text-2">{loadError}</p>
            <button type="button" onClick={() => void loadDocument()} className="rounded-lg border border-border-1 bg-surface-2 px-3 py-2 text-xs font-semibold text-text-1 hover:bg-surface-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary">Try again</button>
          </div>
        ) : mode === 'edit' ? (
          <textarea
            data-openpaw-hotkeys="ignore"
            value={content}
            onChange={(event) => setContent(event.target.value)}
            onKeyDown={(event) => {
              if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
                event.preventDefault();
                void save();
              }
            }}
            spellCheck
            aria-label={`Edit ${title}`}
            className="h-full w-full resize-none bg-transparent px-5 py-5 text-sm leading-7 text-text-1 outline-none placeholder:text-text-3 md:px-8 md:py-7"
            placeholder="Start writing…"
          />
        ) : (
          <div className="h-full overflow-y-auto px-5 py-5 md:px-8 md:py-7">
            <article className="prose-chat prose-measure mx-auto max-w-3xl">
              {content.trim() ? (
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownLinkComponents}>{content}</ReactMarkdown>
              ) : (
                <p className="text-sm text-text-3">This document is empty.</p>
              )}
            </article>
          </div>
        )}
      </main>
    </div>
  );
}

function PreviewCanvas({ url, title, onUrlChange, onClose }: { url: string; title?: string; onUrlChange: (url: string) => void; onClose: () => void }) {
  const [draft, setDraft] = useState(() => displayUrl(url));
  const [narrow, setNarrow] = useState(false);
  // Bumped to remount the iframe. Reassigning the same src does nothing, and a
  // cross-origin frame's own location is off limits.
  const [reloadKey, setReloadKey] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  // An agent changing the canvas has to be visible in the address bar too —
  // unless the user is mid-edit, where overwriting what they are typing would
  // be its own bug. Adjusted during render rather than in an effect: this is
  // state derived from a prop, and an effect would paint the stale address
  // first.
  const [editing, setEditing] = useState(false);
  const [syncedUrl, setSyncedUrl] = useState(url);
  if (syncedUrl !== url) {
    setSyncedUrl(url);
    if (!editing) setDraft(displayUrl(url));
  }

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const next = normalizeInput(draft);
    if (!next) return;
    if (next === url) setReloadKey(k => k + 1);
    else onUrlChange(next);
    inputRef.current?.blur();
  };

  const isLocalFile = url.startsWith(FS_PREFIX);

  return (
    <div className="flex flex-col h-full min-w-0 bg-surface-0">
      <div className="flex items-center gap-1.5 px-2 py-1.5 border-b border-border-0 bg-surface-1">
        <form onSubmit={submit} className="flex-1 min-w-0">
          <input
            ref={inputRef}
            value={draft}
            onChange={e => setDraft(e.target.value)}
            onFocus={() => setEditing(true)}
            onBlur={() => setEditing(false)}
            onKeyDown={e => { if (e.key === 'Escape') { setDraft(displayUrl(url)); inputRef.current?.blur(); } }}
            placeholder="localhost:5173 or /path/to/index.html"
            spellCheck={false}
            autoComplete="off"
            aria-label="Canvas address"
            className="h-8 w-full rounded-lg border border-border-1 bg-surface-2 px-2.5 text-xs text-text-1 placeholder:text-text-3 focus:outline-none focus:ring-1 focus:ring-accent-primary"
          />
        </form>

        <button
          onClick={() => setReloadKey(k => k + 1)}
          disabled={!url}
          className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-text-3 transition-colors hover:bg-surface-2 hover:text-text-1 cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
          title="Reload"
          aria-label="Reload canvas"
        >
          <RotateCw className="w-3.5 h-3.5" aria-hidden="true" />
        </button>
        <button
          type="button"
          onClick={() => setNarrow(n => !n)}
          className={`inline-flex h-8 w-8 items-center justify-center rounded-lg border shadow-sm transition-colors cursor-pointer ${
            narrow
              ? 'border-accent-primary/50 bg-accent-muted text-accent-text'
              : 'border-border-1 bg-surface-2 text-text-1 hover:border-accent-primary/40 hover:bg-surface-3 hover:text-accent-text'
          }`}
          title={narrow ? 'Full width' : 'Phone width'}
          aria-label={narrow ? 'Show at full width' : 'Show at phone width'}
          aria-pressed={narrow}
        >
          {narrow ? <Monitor className="w-3.5 h-3.5" aria-hidden="true" /> : <Smartphone className="w-3.5 h-3.5" aria-hidden="true" />}
        </button>
        <button
          onClick={() => openExternal(url.startsWith('/') ? window.location.origin + url : url)}
          disabled={!url}
          className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-text-3 transition-colors hover:bg-surface-2 hover:text-text-1 cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
          title="Open in browser"
          aria-label="Open in browser"
        >
          <ExternalLink className="w-3.5 h-3.5" aria-hidden="true" />
        </button>
        <button
          type="button"
          onClick={onClose}
          className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border-1 bg-surface-2 text-text-1 shadow-sm transition-colors hover:border-red-400/50 hover:bg-red-500/10 hover:text-red-300 cursor-pointer"
          title="Close canvas"
          aria-label="Close canvas"
        >
          <X className="w-4 h-4" aria-hidden="true" />
        </button>
      </div>

      {title && (
        <div className="px-3 py-1 border-b border-border-0 bg-surface-1/60 text-[10px] uppercase tracking-wider text-text-3 truncate">
          {title}
        </div>
      )}

      <div className="flex-1 min-h-0 bg-white/[0.02] flex justify-center">
        {url ? (
          <iframe
            key={`${url}-${reloadKey}`}
            src={url}
            title={title || 'Canvas preview'}
            className={`h-full bg-white ${narrow ? 'w-[390px] max-w-full border-x border-border-0' : 'w-full'}`}
            // Local files are served from this origin, so the response carries
            // its own `sandbox` CSP — the frame gets an opaque origin there and
            // cannot reach the app's cookies or API. Remote URLs are already a
            // different origin, and sandboxing them here mostly breaks dev
            // servers that expect storage and cookies.
            allow="clipboard-read; clipboard-write; fullscreen"
            referrerPolicy="no-referrer"
          />
        ) : (
          <div className="flex flex-col items-center justify-center gap-4 px-10 py-8 text-center">
            <div className="w-12 h-12 rounded-2xl bg-surface-2 flex items-center justify-center">
              <MonitorPlay className="w-6 h-6 text-text-3" aria-hidden="true" />
            </div>
            <p className="text-lg font-semibold text-text-1">Canvas is empty</p>
            <p className="max-w-xs text-sm leading-relaxed text-text-2">
              Enter a local URL or file path above, or ask an agent to show their work here.
            </p>
          </div>
        )}
      </div>

      {isLocalFile && (
        <div className="px-3 py-1 border-t border-border-0 bg-surface-1/60 text-[10px] text-text-3 truncate">
          Served from disk · sandboxed
        </div>
      )}
    </div>
  );
}
