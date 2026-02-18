import { useState, useCallback } from 'react';
import type { WidgetPayload } from '../lib/api';
import type { StreamingTool, CostInfo } from '../lib/chatUtils';

export function useStreamingState() {
  const [streamingText, setStreamingText] = useState('');
  const [streamingTools, setStreamingTools] = useState<StreamingTool[]>([]);
  const [streamingWidgets, setStreamingWidgets] = useState<WidgetPayload[]>([]);
  const [costInfo, setCostInfo] = useState<CostInfo | null>(null);
  const [thinkingText, setThinkingText] = useState('');

  const resetStreaming = useCallback(() => {
    setStreamingText('');
    setStreamingTools([]);
    setStreamingWidgets([]);
    setCostInfo(null);
    setThinkingText('');
  }, []);

  return {
    streamingText, setStreamingText,
    streamingTools, setStreamingTools,
    streamingWidgets, setStreamingWidgets,
    costInfo, setCostInfo,
    thinkingText, setThinkingText,
    resetStreaming,
  };
}
