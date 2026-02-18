import type { ReactNode, HTMLAttributes, KeyboardEvent } from 'react';

interface CardProps extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
  padding?: boolean;
  hover?: boolean;
}

export function Card({ children, padding = true, hover = false, className = '', onClick, ...props }: CardProps) {
  const isClickable = hover && !!onClick;

  const handleKeyDown = isClickable ? (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onClick?.(e as unknown as React.MouseEvent<HTMLDivElement>);
    }
  } : undefined;

  return (
    <div
      className={`rounded-xl border border-border-0 bg-surface-1 shadow-sm ${padding ? 'p-5' : ''} ${hover ? 'transition-all duration-150 hover:border-border-1 hover:shadow-md cursor-pointer' : ''} ${isClickable ? 'focus-visible:ring-2 focus-visible:ring-accent-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0 focus-visible:outline-none' : ''} ${className}`}
      onClick={onClick}
      onKeyDown={handleKeyDown}
      tabIndex={isClickable ? 0 : undefined}
      role={isClickable ? 'button' : undefined}
      {...props}
    >
      {children}
    </div>
  );
}
