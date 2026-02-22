import { useWebSocket } from '../lib/useWebSocket';

const noop = () => {};

export function useConnectionStatus() {
  const { connected } = useWebSocket({ onMessage: noop, enabled: true });
  return connected;
}
