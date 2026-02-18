import { useState, useCallback } from 'react';
import { notificationsApi, type AppNotification, type WSMessage } from '../lib/api';
import { useWebSocket } from '../lib/useWebSocket';
import { requestNotificationPermission, showBrowserNotification, playNotificationSound } from '../lib/pushNotifications';

let permissionRequested = false;

export function useNotifications() {
  const [notifications, setNotifications] = useState<AppNotification[]>(() => {
    if (!permissionRequested) {
      permissionRequested = true;
      requestNotificationPermission();
    }
    return [];
  });
  const [unreadCount, setUnreadCount] = useState(0);
  const [initialized, setInitialized] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const [list, count] = await Promise.all([
        notificationsApi.list(),
        notificationsApi.unreadCount(),
      ]);
      setNotifications(list);
      setUnreadCount(count.count);
    } catch (e) {
      console.warn('refreshNotifications failed:', e);
    }
  }, []);

  if (!initialized) {
    setInitialized(true);
    refresh();
  }

  const handleWsMessage = useCallback((msg: WSMessage) => {
    if (msg.type === 'notification_created') {
      const n = msg.payload as unknown as AppNotification;
      setNotifications(prev => [n, ...prev]);
      setUnreadCount(prev => prev + 1);

      playNotificationSound();
      showBrowserNotification(n.title, { body: n.body });
    }

    if (msg.type === 'notification_read' || msg.type === 'notifications_cleared') {
      refresh();
    }
  }, [refresh]);

  useWebSocket({ onMessage: handleWsMessage });

  const markRead = useCallback(async (id: string) => {
    try {
      await notificationsApi.markRead(id);
      setNotifications(prev => prev.map(n => n.id === id ? { ...n, read: true } : n));
      setUnreadCount(prev => Math.max(0, prev - 1));
    } catch (e) {
      console.warn('markNotificationRead failed:', e);
    }
  }, []);

  const markAllRead = useCallback(async () => {
    try {
      await notificationsApi.markAllRead();
      setNotifications(prev => prev.map(n => ({ ...n, read: true })));
      setUnreadCount(0);
    } catch (e) {
      console.warn('markAllNotificationsRead failed:', e);
    }
  }, []);

  const dismiss = useCallback(async (id: string) => {
    try {
      await notificationsApi.dismiss(id);
      setNotifications(prev => {
        const n = prev.find(x => x.id === id);
        if (n && !n.read) setUnreadCount(c => Math.max(0, c - 1));
        return prev.filter(x => x.id !== id);
      });
    } catch (e) {
      console.warn('dismissNotification failed:', e);
    }
  }, []);

  return { notifications, unreadCount, markRead, markAllRead, dismiss, refresh };
}
