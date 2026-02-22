import { useState, type ReactNode } from 'react';
import { ChevronRight, Copy, Check } from 'lucide-react';

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
  const [copied, setCopied] = useState(false);

  const isJson = language === 'json' || (!language && isJsonContent(raw));
  const isLong = raw.split('\n').length > 6;

  if (!isJson && !isLong) {
    return (
      <pre className="rounded-lg bg-surface-2/50 border border-border-1 p-3 overflow-x-auto text-xs">
        <code>{children}</code>
      </pre>
    );
  }

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await navigator.clipboard.writeText(raw);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const preview = isJson ? getJsonPreview(raw) : `${raw.split('\n').length} lines`;

  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/30 overflow-hidden my-1">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-1.5 text-left cursor-pointer hover:bg-surface-2/50 transition-colors"
      >
        <ChevronRight className={`w-3 h-3 text-text-3 transition-transform flex-shrink-0 ${expanded ? 'rotate-90' : ''}`} />
        <span className="text-[11px] font-mono text-text-3 truncate flex-1">
          {language && <span className="text-accent-primary mr-1.5">{language}</span>}
          {preview}
        </span>
        <button
          onClick={handleCopy}
          className="p-1 rounded hover:bg-surface-3 text-text-3 hover:text-text-1 transition-colors flex-shrink-0 cursor-pointer"
          title="Copy"
        >
          {copied ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
        </button>
      </button>
      {expanded && (
        <div className="border-t border-border-1">
          <pre className="p-3 overflow-x-auto text-xs max-h-80 overflow-y-auto">
            <code>{children}</code>
          </pre>
        </div>
      )}
    </div>
  );
}
