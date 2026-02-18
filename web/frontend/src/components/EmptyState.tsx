import type { ReactNode } from 'react';

interface EmptyStateProps {
  icon: ReactNode;
  title: string;
  description: string;
  action?: ReactNode;
  compact?: boolean;
}

export function EmptyState({ icon, title, description, action, compact = false }: EmptyStateProps) {
  return (
    <div className={`flex flex-col items-center justify-center ${compact ? 'py-8' : 'py-16'} px-4 text-center`} role="status">
      <div className="w-16 h-16 rounded-2xl bg-surface-2 flex items-center justify-center mb-4 text-text-2">
        {icon}
      </div>
      <h3 className="text-lg font-semibold text-text-1 mb-1">{title}</h3>
      <p className="text-sm text-text-2 max-w-sm mb-6">{description}</p>
      {action}
    </div>
  );
}
