type Status = 'ready' | 'building' | 'disabled' | 'error' | 'success' | 'running' | 'pending' | 'active' | 'stopped' | 'starting' | 'compiling';

const statusConfig: Record<Status, { bg: string; text: string; dot: string; label: string }> = {
  ready: { bg: 'bg-emerald-500/10', text: 'text-emerald-400', dot: 'bg-emerald-400', label: 'Ready' },
  active: { bg: 'bg-emerald-500/10', text: 'text-emerald-400', dot: 'bg-emerald-400', label: 'Active' },
  building: { bg: 'bg-amber-500/10', text: 'text-amber-400', dot: 'bg-amber-400', label: 'Building' },
  compiling: { bg: 'bg-blue-500/10', text: 'text-blue-400', dot: 'bg-blue-400', label: 'Compiling' },
  starting: { bg: 'bg-amber-500/10', text: 'text-amber-400', dot: 'bg-amber-400', label: 'Starting' },
  disabled: { bg: 'bg-gray-500/10', text: 'text-gray-400', dot: 'bg-gray-400', label: 'Disabled' },
  stopped: { bg: 'bg-gray-500/10', text: 'text-gray-400', dot: 'bg-gray-400', label: 'Stopped' },
  error: { bg: 'bg-red-500/10', text: 'text-red-400', dot: 'bg-red-400', label: 'Error' },
  success: { bg: 'bg-emerald-500/10', text: 'text-emerald-400', dot: 'bg-emerald-400', label: 'Success' },
  running: { bg: 'bg-blue-500/10', text: 'text-blue-400', dot: 'bg-blue-400', label: 'Running' },
  pending: { bg: 'bg-amber-500/10', text: 'text-amber-400', dot: 'bg-amber-400', label: 'Pending' },
};

interface StatusBadgeProps {
  status: string;
  label?: string;
}

export function StatusBadge({ status, label }: StatusBadgeProps) {
  const config = statusConfig[status as Status] || statusConfig.disabled;
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium ${config.bg} ${config.text}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${config.dot} ${status === 'building' || status === 'running' ? 'animate-pulse' : ''}`} />
      {label || config.label}
    </span>
  );
}
