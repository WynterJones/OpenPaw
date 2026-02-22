import type { AgentRole } from '../../lib/api';

export function MentionBadge({ name, role }: { name: string; role?: AgentRole }) {
  return (
    <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-md bg-accent-muted text-accent-text text-[0.85em] font-medium align-baseline">
      {role?.avatar_path && (
        <img src={role.avatar_path} alt={name} className="w-3.5 h-3.5 rounded-full" />
      )}
      @{name}
    </span>
  );
}
