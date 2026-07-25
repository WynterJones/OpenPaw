import { createContext, useContext } from 'react';

/**
 * The three layout panes the user can show or hide.
 *
 * Context and hook live in this plain .ts module — separate from the provider
 * component — so the provider file exports only components and stays
 * fast-refresh friendly.
 */
export type ViewToggleKey = 'sidebar' | 'chatList' | 'chatPanel';

export interface ViewTogglesValue {
  /** true = visible. */
  sidebar: boolean;
  chatList: boolean;
  chatPanel: boolean;
  toggle: (key: ViewToggleKey) => void;
  set: (key: ViewToggleKey, visible: boolean) => void;
}

export const STORAGE_KEYS: Record<ViewToggleKey, string> = {
  sidebar: 'openpaw_show_sidebar',
  chatList: 'openpaw_show_chat_list',
  chatPanel: 'openpaw_show_chat_panel',
};

export const ViewTogglesContext = createContext<ViewTogglesValue | null>(null);

export function useViewToggles(): ViewTogglesValue {
  const ctx = useContext(ViewTogglesContext);
  if (!ctx) throw new Error('useViewToggles must be used inside ViewTogglesProvider');
  return ctx;
}
