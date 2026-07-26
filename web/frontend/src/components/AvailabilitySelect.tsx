import { useEffect, useState } from 'react';
import { Globe } from 'lucide-react';
import { workspaces } from '../lib/api-helpers';
import type { Workspace } from '../lib/types';

interface AvailabilitySelectProps {
  /** null/undefined/'' all mean "All workspaces" (global). */
  value: string | null | undefined;
  /** Receives null when "All workspaces" is selected. */
  onChange: (value: string | null) => void;
  className?: string;
  label?: string;
}

/**
 * Labeled dropdown for scoping an agent/service/skill to a single workspace,
 * or leaving it available in every workspace ("All workspaces").
 */
export function AvailabilitySelect({ value, onChange, className, label = 'Availability' }: AvailabilitySelectProps) {
  const [workspaceList, setWorkspaceList] = useState<Workspace[]>([]);

  useEffect(() => {
    workspaces.list().then(setWorkspaceList).catch((e) => { console.warn('load workspaces failed:', e); });
  }, []);

  return (
    <div className={className}>
      <label className="block text-xs font-medium text-text-2 mb-1.5 flex items-center gap-1.5">
        <Globe className="w-3.5 h-3.5" />
        {label}
      </label>
      <select
        value={value || ''}
        onChange={(e) => onChange(e.target.value || null)}
        className="w-full px-3 py-2 rounded-lg border border-border-1 bg-surface-0 text-sm text-text-1 hover:border-border-0 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors cursor-pointer"
      >
        <option value="">All workspaces</option>
        {workspaceList.map((w) => (
          <option key={w.id} value={w.id}>{w.name}</option>
        ))}
      </select>
    </div>
  );
}
