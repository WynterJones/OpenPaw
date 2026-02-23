import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  useRef,
  type ReactNode,
} from 'react';
import { terminalApi, type TerminalSession, type Workbench } from '../../lib/api';
import { terminalManager } from '../../lib/terminal-manager';
import { useDesign } from '../../contexts/DesignContext';

export interface PanelNode {
  id: string;
  type: 'leaf' | 'split';
  direction?: 'horizontal' | 'vertical';
  children?: PanelNode[];
  tabs?: string[]; // session IDs
  activeTab?: string;
  sizes?: number[]; // flex ratios for children
}

interface WorkbenchContextType {
  sessions: TerminalSession[];
  workbenches: Workbench[];
  activeWorkbenchId: string | null;
  rootPanel: PanelNode | null;
  activeSessionId: string | null;
  createSession: (panelId?: string) => Promise<void>;
  closeSession: (sessionId: string) => Promise<void>;
  splitPanel: (panelId: string, direction: 'horizontal' | 'vertical') => Promise<void>;
  activateTab: (panelId: string, sessionId: string) => void;
  updateSession: (sessionId: string, data: { title?: string; color?: string }) => Promise<void>;
  updatePanelSizes: (panelId: string, sizes: number[]) => void;
  createWorkbench: (name: string) => Promise<void>;
  renameWorkbench: (id: string, name: string) => Promise<void>;
  updateWorkbenchColor: (id: string, color: string) => Promise<void>;
  deleteWorkbench: (id: string) => Promise<void>;
  switchWorkbench: (id: string) => void;
  busySessions: Set<string>;
  loading: boolean;
}

const WorkbenchContext = createContext<WorkbenchContextType | null>(null);

const LAYOUT_KEY = 'openpaw:workbench:layout';

let panelIdCounter = 0;
function nextPanelId(): string {
  return `panel-${++panelIdCounter}-${Date.now()}`;
}

// ── Helpers for immutable panel tree traversal ──

function findFirstLeaf(node: PanelNode): PanelNode | null {
  if (node.type === 'leaf') return node;
  if (node.children) {
    for (const child of node.children) {
      const found = findFirstLeaf(child);
      if (found) return found;
    }
  }
  return null;
}

function clonePanel(node: PanelNode): PanelNode {
  const clone: PanelNode = { ...node };
  if (node.tabs) clone.tabs = [...node.tabs];
  if (node.sizes) clone.sizes = [...node.sizes];
  if (node.children) clone.children = node.children.map(clonePanel);
  return clone;
}

/** Deep update: returns a new tree with the target panel replaced */
function updatePanel(
  root: PanelNode,
  targetId: string,
  updater: (panel: PanelNode) => PanelNode,
): PanelNode {
  if (root.id === targetId) return updater(clonePanel(root));
  if (!root.children) return root;
  const newChildren = root.children.map((child) =>
    updatePanel(child, targetId, updater),
  );
  if (newChildren.every((c, i) => c === root.children![i])) return root;
  return { ...root, children: newChildren };
}

/** Remove a session from every leaf in the tree, pruning empty leaves/splits */
function removeSessionFromTree(
  node: PanelNode,
  sessionId: string,
): PanelNode | null {
  if (node.type === 'leaf') {
    const tabs = (node.tabs || []).filter((t) => t !== sessionId);
    if (tabs.length === 0) return null;
    const activeTab =
      node.activeTab === sessionId ? tabs[0] : node.activeTab;
    return { ...node, tabs, activeTab };
  }

  if (!node.children) return null;

  const newChildren: PanelNode[] = [];
  const newSizes: number[] = [];
  for (let i = 0; i < node.children.length; i++) {
    const result = removeSessionFromTree(node.children[i], sessionId);
    if (result) {
      newChildren.push(result);
      newSizes.push(node.sizes?.[i] ?? 1);
    }
  }

  if (newChildren.length === 0) return null;
  if (newChildren.length === 1) return newChildren[0];
  return { ...node, children: newChildren, sizes: newSizes };
}

