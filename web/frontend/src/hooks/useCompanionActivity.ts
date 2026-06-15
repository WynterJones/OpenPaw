/**
 * useCompanionActivity
 *
 * Drives the pinned companions' live "mood" from chat activity by subscribing to
 * the shared WebSocket (the same stream Chat uses). It maps:
 *   - agent_status (routing / analyzing / thinking / spawning / compacting) and
 *     gateway_thinking  -> "thinking"
 *   - agent_stream tool_start                                       -> "toolcall"
 *   - agent_stream text_delta                                       -> "responding"
 *   - agent_completed / status "done" / stream result              -> decay to "idle"
 *
 * It also tracks which agent is active (from the routing status' agent_role_slug)
 * so a companion assigned to that agent can react while the others rest.
 *
 * Mood settles back to idle after a short quiet period.
 */

import { useEffect, useRef } from 'react';
import { useWebSocket } from '../lib/useWebSocket';
import { companionStore, type CompanionMood } from '../lib/companionStore';
import type { WSMessage } from '../lib/types';

const IDLE_DELAY_MS = 1500;

const THINKING_STATUSES = new Set([
  'routing',
  'analyzing',
  'thinking',
  'spawning',
  'compacting',
]);

export function useCompanionActivity(enabled: boolean): void {
  const idleTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (idleTimer.current) clearTimeout(idleTimer.current);
    };
  }, []);

  const scheduleIdle = () => {
    if (idleTimer.current) clearTimeout(idleTimer.current);
    idleTimer.current = setTimeout(() => {
      companionStore.setMood('idle');
      companionStore.setActiveAgent(null);
    }, IDLE_DELAY_MS);
  };

  const bump = (mood: CompanionMood) => {
    companionStore.setMood(mood);
    scheduleIdle();
  };

  useWebSocket({
    enabled,
    onMessage: (msg: WSMessage) => {
      switch (msg.type) {
        case 'gateway_thinking':
          bump('thinking');
          break;
        case 'agent_status': {
          const status = String(msg.payload?.status ?? '');
          const slug = msg.payload?.agent_role_slug as string | undefined;
          if (status === 'routing' && slug) companionStore.setActiveAgent(slug);
          if (status === 'done') {
            scheduleIdle();
          } else if (THINKING_STATUSES.has(status)) {
            bump('thinking');
          }
          break;
        }
        case 'agent_stream': {
          const evt = msg.payload?.event;
          if (!evt) break;
          if (evt.type === 'tool_start') bump('toolcall');
          else if (evt.type === 'tool_end') bump('toolcall');
          else if (evt.type === 'text_delta') bump('responding');
          else if (evt.type === 'result') scheduleIdle();
          break;
        }
        case 'agent_completed':
          scheduleIdle();
          break;
      }
    },
  });
}
