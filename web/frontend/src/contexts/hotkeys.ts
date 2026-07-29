import { createContext, useContext } from 'react';
import type { HotkeyModifier } from '../lib/app-navigation';

export interface HotkeySettings {
  enabled: boolean;
  modifier: HotkeyModifier;
  showBadges: boolean;
  bindings: Record<string, string>;
}

export interface HotkeysValue extends HotkeySettings {
  paletteOpen: boolean;
  setPaletteOpen: (open: boolean) => void;
  update: (next: Partial<HotkeySettings>) => Promise<void>;
  runNewChat: () => void;
}

export const defaultHotkeySettings: HotkeySettings = {
  enabled: true,
  modifier: 'ctrl',
  showBadges: false,
  bindings: {},
};

export const HotkeysContext = createContext<HotkeysValue>({
  ...defaultHotkeySettings,
  paletteOpen: false,
  setPaletteOpen: () => {},
  update: async () => {},
  runNewChat: () => {},
});

export const useHotkeys = () => useContext(HotkeysContext);
