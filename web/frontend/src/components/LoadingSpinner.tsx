import { Loader2 } from 'lucide-react';

interface LoadingSpinnerProps {
  message?: string;
  fullPage?: boolean;
}

export function LoadingSpinner({ message, fullPage }: LoadingSpinnerProps) {
  const content = (
    <div className="flex flex-col items-center justify-center gap-3" role="status" aria-live="polite">
      <Loader2 className="w-8 h-8 animate-spin text-accent-primary" aria-hidden="true" />
      <span className="sr-only">{message || 'Loading'}</span>
      {message && <p className="text-sm text-text-2">{message}</p>}
    </div>
  );

  if (fullPage) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        {content}
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center py-16">
      {content}
    </div>
  );
}
