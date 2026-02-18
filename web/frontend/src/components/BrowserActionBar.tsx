import { useState, useRef } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  RefreshCw,
  Camera,
  Keyboard,
  MousePointer,
  HandMetal,
  Globe,
} from 'lucide-react';
import { browserApi } from '../lib/api';
import { useToast } from './Toast';

interface BrowserActionBarProps {
  sessionId: string;
  currentUrl?: string;
  humanControl: boolean;
  onTakeControl: () => void;
  onReleaseControl: () => void;
}

export function BrowserActionBar({
  sessionId,
  currentUrl = '',
  humanControl,
  onTakeControl,
  onReleaseControl,
}: BrowserActionBarProps) {
  const { toast } = useToast();
  const [urlInput, setUrlInput] = useState('');
  const [typeInput, setTypeInput] = useState('');
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const typeInputRef = useRef<HTMLInputElement>(null);

  const doAction = async (action: string, payload: Record<string, unknown> = {}) => {
    setLoadingAction(action);
    try {
      await browserApi.executeAction(sessionId, { action, ...payload });
    } catch (err) {
      toast('error', err instanceof Error ? err.message : `Action "${action}" failed`);
    } finally {
      setLoadingAction(null);
    }
  };

  const handleNavigate = async (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') return;
    const url = urlInput.trim();
    if (!url) return;
    await doAction('navigate', { value: url });
    setUrlInput('');
  };

  const handleType = async (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') return;
    const text = typeInput.trim();
    if (!text) return;
    await doAction('type', { value: text });
    setTypeInput('');
  };

  const handleScreenshot = async () => {
    setLoadingAction('screenshot');
    try {
      await browserApi.executeAction(sessionId, { action: 'screenshot' });
      toast('success', 'Screenshot captured');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Screenshot failed');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleTakeControl = async () => {
    setLoadingAction('control');
    try {
      await browserApi.takeControl(sessionId);
      onTakeControl();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to take control');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleReleaseControl = async () => {
    setLoadingAction('release');
    try {
      await browserApi.releaseControl(sessionId);
      onReleaseControl();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to release control');
    } finally {
      setLoadingAction(null);
    }
  };

  const isLoading = (action: string) => loadingAction === action;

  return (
    <div className="rounded-xl border border-border-0 bg-surface-1 divide-y divide-border-0">
      {/* Navigation row */}
      <div className="flex items-center gap-2 px-3 py-2.5">
        <button
          onClick={() => doAction('back')}
          disabled={isLoading('back')}
          title="Back"
          aria-label="Back"
          className="p-1.5 rounded-lg text-text-2 hover:text-text-0 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed flex-shrink-0"
        >
          <ArrowLeft className="w-4 h-4" />
        </button>
        <button
          onClick={() => doAction('forward')}
          disabled={isLoading('forward')}
          title="Forward"
          aria-label="Forward"
          className="p-1.5 rounded-lg text-text-2 hover:text-text-0 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed flex-shrink-0"
        >
          <ArrowRight className="w-4 h-4" />
        </button>
        <button
          onClick={() => doAction('refresh')}
          disabled={isLoading('refresh')}
          title="Refresh"
          aria-label="Refresh"
          className="p-1.5 rounded-lg text-text-2 hover:text-text-0 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed flex-shrink-0"
        >
          <RefreshCw className={`w-4 h-4 ${isLoading('refresh') ? 'animate-spin' : ''}`} />
        </button>

        <div className="flex-1 flex items-center gap-2 bg-surface-2 border border-border-0 rounded-lg px-2.5 py-1.5 min-w-0">
          <Globe className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
          <input
            type="url"
            value={urlInput}
            onChange={(e) => setUrlInput(e.target.value)}
            onKeyDown={handleNavigate}
            placeholder={currentUrl || 'Enter URL and press Enter…'}
            aria-label="Navigate to URL"
            className="flex-1 min-w-0 text-sm bg-transparent text-text-0 placeholder:text-text-3 focus-visible:outline-none"
          />
        </div>
      </div>

      {/* Type row */}
      <div className="flex items-center gap-2 px-3 py-2.5">
        <Keyboard className="w-4 h-4 text-text-3 flex-shrink-0" />
        <input
          ref={typeInputRef}
          type="text"
          value={typeInput}
          onChange={(e) => setTypeInput(e.target.value)}
          onKeyDown={handleType}
          placeholder="Type text and press Enter to send…"
          aria-label="Type text to browser"
          className="flex-1 min-w-0 text-sm bg-surface-2 border border-border-0 rounded-lg px-2.5 py-1.5 text-text-0 placeholder:text-text-3 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent-primary"
        />
      </div>

      {/* Actions row */}
      <div className="flex items-center gap-2 px-3 py-2.5 flex-wrap">
        <button
          onClick={handleScreenshot}
          disabled={isLoading('screenshot')}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-text-1 bg-surface-2 border border-border-0 hover:border-border-1 hover:bg-surface-3 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <Camera className="w-3.5 h-3.5" />
          Screenshot
        </button>

        <div className="flex-1" />

        {humanControl ? (
          <button
            onClick={handleReleaseControl}
            disabled={isLoading('release')}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-amber-400 bg-amber-500/10 border border-amber-500/20 hover:bg-amber-500/20 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <HandMetal className="w-3.5 h-3.5" />
            Release Control
          </button>
        ) : (
          <button
            onClick={handleTakeControl}
            disabled={isLoading('control')}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-accent-primary bg-accent-primary/10 border border-accent-primary/20 hover:bg-accent-primary/20 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <MousePointer className="w-3.5 h-3.5" />
            Take Control
          </button>
        )}
      </div>
    </div>
  );
}
