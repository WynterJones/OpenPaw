/**
 * CanvasPanel
 *
 * The preview pane beside the chat: a local dev server, a built page, an HTML
 * file on disk. Working on something local means change → look → say what's
 * next, and that loop breaks the moment it involves alt-tabbing to a browser.
 *
 * Either side can drive it. The user types a URL (or a path) here; an agent
 * calls canvas_show and the URL arrives over the websocket.
 *
 * Local files can't be loaded as file:// from an http page, so they are served
 * back through /api/v1/canvas/fs/<abs path> — a path typed in here is rewritten
 * to that, and shown as the plain path so the address stays readable.
 */

import { useRef, useState } from 'react';
import {
  RotateCw,
  ExternalLink,
  X,
  Monitor,
  Smartphone,
  MonitorPlay,
} from 'lucide-react';
import { openExternal } from '../../lib/openExternal';

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

interface CanvasPanelProps {
  url: string;
  title?: string;
  onUrlChange: (url: string) => void;
  onClose: () => void;
}

export function CanvasPanel({ url, title, onUrlChange, onClose }: CanvasPanelProps) {
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
