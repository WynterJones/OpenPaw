import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { Loader2 } from 'lucide-react';

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost';
type Size = 'sm' | 'md' | 'lg';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
  icon?: ReactNode;
  children: ReactNode;
}

const sizeStyles: Record<Size, string> = {
  sm: 'px-3 py-1.5 text-xs',
  md: 'px-4 py-2 text-sm',
  lg: 'px-6 py-2.5 text-base',
};

const variantConfig: Record<Variant, { className: string; style?: React.CSSProperties }> = {
  primary: {
    className: 'btn-primary',
    style: {
      color: 'var(--op-accent-btn-text, #ffffff)',
      background: 'linear-gradient(180deg, var(--op-accent) 0%, var(--op-accent-hover) 100%)',
      borderWidth: '1px',
      borderColor: 'color-mix(in srgb, var(--op-accent-text) 30%, transparent)',
      boxShadow: 'inset 0 1px 0 0 rgba(255,255,255,0.15), 0 1px 3px 0 color-mix(in srgb, var(--op-accent) 30%, transparent), 0 0 12px -3px color-mix(in srgb, var(--op-accent) 20%, transparent)',
    },
  },
  secondary: {
    className: 'btn-secondary text-text-1 border border-border-1',
    style: {
      background: 'linear-gradient(180deg, #1f1f1f 0%, #141414 100%)',
      boxShadow: 'inset 0 1px 0 0 rgba(255,255,255,0.05), 0 1px 2px 0 rgba(0,0,0,0.4)',
    },
  },
  danger: {
    className: 'btn-danger text-white border border-red-500/30',
    style: {
      background: 'linear-gradient(180deg, #dc2626 0%, #b91c1c 100%)',
      boxShadow: 'inset 0 1px 0 0 rgba(255,255,255,0.15), 0 1px 3px 0 rgba(220,38,38,0.3), 0 0 12px -3px rgba(220,38,38,0.2)',
    },
  },
  ghost: {
    className: 'text-text-2 hover:text-text-1 hover:bg-surface-2 border border-transparent',
  },
};

export function Button({ variant = 'primary', size = 'md', loading, icon, children, disabled, className = '', ...props }: ButtonProps) {
  const base = 'inline-flex items-center justify-center gap-2 rounded-lg font-bold uppercase tracking-wide transition-all duration-150 focus-visible:ring-2 focus-visible:ring-accent-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0 focus-visible:outline-none disabled:opacity-40 disabled:cursor-not-allowed disabled:pointer-events-none cursor-pointer';
  const cfg = variantConfig[variant];

  return (
    <button
      className={`${base} ${cfg.className} ${sizeStyles[size]} ${className}`}
      style={cfg.style}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : icon}
      {children}
    </button>
  );
}
