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

// Topic subscriptions used to live here — a useWsTopic hook, subscribe/
// unsubscribe senders, and a monkey-patch of ws.onopen to replay them after a
// reconnect. Nothing ever called any of it: no component subscribed and the
// server never called BroadcastToTopic either, so every connection paid for the
// patch to replay an always-empty set. The server half (Hub.BroadcastToTopic
// and the subscribe/unsubscribe message handling in hub.go) is still there if
// filtered broadcasts are ever wanted.

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

    return () => {
      messageListeners.delete(handler);
      connectionListeners.delete(connHandler);
      maybeDisconnect();
    };
  }, [enabled]);

  return { connected };
}
