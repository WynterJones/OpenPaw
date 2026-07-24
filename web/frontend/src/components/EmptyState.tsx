import type { ReactNode } from 'react';

interface EmptyStateProps {
  icon: ReactNode;
  title: string;
  description: string;
  action?: ReactNode;
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div
      className="w-full h-full flex flex-col items-center justify-center text-center px-4 py-12 min-h-[18rem]"
      role="status"
    >
      <div className="w-16 h-16 rounded-2xl bg-surface-2 flex items-center justify-center mb-4 text-text-2">
        {icon}
      </div>
      <h3 className="text-base font-semibold text-text-1 mb-1">{title}</h3>
      <p className="text-xs text-text-2 max-w-sm mb-6">{description}</p>
      {action}
    </div>
  );
}
