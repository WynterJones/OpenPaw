import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { ViewTogglesContext, STORAGE_KEYS, type ViewToggleKey } from './viewToggles';

/**
 * Owns the show/hide state for the sidebar, chat list and chat panel.
 *
 * These used to be three separate buttons, each owning its own state — one in
 * the sidebar footer, two in the chat header. They live here so a single menu in
 * the header can drive all of them, and so the chat panes keep their setting
 * when you navigate away and back.
 */
function loadInitial(key: ViewToggleKey, fallback: boolean): boolean {
  const stored = localStorage.getItem(STORAGE_KEYS[key]);
  if (stored === '1') return true;
  if (stored === '0') return false;
  return fallback;
}

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