/** Collect all session IDs referenced in the panel tree */
function collectSessionIds(node: PanelNode): Set<string> {
  const ids = new Set<string>();
  if (node.type === 'leaf' && node.tabs) {
    for (const id of node.tabs) ids.add(id);
  }
  if (node.children) {
    for (const child of node.children) {
      for (const id of collectSessionIds(child)) ids.add(id);
    }
  }
  return ids;
}

/** Reconcile layout with actual sessions: remove stale refs, add orphans */
function reconcileLayout(
  layout: PanelNode,
  sessions: TerminalSession[],
): PanelNode | null {
  const sessionIds = new Set(sessions.map((s) => s.id));
  const layoutIds = collectSessionIds(layout);

  // Remove sessions from layout that no longer exist
  let result: PanelNode | null = clonePanel(layout);
  for (const id of layoutIds) {
    if (!sessionIds.has(id) && result) {
      result = removeSessionFromTree(result, id);
    }
  }

  // Find orphan sessions not in any panel
  const remainingIds = result ? collectSessionIds(result) : new Set<string>();
  const orphans = sessions.filter((s) => !remainingIds.has(s.id));

  if (orphans.length > 0 && result) {
    // Add orphans to first leaf
    const firstLeaf = findFirstLeaf(result);
    if (firstLeaf) {
      result = updatePanel(result, firstLeaf.id, (panel) => ({
        ...panel,
        tabs: [...(panel.tabs || []), ...orphans.map((s) => s.id)],
        activeTab: panel.activeTab || orphans[0].id,
      }));
    }
  } else if (orphans.length > 0 && !result) {
    // No layout at all, create fresh leaf
    result = {
      id: nextPanelId(),
      type: 'leaf',
      tabs: orphans.map((s) => s.id),
      activeTab: orphans[0].id,
    };
  }

  return result;
}

// Layout is stored per-workbench
function layoutKey(workbenchId: string): string {
  return `${LAYOUT_KEY}:${workbenchId}`;
}

