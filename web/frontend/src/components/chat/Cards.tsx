import { useState } from 'react';
import { Loader2, Check, Hammer, XCircle, CheckCircle2, Server } from 'lucide-react';
import { api, type ConfirmationCard as ConfirmationCardType, type ToolSummaryCard as ToolSummaryCardType } from '../../lib/api';
import { Button } from '../Button';
import { useToast } from '../Toast';

export function ConfirmationCardUI({ card, threadId, onUpdate }: { card: ConfirmationCardType; threadId: string; onUpdate: () => void }) {
  const [loading, setLoading] = useState(false);
  const { toast } = useToast();

  const handleConfirm = async () => {
    setLoading(true);
    try {
      await api.post(`/chat/threads/${threadId}/confirm`, {
        work_order_id: card.work_order_id,
        message_id: card.message_id,
      });
      onUpdate();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to confirm');
    } finally {
      setLoading(false);
    }
  };

  const handleReject = async () => {
    setLoading(true);
    try {
      await api.post(`/chat/threads/${threadId}/reject`, {
        work_order_id: card.work_order_id,
        message_id: card.message_id,
      });
      onUpdate();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to cancel');
    } finally {
      setLoading(false);
    }
  };

  const isPending = card.status === 'pending';

  return (
    <div className={`rounded-xl overflow-hidden max-w-md ${
      isPending
        ? 'border-2 border-accent-primary bg-surface-1 shadow-lg shadow-accent-primary/10 ring-1 ring-accent-primary/20'
        : 'border border-border-1 bg-surface-1'
    }`}>
      <div className={`flex items-center gap-2 px-4 py-3 border-b ${
        isPending ? 'bg-accent-primary/10 border-accent-primary/20' : 'bg-surface-2/50 border-border-1'
      }`}>
        <Hammer className={`w-5 h-5 ${isPending ? 'text-accent-primary' : 'text-text-3'}`} />
        <span className={`text-xs font-bold uppercase tracking-wide ${isPending ? 'text-accent-primary' : 'text-text-2'}`}>
          {isPending ? 'Approval Required' : card.action_label}
        </span>
      </div>
      <div className="px-4 py-4 space-y-1.5">
        <p className="text-sm font-semibold text-text-0">{card.title}</p>
        {card.description && (
          <p className="text-xs text-text-2 leading-relaxed">{card.description}</p>
        )}
      </div>
      <div className="px-4 py-3 border-t border-border-1">
        {isPending && (
          <div className="flex items-center gap-3">
            <Button onClick={handleConfirm} disabled={loading} icon={loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Check className="w-4 h-4" />}>
              Approve & Build
            </Button>
            <Button variant="ghost" onClick={handleReject} disabled={loading}>
              Cancel
            </Button>
          </div>
        )}
        {card.status === 'confirmed' && (
          <div className="flex items-center gap-2 text-emerald-400">
            <CheckCircle2 className="w-4 h-4" />
            <span className="text-sm font-medium">Approved</span>
          </div>
        )}
        {card.status === 'rejected' && (
          <div className="flex items-center gap-2 text-text-3">
            <XCircle className="w-4 h-4" />
            <span className="text-sm font-medium">Cancelled</span>
          </div>
        )}
      </div>
    </div>
  );
}

export function ToolSummaryCardUI({ card }: { card: ToolSummaryCardType }) {
  return (
    <div className="rounded-xl overflow-hidden max-w-md border border-border-1 bg-surface-1">
      <div className="flex items-center gap-2 px-4 py-3 border-b bg-emerald-500/10 border-emerald-500/20">
        <Server className="w-4 h-4 text-emerald-400" />
        <span className="text-xs font-bold uppercase tracking-wide text-emerald-400">Tool Active</span>
        <span className="ml-auto text-[10px] font-mono text-text-3">:{card.port}</span>
      </div>
      <div className="px-4 py-3">
        <p className="text-sm font-semibold text-text-0">{card.tool_name}</p>
      </div>
      {card.endpoints.length > 0 && (
        <div className="px-4 pb-3 space-y-1.5">
          {card.endpoints.map((ep, i) => (
            <div key={i} className="flex items-start gap-2 text-xs">
              <span className="shrink-0 font-mono font-bold px-1.5 py-0.5 rounded bg-accent-primary/15 text-accent-primary">{ep.method}</span>
              <code className="font-mono text-text-1 pt-0.5">{ep.path}</code>
              {ep.description && <span className="text-text-3 pt-0.5">— {ep.description}</span>}
            </div>
          ))}
        </div>
      )}
      <div className="px-4 py-2.5 border-t border-border-1 flex items-center gap-2">
        <span className={`w-2 h-2 rounded-full ${card.healthy ? 'bg-emerald-400' : 'bg-amber-400'}`} />
        <span className="text-[11px] text-text-3">{card.healthy ? 'Healthy' : 'Starting...'}</span>
      </div>
    </div>
  );
}
