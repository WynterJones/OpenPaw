import { useState, useRef, useEffect, useCallback } from 'react';
import { ChevronDown, Check } from 'lucide-react';

interface FilterDropdownProps {
  value: string;
  onChange: (value: string) => void;
  options: string[];
  allLabel?: string;
  placeholder?: string;
}

export function FilterDropdown({ value, onChange, options, allLabel = 'All', placeholder }: FilterDropdownProps) {
  const [open, setOpen] = useState(false);
  const [focusIndex, setFocusIndex] = useState(-1);
  const ref = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const allOptions = ['', ...options];
  const displayLabel = value || allLabel;

  const close = useCallback(() => {
    setOpen(false);
    setFocusIndex(-1);
  }, []);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) close();
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close();
    };
    document.addEventListener('mousedown', handleClick);
    document.addEventListener('keydown', handleKey);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKey);
    };
  }, [open, close]);

  useEffect(() => {
    if (!open || focusIndex < 0) return;
    const items = listRef.current?.querySelectorAll('[role="option"]');
    if (items?.[focusIndex]) {
      (items[focusIndex] as HTMLElement).scrollIntoView({ block: 'nearest' });
    }
  }, [focusIndex, open]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        setOpen(true);
        setFocusIndex(allOptions.indexOf(value));
        return;
      }
      return;
    }

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setFocusIndex(i => Math.min(i + 1, allOptions.length - 1));
        break;
      case 'ArrowUp':
        e.preventDefault();
        setFocusIndex(i => Math.max(i - 1, 0));
        break;
      case 'Enter':
      case ' ':
        e.preventDefault();
        if (focusIndex >= 0) {
          onChange(allOptions[focusIndex]);
          close();
        }
        break;
      case 'Home':
        e.preventDefault();
        setFocusIndex(0);
        break;
      case 'End':
        e.preventDefault();
        setFocusIndex(allOptions.length - 1);
        break;
    }
  };

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => { setOpen(!open); if (!open) setFocusIndex(allOptions.indexOf(value)); }}
        onKeyDown={handleKeyDown}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={placeholder || 'Filter by category'}
        className={`
          flex items-center gap-2 px-3.5 py-2 rounded-lg text-sm font-medium
          border transition-all duration-150 cursor-pointer whitespace-nowrap
          ${open
            ? 'bg-surface-2 border-accent-primary text-text-0 shadow-[0_0_0_1px_var(--op-accent-primary)]'
            : value
              ? 'bg-accent-primary/10 border-accent-primary/30 text-accent-primary hover:bg-accent-primary/15'
              : 'bg-surface-1 border-border-0 text-text-1 hover:bg-surface-2 hover:border-border-1'
          }
        `}
      >
        {displayLabel}
        <ChevronDown className={`w-3.5 h-3.5 transition-transform duration-200 ${open ? 'rotate-180' : ''}`} />
      </button>

      {open && (
        <div
          ref={listRef}
          role="listbox"
          aria-activedescendant={focusIndex >= 0 ? `filter-opt-${focusIndex}` : undefined}
          className="
            absolute right-0 top-full mt-1.5 min-w-[180px] max-h-[280px] overflow-y-auto
            rounded-xl border border-border-0 bg-surface-1 shadow-xl z-50
            py-1.5
          "
        >
          {allOptions.map((opt, i) => {
            const label = opt || allLabel;
            const isSelected = opt === value;
            const isFocused = i === focusIndex;

            return (
              <button
                key={opt}
                id={`filter-opt-${i}`}
                role="option"
                aria-selected={isSelected}
                onClick={() => { onChange(opt); close(); }}
                onMouseEnter={() => setFocusIndex(i)}
                className={`
                  w-full flex items-center gap-2.5 px-3 py-2 text-sm text-left
                  transition-colors duration-75 cursor-pointer
                  ${isFocused ? 'bg-surface-2' : ''}
                  ${isSelected ? 'text-accent-primary font-medium' : 'text-text-1'}
                `}
              >
                <span className="w-4 h-4 flex-shrink-0 flex items-center justify-center">
                  {isSelected && <Check className="w-3.5 h-3.5" />}
                </span>
                {label}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