export function WorkbenchProvider({ children }: { children: ReactNode }) {
  const [sessions, setSessions] = useState<TerminalSession[]>([]);
  const [workbenches, setWorkbenches] = useState<Workbench[]>([]);
  const [activeWorkbenchId, setActiveWorkbenchId] = useState<string | null>(null);
  const [rootPanel, setRootPanel] = useState<PanelNode | null>(null);
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busySessions, setBusySessions] = useState<Set<string>>(new Set());
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Sync terminal themes when design config changes ──
  const { config } = useDesign();
  useEffect(() => {
    terminalManager.refreshThemes();
  }, [config]);

  // ── Track busy terminals via activity ──
  useEffect(() => {
    const handler = (sessionId: string, busy: boolean) => {
      setBusySessions(prev => {
        const next = new Set(prev);
        if (busy) next.add(sessionId);
        else next.delete(sessionId);
        if (next.size === prev.size && [...next].every(id => prev.has(id))) return prev;
        return next;
      });
    };
    terminalManager.onBusyChange = handler;
    return () => { terminalManager.onBusyChange = null; };
  }, []);

  // ── Persist layout (debounced, per-workbench) ──
  const saveLayout = useCallback((panel: PanelNode | null, workbenchId: string | null) => {
    if (!workbenchId) return;
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      const key = layoutKey(workbenchId);
      if (panel) {
        try {
          localStorage.setItem(key, JSON.stringify(panel));
        } catch {
          // localStorage full or unavailable
        }
      } else {
        localStorage.removeItem(key);
      }
    }, 300);
  }, []);

  // ── Load workbenches + sessions + restore layout ──
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        // Ensure at least one workbench exists
        let wbs = await terminalApi.listWorkbenches();
        if (wbs.length === 0) {
          const wb = await terminalApi.createWorkbench('Default');
          wbs = [wb];
        }
        if (cancelled) return;
        setWorkbenches(wbs);

        const firstWb = wbs[0];
        setActiveWorkbenchId(firstWb.id);

        // Load sessions for first workbench
        const fetchedSessions = await terminalApi.list(firstWb.id);
        if (cancelled) return;
        setSessions(fetchedSessions);

        // Try to restore layout
        let restoredLayout: PanelNode | null = null;
        try {
          const raw = localStorage.getItem(layoutKey(firstWb.id));
          if (raw) restoredLayout = JSON.parse(raw) as PanelNode;
        } catch {
          // corrupt data
        }

        if (restoredLayout && fetchedSessions.length > 0) {
          const reconciled = reconcileLayout(restoredLayout, fetchedSessions);
          setRootPanel(reconciled);
          if (reconciled) {
            const leaf = findFirstLeaf(reconciled);
            setActiveSessionId(leaf?.activeTab ?? null);
          }
        } else if (fetchedSessions.length > 0) {
          // No saved layout, create default
          const panel: PanelNode = {
            id: nextPanelId(),
            type: 'leaf',
            tabs: fetchedSessions.map((s) => s.id),
            activeTab: fetchedSessions[0].id,
          };
          setRootPanel(panel);
          setActiveSessionId(fetchedSessions[0].id);
        }
      } catch {
        // API error
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Save layout whenever rootPanel changes
  useEffect(() => {
    saveLayout(rootPanel, activeWorkbenchId);
  }, [rootPanel, activeWorkbenchId, saveLayout]);

  // ── Switch workbench ──
  const switchWorkbench = useCallback(async (id: string) => {
    setActiveWorkbenchId(id);
    setRootPanel(null);
    setActiveSessionId(null);
    setLoading(true);
    try {
      const fetchedSessions = await terminalApi.list(id);
      setSessions(fetchedSessions);

      let restoredLayout: PanelNode | null = null;
      try {
        const raw = localStorage.getItem(layoutKey(id));
        if (raw) restoredLayout = JSON.parse(raw) as PanelNode;
      } catch {
        // corrupt
      }

      if (restoredLayout && fetchedSessions.length > 0) {
        const reconciled = reconcileLayout(restoredLayout, fetchedSessions);
        setRootPanel(reconciled);
        if (reconciled) {
          const leaf = findFirstLeaf(reconciled);
          setActiveSessionId(leaf?.activeTab ?? null);
        }
      } else if (fetchedSessions.length > 0) {
        const panel: PanelNode = {
          id: nextPanelId(),
          type: 'leaf',
          tabs: fetchedSessions.map((s) => s.id),
          activeTab: fetchedSessions[0].id,
        };
        setRootPanel(panel);
        setActiveSessionId(fetchedSessions[0].id);
      }
    } catch {
      // API error
    } finally {
      setLoading(false);
    }
  }, []);

  // ── Create session ──
  const createSession = useCallback(
    async (panelId?: string) => {
      const session = await terminalApi.create({
        workbench_id: activeWorkbenchId ?? undefined,
      });
      setSessions((prev) => [...prev, session]);

      setRootPanel((prev) => {
        if (!prev) {
          const panel: PanelNode = {
            id: nextPanelId(),
            type: 'leaf',
            tabs: [session.id],
            activeTab: session.id,
          };
          return panel;
        }

        const targetId = panelId || findFirstLeaf(prev)?.id;
        if (!targetId) return prev;

        return updatePanel(prev, targetId, (panel) => ({
          ...panel,
          tabs: [...(panel.tabs || []), session.id],
          activeTab: session.id,
        }));
      });

      setActiveSessionId(session.id);
    },
    [activeWorkbenchId],
  );

  // ── Close session ──
  const closeSession = useCallback(async (sessionId: string) => {
    try {
      await terminalApi.delete(sessionId);
    } catch {
      // May already be gone
    }

    terminalManager.release(sessionId);

    setSessions((prev) => prev.filter((s) => s.id !== sessionId));

    setRootPanel((prev) => {
      if (!prev) return null;
      return removeSessionFromTree(prev, sessionId);
    });

    setActiveSessionId((prev) => {
      if (prev !== sessionId) return prev;
      return null;
    });
  }, []);

  // ── Split panel ──
  const splitPanel = useCallback(
    async (panelId: string, direction: 'horizontal' | 'vertical') => {
      const session = await terminalApi.create({
        workbench_id: activeWorkbenchId ?? undefined,
      });
      setSessions((prev) => [...prev, session]);

      setRootPanel((prev) => {
        if (!prev) return prev;

        return updatePanel(prev, panelId, (panel) => {
          const newLeaf: PanelNode = {
            id: nextPanelId(),
            type: 'leaf',
            tabs: [session.id],
            activeTab: session.id,
          };

          const existingLeaf: PanelNode = {
            ...panel,
            id: nextPanelId(),
          };

          return {
            id: panel.id,
            type: 'split',
            direction,
            children: [existingLeaf, newLeaf],
            sizes: [1, 1],
          };
        });
      });

      setActiveSessionId(session.id);
    },
    [activeWorkbenchId],
  );

  // ── Activate tab ──
  const activateTab = useCallback(
    (panelId: string, sessionId: string) => {
      setRootPanel((prev) => {
        if (!prev) return prev;
        return updatePanel(prev, panelId, (panel) => ({
          ...panel,
          activeTab: sessionId,
        }));
      });
      setActiveSessionId(sessionId);
    },
    [],
  );

  // ── Update session (title + color) ──
  const updateSession = useCallback(
    async (sessionId: string, data: { title?: string; color?: string }) => {
      const updated = await terminalApi.update(sessionId, data);
      setSessions((prev) =>
        prev.map((s) => (s.id === sessionId ? updated : s)),
      );
    },
    [],
  );

  // ── Update panel sizes ──
  const updatePanelSizes = useCallback(
    (panelId: string, sizes: number[]) => {
      setRootPanel((prev) => {
        if (!prev) return prev;
        return updatePanel(prev, panelId, (panel) => ({
          ...panel,
          sizes,
        }));
      });
    },
    [],
  );

  // ── Workbench management ──
  const createWorkbench = useCallback(async (name: string) => {
    const wb = await terminalApi.createWorkbench(name);
    setWorkbenches((prev) => [...prev, wb]);
    // Switch to the new workbench
    setActiveWorkbenchId(wb.id);
    setSessions([]);
    setRootPanel(null);
    setActiveSessionId(null);
  }, []);

  const renameWorkbench = useCallback(async (id: string, name: string) => {
    const wb = workbenches.find(w => w.id === id);
    await terminalApi.updateWorkbench(id, { name, color: wb?.color });
    setWorkbenches((prev) =>
      prev.map((w) => (w.id === id ? { ...w, name } : w)),
    );
  }, [workbenches]);

  const updateWorkbenchColor = useCallback(async (id: string, color: string) => {
    const wb = workbenches.find(w => w.id === id);
    await terminalApi.updateWorkbench(id, { name: wb?.name || 'Workspace', color });
    setWorkbenches((prev) =>
      prev.map((w) => (w.id === id ? { ...w, color } : w)),
    );
  }, [workbenches]);

  const deleteWorkbench = useCallback(async (id: string) => {
    await terminalApi.deleteWorkbench(id);
    // Release all terminals that belonged to this workbench
    const sessionsToRelease = sessions.filter((s) => s.workbench_id === id);
    terminalManager.releaseAll(sessionsToRelease.map((s) => s.id));
    // Remove layout from localStorage
    localStorage.removeItem(layoutKey(id));
    setWorkbenches((prev) => {
      const next = prev.filter((wb) => wb.id !== id);
      // If we deleted the active workbench, switch to first remaining
      if (id === activeWorkbenchId && next.length > 0) {
        switchWorkbench(next[0].id);
      }
      return next;
    });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeWorkbenchId, sessions]);

  return (
    <WorkbenchContext.Provider
      value={{
        sessions,
        workbenches,
        activeWorkbenchId,
        rootPanel,
        activeSessionId,
        createSession,
        closeSession,
        splitPanel,
        activateTab,
        updateSession,
        updatePanelSizes,
        createWorkbench,
        renameWorkbench,
        updateWorkbenchColor,
        deleteWorkbench,
        switchWorkbench,
        busySessions,
        loading,
      }}
    >
      {children}
    </WorkbenchContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useWorkbench(): WorkbenchContextType {
  const ctx = useContext(WorkbenchContext);
  if (!ctx)
    throw new Error('useWorkbench must be used within WorkbenchProvider');
  return ctx;
}
