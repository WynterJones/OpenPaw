import { useState, useEffect, useRef, useCallback } from 'react';
import type { DashboardWidgetConfig, WSMessage } from '../lib/api';
import { useWebSocket } from '../lib/useWebSocket';

const WS_DEBOUNCE_MS = 3_000;

export function useDashboardRefresh(
  dashboardId: string | undefined,
  widgets: DashboardWidgetConfig[],
  refreshFn: (id: string) => Promise<Record<string, unknown>>,
) {
  const [widgetData, setWidgetData] = useState<Record<string, unknown>>({});
  const [loading, setLoading] = useState(false);
  const intervalsRef = useRef<ReturnType<typeof setInterval>[]>([]);
  const wsDebounceRef = useRef<number | null>(null);

  const refresh = useCallback(async () => {
    if (!dashboardId) return;
    setLoading(true);
    try {
      const data = await refreshFn(dashboardId);
      setWidgetData(data);
    } catch (e) {
      console.warn('dashboardRefresh failed:', e);
    } finally {
      setLoading(false);
    }
  }, [dashboardId, refreshFn]);

  const handleWsMessage = useCallback((msg: WSMessage) => {
    if (msg.type === 'agent_completed' || msg.type === 'audit_log_created') {
      if (wsDebounceRef.current) clearTimeout(wsDebounceRef.current);
      wsDebounceRef.current = window.setTimeout(refresh, WS_DEBOUNCE_MS);
    }
  }, [refresh]);

  useWebSocket({ onMessage: handleWsMessage, enabled: !!dashboardId });

  useEffect(() => {
    refresh();

    // Clean up old intervals
    intervalsRef.current.forEach(clearInterval);
    intervalsRef.current = [];

    // Set up per-widget refresh intervals as fallback
    const intervals = new Set<number>();
    for (const w of widgets) {
      const interval = w.dataSource?.refreshInterval;
      if (interval && interval > 0 && !intervals.has(interval)) {
        intervals.add(interval);
        const id = setInterval(refresh, interval * 1000);
        intervalsRef.current.push(id);
      }
    }

    return () => {
      intervalsRef.current.forEach(clearInterval);
      intervalsRef.current = [];
      if (wsDebounceRef.current) clearTimeout(wsDebounceRef.current);
    };
  }, [dashboardId, widgets, refresh]);

  return { widgetData, loading, refresh };
}
