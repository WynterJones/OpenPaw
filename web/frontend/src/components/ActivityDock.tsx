/**
 * ActivityDock
 *
 * The bottom-right stack of "what is running right now" cards — background
 * automation on top, then terminals, then chats. Each card hides itself when
 * empty so the survivors keep the same spot.
 *
 * On a phone the stack sat on top of the bottom tab bar and covered a third of
 * the screen with no way to dismiss it. There it now floats clear of the tab
 * bar and starts collapsed behind a single pill showing the total, which
 * expands on tap. The cards stay mounted while collapsed so their polling — and
 * therefore the count on the pill — keeps running.
 */

import { useCallback, useState } from 'react';
import { Activity, ChevronDown } from 'lucide-react';
import { ActiveAutomationIndicator } from './ActiveAutomationIndicator';
import { ActiveTerminalsIndicator } from './ActiveTerminalsIndicator';
import { ActiveChatsIndicator } from './ActiveChatsIndicator';
import { DockContext, type DockKey } from '../contexts/activityDock';

export function ActivityDock() {
  const [counts, setCounts] = useState<Record<DockKey, number>>({
    automation: 0,
    terminals: 0,
    chats: 0,
  });
  const [open, setOpen] = useState(false);

  const report = useCallback((key: DockKey, count: number) => {
    setCounts((prev) => (prev[key] === count ? prev : { ...prev, [key]: count }));
  }, []);

  const total = counts.automation + counts.terminals + counts.chats;
  // Nothing running means nothing to expand, whatever the toggle last said.
  const expanded = open && total > 0;

  return (
    <DockContext.Provider value={report}>
      <div className="fixed right-3 md:right-4 bottom-[calc(var(--op-bottom-nav-space)+0.5rem)] md:bottom-4 z-30 flex flex-col gap-2 items-end pointer-events-none">
        <div className={`flex-col gap-2 items-end ${expanded ? 'flex' : 'hidden'} md:flex`}>
          <ActiveAutomationIndicator />
          <ActiveTerminalsIndicator />
          <ActiveChatsIndicator />
        </div>

        {total > 0 && (
          <button
            onClick={() => setOpen((o) => !o)}
            aria-expanded={expanded}
            aria-label={expanded ? 'Hide running activity' : 'Show running activity'}
            className="md:hidden pointer-events-auto inline-flex items-center gap-1.5 h-9 pl-3 pr-2.5 rounded-full border border-border-1 bg-surface-1/95 backdrop-blur-md shadow-xl shadow-black/30 text-xs font-semibold text-text-1 cursor-pointer"
          >
            <Activity className="w-3.5 h-3.5 text-accent-primary" aria-hidden="true" />
            {total} running
            <ChevronDown
              className={`w-3.5 h-3.5 text-text-3 transition-transform ${expanded ? '' : 'rotate-180'}`}
              aria-hidden="true"
            />
          </button>
        )}
      </div>
    </DockContext.Provider>
  );
}
