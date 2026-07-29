import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from 'react';
import { ChevronRight, Copy, Check, CircleAlert } from 'lucide-react';
import { copyText } from '../../lib/clipboard';

interface CollapsibleCodeProps {
  language?: string;
  children: ReactNode;
  raw: string;
}

function isJsonContent(raw: string): boolean {
  const trimmed = raw.trim();
  if ((trimmed.startsWith('{') && trimmed.endsWith('}')) ||
      (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
    try {
      JSON.parse(trimmed);
      return true;
    } catch {
      return false;
    }
  }
  return false;
}

function getJsonPreview(raw: string): string {
  const trimmed = raw.trim();
  try {
    const parsed = JSON.parse(trimmed);
    if (Array.isArray(parsed)) {
      return `Array [${parsed.length} items]`;
    }
    const keys = Object.keys(parsed);
    if (keys.length <= 3) {
      return `{ ${keys.join(', ')} }`;
    }
    return `{ ${keys.slice(0, 3).join(', ')} ... +${keys.length - 3} }`;
  } catch {
    return 'JSON';
  }
}

export function CollapsibleCode({ language, children, raw }: CollapsibleCodeProps) {
  const [expanded, setExpanded] = useState(false);
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>('idle');
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const isJson = language === 'json' || (!language && isJsonContent(raw));
  const isLong = raw.split('\n').length > 6;

  useEffect(() => () => {
    if (resetTimer.current) clearTimeout(resetTimer.current);
  }, []);

  if (!isJson && !isLong) {
    return (
      <pre className="op-code-raw rounded-lg bg-surface-2/50 border border-border-1 p-3 overflow-x-auto text-xs">
        <code>{children}</code>
      </pre>
    );
  }

  const handleCopy = async (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    try {
      await copyText(raw);
      setCopyState('copied');
    } catch {
      setCopyState('failed');
    }
    if (resetTimer.current) clearTimeout(resetTimer.current);
    resetTimer.current = setTimeout(() => setCopyState('idle'), 1500);
  };

  const preview = isJson ? getJsonPreview(raw) : `${raw.split('\n').length} lines`;
  const copyLabel = copyState === 'copied'
    ? 'Copied to clipboard'
    : copyState === 'failed'
      ? 'Copy failed'
      : 'Copy code';

  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/30 overflow-hidden my-1">
      <div className="flex items-center hover:bg-surface-2/50 transition-colors">
        <button
          type="button"
          onClick={() => setExpanded(!expanded)}
          className="min-w-0 flex flex-1 items-center gap-2 px-3 py-1.5 text-left cursor-pointer"
          aria-expanded={expanded}
        >
          <ChevronRight className={`w-3 h-3 text-text-3 transition-transform flex-shrink-0 ${expanded ? 'rotate-90' : ''}`} />
          <span className="text-[11px] font-mono text-text-3 truncate flex-1">
            {language && <span className="text-accent-primary mr-1.5">{language}</span>}
            {preview}
          </span>
        </button>
        <button
          type="button"
          onClick={handleCopy}
          className="mr-2 inline-flex items-center gap-1 rounded px-1.5 py-1 text-[10px] text-text-3 hover:bg-surface-3 hover:text-text-1 transition-colors flex-shrink-0 cursor-pointer"
          title={copyLabel}
          aria-label={copyLabel}
        >
          {copyState === 'copied' ? (
            <Check className="w-3 h-3 text-emerald-400" />
          ) : copyState === 'failed' ? (
            <CircleAlert className="w-3 h-3 text-danger" />
          ) : (
            <Copy className="w-3 h-3" />
          )}
          {copyState !== 'idle' && <span>{copyState === 'copied' ? 'Copied' : 'Failed'}</span>}
        </button>
        <span className="sr-only" role="status" aria-live="polite">
          {copyState === 'idle' ? '' : copyLabel}
        </span>
      </div>
      {expanded && (
        <div className="border-t border-border-1">
          <pre className="op-code-raw p-3 overflow-x-auto text-xs max-h-80 overflow-y-auto">
            <code>{children}</code>
          </pre>
        </div>
      )}
    </div>
  );
}
