import { useState, useEffect, useCallback, useRef } from 'react';
import { api, type WSMessage } from '../lib/api';
import { useWebSocket } from '../lib/useWebSocket';

const DEBOUNCE_MS = 2_000;

interface BalanceResponse {
  usage: number;
  usage_monthly: number;
  limit: number | null;
  limit_remaining: number | null;
  is_free_tier: boolean;
  label: string;
  rate_limit?: { requests: number; interval: string };
  total_credits?: number;
  total_usage?: number;
}

export interface BalanceData {
  usage: number | null;
  usageMonthly: number | null;
  limit: number | null;
  limitRemaining: number | null;
  isFreeTier: boolean;
  label: string | null;
  rateLimit: { requests: number; interval: string } | null;
  totalCredits: number | null;
  totalUsage: number | null;
}

const EMPTY: BalanceData = {
  usage: null,
  usageMonthly: null,
  limit: null,
  limitRemaining: null,
  isFreeTier: false,
  label: null,
  rateLimit: null,
  totalCredits: null,
  totalUsage: null,
};

export function useOpenRouterBalance(): BalanceData {
  const [data, setData] = useState<BalanceData>(EMPTY);
  const debounceRef = useRef<number | null>(null);

  const fetchBalance = useCallback(async () => {
    if (document.visibilityState === 'hidden') return;
    try {
      const res = await api.get<BalanceResponse>('/system/balance');
      setData({
        usage: res.usage ?? null,
        usageMonthly: res.usage_monthly ?? null,
        limit: res.limit ?? null,
        limitRemaining: res.limit_remaining ?? null,
        isFreeTier: res.is_free_tier ?? false,
        label: res.label || null,
        rateLimit: res.rate_limit ?? null,
        totalCredits: res.total_credits ?? null,
        totalUsage: res.total_usage ?? null,
      });
    } catch {
      setData(EMPTY);
    }
  }, []);

  const debouncedFetch = useCallback(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(fetchBalance, DEBOUNCE_MS);
  }, [fetchBalance]);

  const handleMessage = useCallback((msg: WSMessage) => {
    if (msg.type === 'agent_completed') {
      debouncedFetch();
    }
  }, [debouncedFetch]);

  useWebSocket({ onMessage: handleMessage, enabled: true });

  useEffect(() => {
    // Defer initial fetch to avoid synchronous setState in effect body
    const initTimer = window.setTimeout(fetchBalance, 0);

    function handleVisibilityChange() {
      if (document.visibilityState === 'visible') {
        fetchBalance();
      }
    }

    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => {
      clearTimeout(initTimer);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [fetchBalance]);

  return data;
}
