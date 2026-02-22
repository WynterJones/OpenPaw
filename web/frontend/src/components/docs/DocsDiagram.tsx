import type { ReactNode } from 'react';

interface DiagramProps {
  title?: string;
  children: ReactNode;
  className?: string;
}

export function Diagram({ title, children, className = '' }: DiagramProps) {
  return (
    <div className={`my-6 rounded-xl border border-border-0 bg-surface-1/50 overflow-hidden ${className}`}>
      {title && (
        <div className="px-4 py-2 border-b border-border-0 bg-surface-2/50">
          <p className="text-xs font-semibold text-text-2 uppercase tracking-wider">{title}</p>
        </div>
      )}
      <div className="p-4 sm:p-6">{children}</div>
    </div>
  );
}

type BoxVariant = 'primary' | 'accent' | 'muted' | 'outline';

const boxVariants: Record<BoxVariant, string> = {
  primary: 'bg-accent-primary/15 border-accent-primary/30 text-accent-text',
  accent: 'bg-accent-muted border-accent-primary/20 text-accent-text',
  muted: 'bg-surface-2 border-border-1 text-text-1',
  outline: 'bg-transparent border-border-1 text-text-2 border-dashed',
};

interface DiagramBoxProps {
  children: ReactNode;
  variant?: BoxVariant;
  className?: string;
}

export function DiagramBox({ children, variant = 'muted', className = '' }: DiagramBoxProps) {
  return (
    <div className={`px-4 py-2.5 rounded-lg border text-sm font-medium text-center ${boxVariants[variant]} ${className}`}>
      {children}
    </div>
  );
}

type ArrowDirection = 'down' | 'right' | 'left';

interface DiagramArrowProps {
  direction?: ArrowDirection;
  label?: string;
  className?: string;
}

export function DiagramArrow({ direction = 'down', label, className = '' }: DiagramArrowProps) {
  const isVertical = direction === 'down';

  return (
    <div className={`flex ${isVertical ? 'flex-col' : ''} items-center gap-0.5 ${className}`}>
      {label && (
        <span className="text-[10px] font-medium text-text-3 uppercase tracking-wider">{label}</span>
      )}
      <svg
        className="text-border-1"
        width={isVertical ? 16 : 32}
        height={isVertical ? 24 : 16}
        viewBox={isVertical ? '0 0 16 24' : '0 0 32 16'}
        fill="none"
      >
        {isVertical ? (
          <>
            <line x1="8" y1="0" x2="8" y2="20" stroke="currentColor" strokeWidth="1.5" />
            <path d="M4 16 L8 22 L12 16" stroke="currentColor" strokeWidth="1.5" fill="none" />
          </>
        ) : direction === 'right' ? (
          <>
            <line x1="0" y1="8" x2="28" y2="8" stroke="currentColor" strokeWidth="1.5" />
            <path d="M24 4 L30 8 L24 12" stroke="currentColor" strokeWidth="1.5" fill="none" />
          </>
        ) : (
          <>
            <line x1="4" y1="8" x2="32" y2="8" stroke="currentColor" strokeWidth="1.5" />
            <path d="M8 4 L2 8 L8 12" stroke="currentColor" strokeWidth="1.5" fill="none" />
          </>
        )}
      </svg>
    </div>
  );
}

interface DiagramRowProps {
  children: ReactNode;
  className?: string;
}

export function DiagramRow({ children, className = '' }: DiagramRowProps) {
  return (
    <div className={`flex items-center justify-center gap-3 flex-wrap ${className}`}>
      {children}
    </div>
  );
}

interface DiagramStackProps {
  children: ReactNode;
  className?: string;
}

export function DiagramStack({ children, className = '' }: DiagramStackProps) {
  return (
    <div className={`flex flex-col items-center gap-2 ${className}`}>
      {children}
    </div>
  );
}
