import { useWorkbench, type PanelNode } from './WorkbenchProvider';
import { TabBar } from './TabBar';
import { TerminalView } from './TerminalView';

interface PanelProps {
  node: PanelNode;
}

export function Panel({ node }: PanelProps) {
  const { activeSessionId, closeSession } = useWorkbench();

  const tabs = node.tabs || [];
  const activeTab = node.activeTab;

  return (
    <div className="flex flex-col h-full w-full min-h-0 min-w-0">
      <TabBar node={node} />

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
        {tabs.length === 0 && (
          <div className="flex items-center justify-center h-full text-text-3 text-sm">
            No terminals open
          </div>
        )}
      </div>
    </div>
  );
}
