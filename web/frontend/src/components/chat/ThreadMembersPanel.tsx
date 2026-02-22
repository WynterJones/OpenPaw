import { Bot, X } from 'lucide-react';
import type { ThreadMember } from '../../lib/api';

export function ThreadMembersPanel({ members, activeSlug, onRemove }: {
  members: ThreadMember[];
  activeSlug: string | null;
  onRemove: (slug: string) => void;
}) {
  if (members.length === 0) {
    return (
      <div className="p-3 text-center">
        <p className="text-xs text-text-3">No agents in this chat yet</p>
      </div>
    );
  }

  return (
    <div className="p-3 space-y-1">
      <p className="text-[11px] font-semibold text-text-3 uppercase tracking-wide px-1 mb-2">In this chat</p>
      {members.map(m => (
        <div key={m.agent_role_slug} className="group flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-surface-2 transition-colors">
          <div className="relative flex-shrink-0">
            {m.avatar_path ? (
              <img src={m.avatar_path} alt={m.name} className="w-7 h-7 rounded-full object-cover ring-1 ring-border-1" />
            ) : (
              <div className="w-7 h-7 rounded-full bg-surface-3 flex items-center justify-center ring-1 ring-border-1">
                <Bot className="w-3.5 h-3.5 text-text-3" />
              </div>
            )}
            {activeSlug === m.agent_role_slug && (
              <div className="absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-emerald-400 ring-2 ring-surface-1" />
            )}
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-xs font-medium text-text-1 truncate">{m.name}</p>
          </div>
          <button
            onClick={() => onRemove(m.agent_role_slug)}
            className="p-1 rounded text-text-3 hover:text-red-400 hover:bg-red-500/10 opacity-0 group-hover:opacity-100 transition-all cursor-pointer"
            title="Remove from chat"
          >
            <X className="w-3 h-3" />
          </button>
        </div>
      ))}
    </div>
  );
}
