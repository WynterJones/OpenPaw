import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { ViewTogglesContext, STORAGE_KEYS, type ViewToggleKey } from './viewToggles';

/**
 * Owns the show/hide state for the sidebar, chat list, chat panel and canvas.
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
  // The canvas always starts closed, whatever it was last session: it takes half
  // the screen, and reopening the app into a stale preview helps nobody.
  const [canvas, setCanvas] = useState(false);

  useEffect(() => { localStorage.setItem(STORAGE_KEYS.sidebar, sidebar ? '1' : '0'); }, [sidebar]);
  useEffect(() => { localStorage.setItem(STORAGE_KEYS.chatList, chatList ? '1' : '0'); }, [chatList]);
  useEffect(() => { localStorage.setItem(STORAGE_KEYS.chatPanel, chatPanel ? '1' : '0'); }, [chatPanel]);

  // What the side panes looked like before the canvas took the screen over, so
  // closing it is a real undo rather than a guess at what was there.
  const beforeCanvas = useRef<{ sidebar: boolean; chatList: boolean; chatPanel: boolean } | null>(null);

  // Unlike the other three, the canvas rearranges the rest: opening it clears
  // the side panes so the conversation and the preview get the width.
  const setCanvasVisible = useCallback((visible: boolean) => {
    setCanvas(prev => {
      if (prev === visible) return prev;
      if (visible) {
        beforeCanvas.current = { sidebar, chatList, chatPanel };
        setSidebar(false);
        setChatList(false);
        setChatPanel(false);
      } else if (beforeCanvas.current) {
        setSidebar(beforeCanvas.current.sidebar);
        setChatList(beforeCanvas.current.chatList);
        setChatPanel(beforeCanvas.current.chatPanel);
        beforeCanvas.current = null;
      }
      return visible;
    });
  }, [sidebar, chatList, chatPanel]);

  const set = useCallback((key: ViewToggleKey, visible: boolean) => {
    if (key === 'sidebar') setSidebar(visible);
    else if (key === 'chatList') setChatList(visible);
    else if (key === 'chatPanel') setChatPanel(visible);
    else setCanvasVisible(visible);
  }, [setCanvasVisible]);

  const toggle = useCallback((key: ViewToggleKey) => {
    if (key === 'sidebar') setSidebar(v => !v);
    else if (key === 'chatList') setChatList(v => !v);
    else if (key === 'chatPanel') setChatPanel(v => !v);
    else setCanvasVisible(!canvas);
  }, [canvas, setCanvasVisible]);

  const value = useMemo(
    () => ({ sidebar, chatList, chatPanel, canvas, toggle, set }),
    [sidebar, chatList, chatPanel, canvas, toggle, set],
  );

  return <ViewTogglesContext.Provider value={value}>{children}</ViewTogglesContext.Provider>;
}
