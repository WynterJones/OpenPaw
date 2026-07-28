/**
 * ActivityDock
 *
 * The floating stack of "what is running right now" cards — background
 * automation on top, then terminals, then chats. Each card hides itself when
 * empty so the survivors keep the same spot.
 *
 * Two things are adjustable, both persisted:
 *
 * Collapsed/expanded. The stack starts behind a single pill showing the total
 * and expands on tap. This used to be phone-only, driven by `md:` classes, so on
 * a desktop the three cards were always up and there was no way to get rid of
 * them. The toggle is now real state at every width. The cards stay mounted
 * while collapsed — hidden with a class, never unmounted — so their polling, and
 * therefore the count on the pill, keeps running.
 *
 * Corner. Bottom-right by default, bottom-left on request, because the right
 * edge is where chat composers and canvas panels tend to be. On a desktop the
 * left position clears the sidebar, whose width depends on whether it is
 * collapsed; the dock is `fixed`, so it has to account for that itself rather
 * than being laid out around it.
 */

import { useCallback, useEffect, useState } from 'react';
import { Activity, ArrowLeftToLine, ArrowRightToLine, ChevronDown } from 'lucide-react';
import { ActiveAutomationIndicator } from './ActiveAutomationIndicator';
import { ActiveTerminalsIndicator } from './ActiveTerminalsIndicator';
import { ActiveChatsIndicator } from './ActiveChatsIndicator';
import { DockContext, type DockKey } from '../contexts/activityDock';
import { useViewToggles } from '../contexts/viewToggles';

type DockSide = 'left' | 'right';

const SIDE_KEY = 'openpaw_dock_side';
const OPEN_KEY = 'openpaw_dock_open';

/**
 * Sidebar width (w-16 collapsed, w-56 open) plus a 1rem gutter. Written as
 * literal classes so Tailwind's scanner sees them.
 */
const LEFT_WITH_SIDEBAR = 'md:left-[15rem]';
const LEFT_WITH_COLLAPSED_SIDEBAR = 'md:left-[5rem]';

function loadSide(): DockSide {
  return localStorage.getItem(SIDE_KEY) === 'left' ? 'left' : 'right';
}

/**
 * Defaults to the behaviour each width had before the toggle existed: cards up
 * on a desktop, collapsed on a phone where they covered a third of the screen.
 * Only the initial value — once the user picks, their choice is what persists.
 */
function loadOpen(): boolean {
  const stored = localStorage.getItem(OPEN_KEY);
  if (stored === '1') return true;
  if (stored === '0') return false;
  return window.innerWidth >= 768;
}

export function ActivityDock() {
  const { sidebar } = useViewToggles();
  const [counts, setCounts] = useState<Record<DockKey, number>>({
    automation: 0,
    terminals: 0,
    chats: 0,
  });
  const [open, setOpen] = useState(loadOpen);
  const [side, setSide] = useState<DockSide>(loadSide);

  useEffect(() => { localStorage.setItem(OPEN_KEY, open ? '1' : '0'); }, [open]);
  useEffect(() => { localStorage.setItem(SIDE_KEY, side); }, [side]);

  const report = useCallback((key: DockKey, count: number) => {
    setCounts((prev) => (prev[key] === count ? prev : { ...prev, [key]: count }));
  }, []);

  const total = counts.automation + counts.terminals + counts.chats;
  // Nothing running means nothing to expand, whatever the toggle last said.
  const expanded = open && total > 0;

  // The sidebar animates its width over 200ms, so the left offset follows it
  // rather than jumping ahead. Swapping corners still jumps: the property being
  // animated changes from `right` to `left`, and neither animates from `auto`.
  const position =
    side === 'right'
      ? 'right-3 md:right-4 items-end'
      : `left-3 items-start ${sidebar ? LEFT_WITH_SIDEBAR : LEFT_WITH_COLLAPSED_SIDEBAR}`;

  return (
    <DockContext.Provider value={report}>
      <div
        className={`fixed z-30 flex flex-col gap-2 pointer-events-none bottom-[calc(var(--op-bottom-nav-space)+0.5rem)] md:bottom-4 transition-[left,right] duration-200 ${position}`}
      >
        <div className={`flex-col gap-2 ${side === 'right' ? 'items-end' : 'items-start'} ${expanded ? 'flex' : 'hidden'}`}>
          <ActiveAutomationIndicator />
          <ActiveTerminalsIndicator />
          <ActiveChatsIndicator />
        </div>

        {total > 0 && (
          <div className="pointer-events-auto inline-flex items-center h-9 rounded-full border border-border-1 bg-surface-1/95 backdrop-blur-md shadow-xl shadow-black/30 overflow-hidden">
            <button
              onClick={() => setOpen((o) => !o)}
              aria-expanded={expanded}
              aria-label={expanded ? 'Hide running activity' : 'Show running activity'}
              className="inline-flex items-center gap-1.5 h-full pl-3 pr-2.5 text-xs font-semibold text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
            >
              <Activity className="w-3.5 h-3.5 text-accent-primary" aria-hidden="true" />
              {total} running
              <ChevronDown
                className={`w-3.5 h-3.5 text-text-3 transition-transform ${expanded ? '' : 'rotate-180'}`}
                aria-hidden="true"
              />
            </button>

            <span className="w-px h-5 bg-border-1 flex-shrink-0" aria-hidden="true" />

            <button
              onClick={() => setSide((s) => (s === 'right' ? 'left' : 'right'))}
              aria-label={side === 'right' ? 'Move to bottom-left corner' : 'Move to bottom-right corner'}
              title={side === 'right' ? 'Move to bottom-left' : 'Move to bottom-right'}
              className="inline-flex items-center justify-center h-full px-2.5 text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
            >
              {side === 'right' ? (
                <ArrowLeftToLine className="w-3.5 h-3.5" aria-hidden="true" />
              ) : (
                <ArrowRightToLine className="w-3.5 h-3.5" aria-hidden="true" />
              )}
            </button>
          </div>
        )}
      </div>
    </DockContext.Provider>
  );
}
