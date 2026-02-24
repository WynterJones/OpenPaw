import { useEffect, useRef } from "react";
import { terminalManager } from "../../lib/terminal-manager";

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

  // Acquire terminal and attach/detach DOM
  useEffect(() => {
    if (!containerRef.current) return;

    terminalManager.acquire(sessionId);
    terminalManager.attach(sessionId, containerRef.current);

    return () => {
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

  // Handle active state — fit and focus
  useEffect(() => {
    if (!isActive) return;

    const timer = setTimeout(() => {
      terminalManager.fit(sessionId);
      terminalManager.focus(sessionId);
    }, 50);

    return () => clearTimeout(timer);
  }, [isActive, sessionId]);

  return (
    <div
      ref={containerRef}
      className="h-full w-full p-1 md:p-4"
      style={{ minHeight: 100 }}
    />
  );
}
