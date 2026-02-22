import { CheckCircle2, XCircle, AlertCircle, Clock } from 'lucide-react';
import type { WidgetProps } from './WidgetRegistry';

const statusConfig: Record<string, { icon: typeof CheckCircle2; color: string; bg: string }> = {
  ok:      { icon: CheckCircle2, color: 'text-emerald-400', bg: 'bg-emerald-500/10' },
  success: { icon: CheckCircle2, color: 'text-emerald-400', bg: 'bg-emerald-500/10' },
  error:   { icon: XCircle,      color: 'text-red-400',     bg: 'bg-red-500/10' },
  warning: { icon: AlertCircle,  color: 'text-amber-400',   bg: 'bg-amber-500/10' },
  pending: { icon: Clock,        color: 'text-blue-400',    bg: 'bg-blue-500/10' },
};

export function StatusCardWidget({ data }: WidgetProps) {
  const label = (data.label as string) || '';
  const status = (data.status as string) || 'ok';
  const message = (data.message as string) || '';

  const config = statusConfig[status] || statusConfig.ok;
  const Icon = config.icon;

  return (
    <div className={`rounded-lg border border-border-1 ${config.bg} px-4 py-3 inline-flex items-center gap-3`}>
      <Icon className={`w-5 h-5 ${config.color} flex-shrink-0`} />
      <div>
        <p className="text-sm font-medium text-text-1">{label}</p>
        {message && <p className="text-xs text-text-2 mt-0.5">{message}</p>}
      </div>
    </div>
  );
}
