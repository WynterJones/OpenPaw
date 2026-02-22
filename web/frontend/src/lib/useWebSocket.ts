import { useEffect, useRef, useState } from 'react';
import type { WSMessage } from './api';

interface UseWebSocketOptions {
  onMessage: (msg: WSMessage) => void;
  enabled?: boolean;
}

type MessageListener = (msg: WSMessage) => void;
type ConnectionListener = (connected: boolean) => void;

let sharedWs: WebSocket | null = null;
let sharedConnected = false;
let reconnectTimer: number | null = null;
const messageListeners = new Set<MessageListener>();
const connectionListeners = new Set<ConnectionListener>();

function ensureConnection() {
  if (sharedWs && (sharedWs.readyState === WebSocket.OPEN || sharedWs.readyState === WebSocket.CONNECTING)) {
    return;
  }

  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${proto}//${window.location.host}/api/v1/ws`;

  const ws = new WebSocket(url);
  sharedWs = ws;

  ws.onopen = () => {
    if (sharedWs !== ws) return; // superseded by a newer connection
    sharedConnected = true;
    connectionListeners.forEach(fn => fn(true));
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  };

  ws.onmessage = (ev) => {
    if (sharedWs !== ws) {
      // Ghost connection — close it and stop delivering messages
      ws.close();
      return;
    }
    try {
      const msg: WSMessage = JSON.parse(ev.data);
      messageListeners.forEach(fn => fn(msg));
    } catch (e) {
      console.warn('WebSocket: failed to parse message:', e);
    }
  };

  ws.onclose = () => {
    // Only update shared state if this is still the active connection
    if (sharedWs !== ws) return;
    sharedConnected = false;
    sharedWs = null;
    connectionListeners.forEach(fn => fn(false));
    if (messageListeners.size > 0) {
      reconnectTimer = window.setTimeout(ensureConnection, 3000);
    }
  };

  ws.onerror = () => {
    ws.close();
  };
}

function maybeDisconnect() {
  if (messageListeners.size === 0) {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (sharedWs) {
      sharedWs.close();
      sharedWs = null;
    }
  }
}

/** Send a raw JSON message to the shared WebSocket (e.g. subscribe/unsubscribe). */
function sendWsMessage(msg: Record<string, unknown>) {
  if (sharedWs && sharedWs.readyState === WebSocket.OPEN) {
    sharedWs.send(JSON.stringify(msg));
  }
}

/** Subscribe to a topic for filtered server broadcasts. */
function subscribeTopic(topic: string) {
  sendWsMessage({ type: 'subscribe', topic });
}

/** Unsubscribe from a topic. */
function unsubscribeTopic(topic: string) {
  sendWsMessage({ type: 'unsubscribe', topic });
}

// Track pending subscriptions to replay after reconnect
const activeTopics = new Set<string>();
// Patch ensureConnection to replay topic subscriptions after reconnect
const _origOnOpen = Symbol();
function patchReplayTopics() {
  if (!sharedWs) return;
  const ws = sharedWs;
  if ((ws as unknown as Record<symbol, unknown>)[_origOnOpen]) return;
  const prevOnOpen = ws.onopen;
  (ws as unknown as Record<symbol, unknown>)[_origOnOpen] = true;
  ws.onopen = (ev) => {
    if (prevOnOpen) (prevOnOpen as (ev: Event) => void).call(ws, ev);
    // Replay active topic subscriptions
    for (const topic of activeTopics) {
      sendWsMessage({ type: 'subscribe', topic });
    }
  };
}

export function useWebSocket({ onMessage, enabled = true }: UseWebSocketOptions) {
  const [connected, setConnected] = useState(sharedConnected);
  const onMessageRef = useRef(onMessage);

  useEffect(() => {
    onMessageRef.current = onMessage;
  }, [onMessage]);

  useEffect(() => {
    if (!enabled) return;

    const handler: MessageListener = (msg) => onMessageRef.current(msg);
    const connHandler: ConnectionListener = (c) => setConnected(c);

    messageListeners.add(handler);
    connectionListeners.add(connHandler);
    ensureConnection();
    patchReplayTopics();

    return () => {
      messageListeners.delete(handler);
      connectionListeners.delete(connHandler);
      maybeDisconnect();
    };
  }, [enabled]);

  return { connected };
}

/** Hook to subscribe to a WebSocket topic while the component is mounted. */
export function useWsTopic(topic: string | null) {
  useEffect(() => {
    if (!topic) return;
    activeTopics.add(topic);
    subscribeTopic(topic);
    return () => {
      activeTopics.delete(topic);
      unsubscribeTopic(topic);
    };
  }, [topic]);
}
