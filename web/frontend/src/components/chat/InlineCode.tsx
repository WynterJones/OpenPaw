import {
  isValidElement,
  useEffect,
  useId,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type MouseEvent,
  type ReactNode,
} from 'react';
import { copyText } from '../../lib/clipboard';

type InlineCodeProps = ComponentPropsWithoutRef<'code'> & { node?: unknown };

function extractText(node: ReactNode): string {
  if (typeof node === 'string') return node;
  if (typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (isValidElement(node)) {
    const props = node.props as Record<string, unknown>;
    return extractText(props.children as ReactNode);
  }
  return '';
}

export function InlineCode(props: InlineCodeProps) {
  const { children, className } = props;
  const codeProps = { ...props };
  delete codeProps.node;
  delete codeProps.children;
  delete codeProps.className;

  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>('idle');
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const tooltipId = useId();
  const value = extractText(children);

  useEffect(() => () => {
    if (resetTimer.current) clearTimeout(resetTimer.current);
  }, []);

  const handleCopy = async (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    try {
      await copyText(value);
      setCopyState('copied');
    } catch {
      setCopyState('failed');
    }

    if (resetTimer.current) clearTimeout(resetTimer.current);
    resetTimer.current = setTimeout(() => setCopyState('idle'), 1500);
  };

  const status = copyState === 'copied'
    ? 'Copied'
    : copyState === 'failed'
      ? 'Copy failed'
      : 'Click to copy';

  return (
    <span className="group/inline-code relative inline-flex align-baseline">
      <button
        type="button"
        onClick={handleCopy}
        className="inline-flex p-0 border-0 bg-transparent rounded cursor-copy align-baseline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary/50"
        aria-label={`Copy ${value}`}
        aria-describedby={tooltipId}
      >
        <code
          {...codeProps}
          className={`${className || ''} transition-colors group-hover/inline-code:border-accent-primary/50 group-focus-within/inline-code:border-accent-primary/50`}
        >
          {children}
        </code>
      </button>
      <span
        id={tooltipId}
        role="tooltip"
        className={`pointer-events-none absolute bottom-full left-1/2 z-30 mb-1.5 -translate-x-1/2 whitespace-nowrap rounded-md border border-border-1 bg-surface-0 px-2 py-1 font-sans text-[10px] font-medium leading-none shadow-lg transition-opacity ${
          copyState === 'idle'
            ? 'opacity-0 group-hover/inline-code:opacity-100 group-focus-within/inline-code:opacity-100 text-text-1'
            : copyState === 'copied'
              ? 'opacity-100 text-emerald-400'
              : 'opacity-100 text-danger'
        }`}
      >
        {status}
      </span>
      <span className="sr-only" role="status" aria-live="polite">
        {copyState === 'idle' ? '' : status}
      </span>
    </span>
  );
}
