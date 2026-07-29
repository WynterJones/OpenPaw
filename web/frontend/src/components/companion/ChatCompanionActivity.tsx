/**
 * Loads workspace companions and keeps their live mood in sync with chat.
 *
 * This component is intentionally non-visual. Companions now live in the
 * assistant avatar slot instead of floating over the app as draggable widgets.
 */

import { useEffect, useMemo } from 'react';
import { useCompanionActivity } from '../../hooks/useCompanionActivity';
import { companionStore, useCompanionStore } from '../../lib/companionStore';

export function ChatCompanionActivity() {
  const { characters } = useCompanionStore();
  const hasChatAvatar = useMemo(
    () => characters.some((character) => character.pinned),
    [characters],
  );

  useEffect(() => {
    companionStore.load().catch(() => {});
  }, []);

  useCompanionActivity(hasChatAvatar);
  return null;
}
