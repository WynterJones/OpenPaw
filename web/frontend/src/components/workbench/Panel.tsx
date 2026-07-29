import { useCallback, useState } from 'react';
import { useWorkbench, type PanelNode } from './WorkbenchProvider';
import { TabBar } from './TabBar';
import { TerminalView } from './TerminalView';
import { NewTerminalScreen } from './NewTerminalScreen';

interface PanelProps {
  node: PanelNode;
}

export function Panel({ node }: PanelProps) {
  const { activeSessionId, closeSession } = useWorkbench();

  // "+" opens this screen instead of spawning a terminal outright. The tab bar
  // stays reachable above it, so clicking a tab backs out of the request.
  const [requestingTerminal, setRequestingTerminal] = useState(false);
  const requestTerminal = useCallback(() => setRequestingTerminal(true), []);
  const dismissTerminalRequest = useCallback(() => setRequestingTerminal(false), []);

  const tabs = node.tabs || [];
  const activeTab = node.activeTab;

  return (
    <div className="flex flex-col h-full w-full min-h-0 min-w-0">
      <TabBar
        node={node}
        onRequestNewTerminal={requestTerminal}
        onDismissNewTerminal={dismissTerminalRequest}
      />

      {/* Terminal content area */}
      <div className="flex-1 relative min-h-0 min-w-0">
        {tabs.map((sessionId) => {
          const isActiveInPanel = sessionId === activeTab;
          const isGloballyActive =
            isActiveInPanel && sessionId === activeSessionId;

          return (
            <div
              key={sessionId}
              className="absolute inset-0"
              style={{
                visibility: isActiveInPanel ? 'visible' : 'hidden',
              }}
            >
              <TerminalView
                sessionId={sessionId}
                isActive={isGloballyActive}
                onExit={closeSession}
              />
            </div>
          );
        })}

        {/* Empty state when no tabs */}
        {tabs.length === 0 && !requestingTerminal && (
          <div className="flex items-center justify-center h-full text-text-3 text-sm">
            No terminals open
          </div>
        )}

        {/* Pending "+" request. It stays on top for drop hit-testing while the
            translucent surface keeps the workspace background visible. */}
        {requestingTerminal && (
          <div className="absolute inset-0 z-10 bg-surface-0/75">
            <NewTerminalScreen panelId={node.id} onOpened={dismissTerminalRequest} />
          </div>
        )}
      </div>
    </div>
  );
}
