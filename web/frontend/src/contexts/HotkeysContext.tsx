import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import { api } from '../lib/api';
import {
  APP_NAV_ITEMS,
  DEFAULT_HOTKEY_BINDINGS,
  type HotkeyModifier,
} from '../lib/app-navigation';
import { HotkeysContext, type HotkeySettings } from './hotkeys';

const defaults: HotkeySettings = {
  enabled: true,
  modifier: 'ctrl',
  showBadges: true,
  bindings: DEFAULT_HOTKEY_BINDINGS,
};

function parseSettings(data: Record<string, string>): HotkeySettings {
  const modifier = data.hotkey_modifier;
  let bindings = DEFAULT_HOTKEY_BINDINGS;
  try {
    bindings = { ...bindings, ...JSON.parse(data.hotkey_bindings || '{}') };
  } catch {
    // A hand-edited invalid setting should fall back to usable defaults.
  }
  return {
    enabled: data.hotkeys_enabled !== 'false',
    modifier: modifier === 'meta' || modifier === 'alt' ? modifier : 'ctrl',
    showBadges: data.hotkey_badges !== 'false',
    bindings,
  };
}

function matchesModifier(event: KeyboardEvent, modifier: HotkeyModifier) {
  if (modifier === 'meta') return event.metaKey && !event.ctrlKey && !event.altKey;
  if (modifier === 'alt') return event.altKey && !event.ctrlKey && !event.metaKey;
  return event.ctrlKey && !event.metaKey && !event.altKey;
}

export function HotkeysProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [settings, setSettings] = useState<HotkeySettings>(defaults);
  const [paletteOpen, setPaletteOpen] = useState(false);

  const refresh = useCallback(() => {
    api.get<Record<string, string>>('/settings')
      .then(data => setSettings(parseSettings(data)))
      .catch(() => {});
  }, []);

  useEffect(() => {
    refresh();
    window.addEventListener('openpaw:hotkeys-updated', refresh);
    return () => window.removeEventListener('openpaw:hotkeys-updated', refresh);
  }, [refresh]);

  const runNewChat = useCallback(() => {
    sessionStorage.setItem('openpaw_new_chat_requested', 'true');
    navigate('/chat');
    window.dispatchEvent(new Event('openpaw:new-chat'));
  }, [navigate]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!settings.enabled || event.isComposing || !matchesModifier(event, settings.modifier)) return;
      const key = event.key.toLowerCase();
      if (key === 'p') {
        event.preventDefault();
        setPaletteOpen(open => !open);
        return;
      }
      if (key === 'n') {
        event.preventDefault();
        runNewChat();
        return;
      }
      const item = APP_NAV_ITEMS.find(nav => {
        const binding = (settings.bindings[nav.id] || '').toLowerCase();
        if (binding !== key) return false;
        return /^[a-z]$/.test(binding) ? event.shiftKey : !event.shiftKey;
      });
      if (item) {
        event.preventDefault();
        navigate(item.to);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [navigate, runNewChat, settings]);

  const update = useCallback(async (next: Partial<HotkeySettings>) => {
    const merged = { ...settings, ...next };
    setSettings(merged);
    await api.put('/settings', {
      hotkeys_enabled: merged.enabled ? 'true' : 'false',
      hotkey_modifier: merged.modifier,
      hotkey_badges: merged.showBadges ? 'true' : 'false',
      hotkey_bindings: JSON.stringify(merged.bindings),
    });
    window.dispatchEvent(new Event('openpaw:hotkeys-updated'));
  }, [settings]);

  const value = useMemo(() => ({
    ...settings,
    paletteOpen,
    setPaletteOpen,
    update,
    runNewChat,
  }), [paletteOpen, runNewChat, settings, update]);

  return <HotkeysContext.Provider value={value}>{children}</HotkeysContext.Provider>;
}
