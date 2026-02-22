import { useState, useRef, useCallback } from 'react';
import { Monitor } from 'lucide-react';
import { browserApi } from '../lib/api';
import { useWebSocket, useWsTopic } from '../lib/useWebSocket';
import { useToast } from './Toast';

interface BrowserScreenshotEvent {
  session_id: string;
  image: string;
  url?: string;
  title?: string;
  width?: number;
  height?: number;
}

interface BrowserViewerProps {
  sessionId: string;
  humanControl?: boolean;
}

const VIEWPORT_WIDTH = 1280;
const VIEWPORT_HEIGHT = 900;

export function BrowserViewer({ sessionId, humanControl = false }: BrowserViewerProps) {
  const { toast } = useToast();
  const [screenshot, setScreenshot] = useState<string | null>(null);
  const [pageUrl, setPageUrl] = useState<string>('');
  const [pageTitle, setPageTitle] = useState<string>('');
  const imgRef = useRef<HTMLImageElement>(null);

  // Subscribe to this browser session's screenshot topic
  useWsTopic(`browser:${sessionId}`);

  useWebSocket({
    onMessage: (msg) => {
      if (msg.type === 'browser_screenshot') {
        const payload = msg.payload as unknown as BrowserScreenshotEvent;
        if (payload.session_id === sessionId) {
          setScreenshot(payload.image);
          if (payload.url) setPageUrl(payload.url);
          if (payload.title) setPageTitle(payload.title);
        }
      }
    },
  });

  const handleClick = useCallback(
    async (e: React.MouseEvent<HTMLImageElement>) => {
      if (!humanControl || !imgRef.current) return;

      const rect = imgRef.current.getBoundingClientRect();
      const relX = e.clientX - rect.left;
      const relY = e.clientY - rect.top;

      const scaleX = VIEWPORT_WIDTH / rect.width;
      const scaleY = VIEWPORT_HEIGHT / rect.height;

      const scaledX = Math.round(relX * scaleX);
      const scaledY = Math.round(relY * scaleY);

      try {
        await browserApi.executeAction(sessionId, {
          action: 'click',
          x: scaledX,
          y: scaledY,
        });
      } catch (err) {
        toast('error', err instanceof Error ? err.message : 'Click action failed');
      }
    },
    [humanControl, sessionId, toast],
  );

  return (
    <div className="rounded-xl border border-border-0 bg-surface-1 overflow-hidden">
      {(pageUrl || pageTitle) && (
        <div className="flex items-center gap-2 px-3 py-2 border-b border-border-0 bg-surface-2/50">
          <div className="flex gap-1.5 flex-shrink-0">
            <span className="w-2.5 h-2.5 rounded-full bg-red-500/60" />
            <span className="w-2.5 h-2.5 rounded-full bg-amber-500/60" />
            <span className="w-2.5 h-2.5 rounded-full bg-emerald-500/60" />
          </div>
          {pageTitle && (
            <span className="text-xs text-text-2 truncate flex-shrink-0 max-w-[160px]">
              {pageTitle}
            </span>
          )}
          {pageUrl && (
            <span className="text-xs text-text-3 font-mono truncate flex-1 min-w-0">
              {pageUrl}
            </span>
          )}
        </div>
      )}

      <div className="relative bg-surface-2 w-full">
        {screenshot ? (
          <img
            ref={imgRef}
            src={`data:image/jpeg;base64,${screenshot}`}
            alt="Browser screenshot"
            className={`w-full h-auto block select-none ${humanControl ? 'cursor-crosshair' : 'cursor-default'}`}
            onClick={humanControl ? handleClick : undefined}
            draggable={false}
          />
        ) : (
          <div className="flex flex-col items-center justify-center gap-3 text-text-3 py-24">
            <Monitor className="w-10 h-10 opacity-30" />
            <p className="text-sm">No screenshot available</p>
            <p className="text-xs text-text-3/60">Screenshots stream automatically when the session is active</p>
          </div>
        )}
      </div>
    </div>
  );
}
