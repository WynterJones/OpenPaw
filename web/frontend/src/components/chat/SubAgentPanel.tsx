import { useState } from 'react';
import { ChevronDown, Loader2, Check, AlertCircle, DollarSign, Users } from 'lucide-react';
import type { SubAgentTask, AgentRole } from '../../lib/types';

export function SubAgentPanel({ tasks, roles }: { tasks: SubAgentTask[]; roles: AgentRole[] }) {
  const [expanded, setExpanded] = useState(true);
  const [expandedTasks, setExpandedTasks] = useState<Set<string>>(new Set());

  if (tasks.length === 0) return null;

  const completed = tasks.filter(t => t.status === 'completed').length;
  const failed = tasks.filter(t => t.status === 'failed').length;
  const running = tasks.filter(t => t.status === 'started').length;
  const allDone = running === 0;

  const toggleTask = (id: string) => {
    setExpandedTasks(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const getRole = (slug: string) => roles.find(r => r.slug === slug);

  return (
    <div className="mt-2 mb-1 rounded-xl border border-border-1 bg-surface-1/60 overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left cursor-pointer hover:bg-surface-2/50 transition-colors"
      >
        <Users className="w-3.5 h-3.5 text-accent-primary flex-shrink-0" />
        <span className="text-xs font-medium text-text-1 flex-1">
          {allDone ? 'Delegated tasks' : 'Delegating tasks'}
          <span className="ml-1.5 text-text-3">
            {completed}/{tasks.length} done
            {failed > 0 && <span className="text-red-400 ml-1">({failed} failed)</span>}
          </span>
        </span>
        {!allDone && <Loader2 className="w-3 h-3 text-accent-primary animate-spin flex-shrink-0" />}
        <ChevronDown className={`w-3.5 h-3.5 text-text-3 transition-transform ${expanded ? 'rotate-180' : ''}`} />
      </button>

      {expanded && (
        <div className="border-t border-border-0">
          {tasks.map(task => {
            const role = getRole(task.agent_slug);
            const isExpanded = expandedTasks.has(task.subagent_id);
            const hasContent = task.streaming_text || task.result_preview;

            return (
              <div key={task.subagent_id} className="border-b border-border-0 last:border-b-0">
                <button
                  onClick={() => hasContent && toggleTask(task.subagent_id)}
                  className={`w-full flex items-center gap-2.5 px-3 py-2 text-left transition-colors ${hasContent ? 'cursor-pointer hover:bg-surface-2/30' : 'cursor-default'}`}
                >
                  <div className="w-5 h-5 rounded-full overflow-hidden flex-shrink-0 ring-1 ring-border-1">
                    {role ? (
                      <img src={role.avatar_path} alt={role.name} className="w-5 h-5 rounded-full object-cover" />
                    ) : (
                      <div className="w-5 h-5 rounded-full bg-surface-3" />
                    )}
                  </div>

                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-1.5">
                      <span className="text-xs font-medium text-text-1 truncate">
                        {task.agent_name}
                      </span>
                      {task.cost_usd != null && task.cost_usd > 0 && (
                        <span className="flex items-center gap-0.5 text-[10px] text-text-3">
                          <DollarSign className="w-2.5 h-2.5" />
                          {task.cost_usd.toFixed(4)}
                        </span>
                      )}
                    </div>
                    <p className="text-[11px] text-text-3 truncate">{task.task_summary}</p>
                  </div>

                  <div className="flex-shrink-0">
                    {task.status === 'started' && (
                      <Loader2 className="w-3.5 h-3.5 text-accent-primary animate-spin" />
                    )}
                    {task.status === 'completed' && (
                      <Check className="w-3.5 h-3.5 text-emerald-400" />
                    )}
                    {task.status === 'failed' && (
                      <AlertCircle className="w-3.5 h-3.5 text-red-400" />
                    )}
                  </div>

                  {hasContent && (
                    <ChevronDown className={`w-3 h-3 text-text-3 flex-shrink-0 transition-transform ${isExpanded ? 'rotate-180' : ''}`} />
                  )}
                </button>

                {isExpanded && hasContent && (
                  <div className="px-3 pb-2.5">
                    <div className="rounded-lg bg-surface-2/50 border border-border-0 px-3 py-2 max-h-48 overflow-y-auto">
                      <pre className="text-xs text-text-2 whitespace-pre-wrap font-mono leading-relaxed">
                        {task.streaming_text || task.result_preview}
                      </pre>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
