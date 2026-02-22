import { useState, useEffect, useCallback } from 'react';

export interface TocItem {
  id: string;
  text: string;
  level: number;
}

interface DocsTableOfContentsProps {
  items: TocItem[];
  scrollContainerId: string;
}

export function DocsTableOfContents({ items, scrollContainerId }: DocsTableOfContentsProps) {
  const [activeId, setActiveId] = useState('');

  useEffect(() => {
    const container = document.getElementById(scrollContainerId);
    if (!container || items.length === 0) return;

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setActiveId(entry.target.id);
          }
        }
      },
      {
        root: container,
        rootMargin: '-80px 0px -66% 0px',
        threshold: 0,
      }
    );

    const headings = items
      .map((item) => document.getElementById(item.id))
      .filter(Boolean) as HTMLElement[];

    headings.forEach((h) => observer.observe(h));
    return () => observer.disconnect();
  }, [items, scrollContainerId]);

  const handleClick = useCallback((id: string) => {
    const el = document.getElementById(id);
    el?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }, []);

  if (items.length === 0) return null;

  return (
    <nav aria-label="Table of contents" className="hidden xl:block w-56 shrink-0">
      <div className="sticky top-24 max-h-[calc(100vh-8rem)] overflow-y-auto">
        <p className="text-xs font-semibold text-text-2 uppercase tracking-wider mb-3">On this page</p>
        <ul className="space-y-1 border-l border-border-0">
          {items.map((item) => (
            <li key={item.id}>
              <button
                onClick={() => handleClick(item.id)}
                className={`block w-full text-left text-sm py-1 transition-colors cursor-pointer ${
                  item.level === 3 ? 'pl-6' : 'pl-4'
                } ${
                  activeId === item.id
                    ? 'text-accent-text border-l-2 border-accent-primary -ml-px font-medium'
                    : 'text-text-3 hover:text-text-1'
                }`}
              >
                {item.text}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </nav>
  );
}
