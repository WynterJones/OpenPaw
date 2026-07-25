import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

/**
 * The three layout panes the user can show or hide.
 *
 * These used to be three separate buttons scattered across the UI — one in the
 * sidebar footer, two in the chat header — each owning its own state. They live
 * here so a single menu in the header can drive all of them, and so the chat
 * panes keep their setting when you navigate away and back.
 */
export type ViewToggleKey = 'sidebar' | 'chatList' | 'chatPanel';

interface ViewTogglesValue {
  /** true = visible. */
  sidebar: boolean;
  chatList: boolean;
  chatPanel: boolean;
  toggle: (key: ViewToggleKey) => void;
  set: (key: ViewToggleKey, visible: boolean) => void;
}

const STORAGE_KEYS: Record<ViewToggleKey, string> = {
  sidebar: 'openpaw_show_sidebar',
  chatList: 'openpaw_show_chat_list',
  chatPanel: 'openpaw_show_chat_panel',
};

function loadInitial(key: ViewToggleKey, fallback: boolean): boolean {
  const stored = localStorage.getItem(STORAGE_KEYS[key]);
  if (stored === '1') return true;
  if (stored === '0') return false;
  return fallback;
}

const ViewTogglesContext = createContext<ViewTogglesValue | null>(null);

export function ViewTogglesProvider({ children }: { children: ReactNode }) {
  const [sidebar, setSidebar] = useState(() => loadInitial('sidebar', true));
  const [chatList, setChatList] = useState(() => loadInitial('chatList', true));
  // The right-hand chat panel starts hidden on narrow screens, where it would
  // otherwise cover the conversation.
  const [chatPanel, setChatPanel] = useState(() => loadInitial('chatPanel', window.innerWidth >= 1280));

  useEffect(() => { localStorage.setItem(STORAGE_KEYS.sidebar, sidebar ? '1' : '0'); }, [sidebar]);
  useEffect(() => { localStorage.setItem(STORAGE_KEYS.chatList, chatList ? '1' : '0'); }, [chatList]);
  useEffect(() => { localStorage.setItem(STORAGE_KEYS.chatPanel, chatPanel ? '1' : '0'); }, [chatPanel]);

  const set = useCallback((key: ViewToggleKey, visible: boolean) => {
    if (key === 'sidebar') setSidebar(visible);
    else if (key === 'chatList') setChatList(visible);
    else setChatPanel(visible);
  }, []);

  const toggle = useCallback((key: ViewToggleKey) => {
    if (key === 'sidebar') setSidebar(v => !v);
    else if (key === 'chatList') setChatList(v => !v);
    else setChatPanel(v => !v);
  }, []);

  const value = useMemo(
    () => ({ sidebar, chatList, chatPanel, toggle, set }),
    [sidebar, chatList, chatPanel, toggle, set],
  );

  return <ViewTogglesContext.Provider value={value}>{children}</ViewTogglesContext.Provider>;
}

export function useViewToggles(): ViewTogglesValue {
  const ctx = useContext(ViewTogglesContext);
  if (!ctx) throw new Error('useViewToggles must be used inside ViewTogglesProvider');
  return ctx;
}
