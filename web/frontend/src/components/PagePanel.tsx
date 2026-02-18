import type { ReactNode } from 'react';

interface PagePanelProps {
  children: ReactNode;
  side?: 'left' | 'right';
  width?: string;
  desktopOnly?: boolean;
  className?: string;
}

export function PagePanel({ children, side = 'left', width = 'w-72', desktopOnly = false, className = '' }: PagePanelProps) {
  const borderClass = side === 'left' ? 'border-r' : 'border-l';
  const visibilityClass = desktopOnly ? 'hidden md:flex' : 'flex';

  return (
    <aside className={`${visibilityClass} ${width} flex-col ${borderClass} border-border-0 bg-surface-1 flex-shrink-0 overflow-hidden ${className}`}>
      {children}
    </aside>
  );
}
