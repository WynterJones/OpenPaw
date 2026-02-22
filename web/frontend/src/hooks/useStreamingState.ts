import { useState, useCallback } from 'react';
import type { WidgetPayload } from '../lib/api';
import type { StreamingTool, CostInfo } from '../lib/chatUtils';
import { useBufferedStreaming } from './useBufferedStreaming';

export function useStreamingState() {
  const {
    displayText: streamingText,
    appendText: appendStreamingText,
    setTextDirect: setStreamingText,
    reset: resetStreamingText,
  } = useBufferedStreaming();
  const [streamingTools, setStreamingTools] = useState<StreamingTool[]>([]);
  const [streamingWidgets, setStreamingWidgets] = useState<WidgetPayload[]>([]);
  const [costInfo, setCostInfo] = useState<CostInfo | null>(null);
  const [thinkingText, setThinkingText] = useState('');

  const resetStreaming = useCallback(() => {
    resetStreamingText();
    setStreamingTools([]);
    setStreamingWidgets([]);
    setCostInfo(null);
    setThinkingText('');
  }, [resetStreamingText]);

  return {
    streamingText, setStreamingText, appendStreamingText,
    streamingTools, setStreamingTools,
    streamingWidgets, setStreamingWidgets,
    costInfo, setCostInfo,
    thinkingText, setThinkingText,
    resetStreaming,
  };
}
