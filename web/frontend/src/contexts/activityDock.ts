/**
 * Shared state for the ActivityDock — the bottom-right stack of "what is
 * running right now" cards.
 *
 * The dock needs a running total before any card has decided whether to render
 * itself, so that on a phone it can show one collapsed pill instead of three
 * stacked panels. Each card reports its own row count here.
 *
 * Split out from the component file so both can hot-reload cleanly.
 */

import { createContext, useContext, useEffect } from 'react';

export type DockKey = 'automation' | 'terminals' | 'chats';

export const DockContext = createContext<(key: DockKey, count: number) => void>(() => {});

/** Reports how many rows a card is showing. Safe to call before an early return. */
export function useDockCount(key: DockKey, count: number) {
  const report = useContext(DockContext);
  useEffect(() => {
    report(key, count);
    return () => report(key, 0);
  }, [report, key, count]);
}
