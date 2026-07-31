import { useCallback, useEffect, useRef } from "react";
import { terminalManager } from "../../lib/terminal-manager";
import { activatePathInsertionTarget, clearPathInsertionTarget } from "../../lib/path-insertion";

interface TerminalViewProps {
  sessionId: string;
  isActive: boolean;
  onExit?: (sessionId: string) => void;
}

export function TerminalView({
  sessionId,
  isActive,
  onExit,
}: TerminalViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const activateInsertion = useCallback(() => {
    activatePathInsertionTarget({
      id: `terminal:${sessionId}`,
      label: 'terminal',
      insert: path => terminalManager.insertPath(sessionId, path),
    });
  }, [sessionId]);

  // Acquire terminal and attach/detach DOM
  useEffect(() => {
    if (!containerRef.current) return;

    terminalManager.acquire(sessionId);
    terminalManager.attach(sessionId, containerRef.current);

    return () => {
      clearPathInsertionTarget(`terminal:${sessionId}`);
      terminalManager.detach(sessionId);
    };
  }, [sessionId]);

  // Set onExit callback
  useEffect(() => {
    terminalManager.setOnExit(sessionId, onExit ?? null);
    return () => {
      terminalManager.setOnExit(sessionId, null);
    };
  }, [sessionId, onExit]);

  // Handle active state — fit, repaint and focus.
  //
  // The repaint is what makes a tab you come back to show its contents. An
  // inactive tab is hidden rather than unmounted, so its terminal is alive and
  // still taking output, but a hidden canvas can lose its WebGL context and
  // xterm only paints rows that change — so a quiet shell reappears blank and
  // only fills in once you type.
  useEffect(() => {
    if (!isActive) return;
    activateInsertion();

    const timer = setTimeout(() => {
      terminalManager.fit(sessionId);
      terminalManager.repaint(sessionId, true);
      terminalManager.focus(sessionId);
    }, 50);

    return () => clearTimeout(timer);
  }, [activateInsertion, isActive, sessionId]);

  // Reconnect and refocus when browser tab becomes visible again
  useEffect(() => {
    if (!isActive) return;

    const onVisible = () => {
      if (document.visibilityState === 'visible') {
        terminalManager.ensureConnected(sessionId);
        setTimeout(() => {
          terminalManager.fit(sessionId);
          // A backgrounded browser tab is where the GPU is most likely to have
          // taken the context back.
          terminalManager.repaint(sessionId, true);
          terminalManager.focus(sessionId);
        }, 100);
      }
    };

    document.addEventListener('visibilitychange', onVisible);
    return () => document.removeEventListener('visibilitychange', onVisible);
  }, [isActive, sessionId]);

  // Click anywhere in the terminal area to refocus
  const handleClick = useCallback(() => {
    if (isActive) {
      activateInsertion();
      terminalManager.focus(sessionId);
    }
  }, [activateInsertion, isActive, sessionId]);

  return (
    <div
      ref={containerRef}
      data-openpaw-hotkeys="ignore"
      className="openpaw-terminal h-full w-full p-1 md:p-4"
      style={{ minHeight: 100 }}
      onClick={handleClick}
    />
  );
}
