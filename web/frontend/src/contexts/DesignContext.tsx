import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { api, type DesignConfig } from '../lib/api';
import { generateTheme, DEFAULT_ACCENT, DEFAULT_MODE, type ThemeMode, type ThemeInput } from '../lib/theme';

const DEFAULT_CONFIG: DesignConfig = generateTheme({ accent: DEFAULT_ACCENT, mode: DEFAULT_MODE });

function configToCSSVars(config: DesignConfig): Record<string, string> {
  const vars: Record<string, string> = {};
  for (const [key, value] of Object.entries(config)) {
    if (key === 'bg_image') continue;
    const cssKey = `--op-${key.replace(/_/g, '-')}`;
    vars[cssKey] = value;
  }
  return vars;
}

function applyVars(vars: Record<string, string>) {
  const root = document.documentElement;
  for (const [key, value] of Object.entries(vars)) {
    root.style.setProperty(key, value);
  }
}

interface SaveAllInput {
  accent: string;
  mode: ThemeMode;
  fontFamily: string;
  fontScale: string;
  bgImage: string;
  showMascot: boolean;
}

interface DesignContextType {
  config: DesignConfig;
  accent: string;
  mode: ThemeMode;
  bgImage: string;
  showMascot: boolean;
  updateTheme: (input: ThemeInput) => Promise<void>;
  updateConfig: (partial: Partial<DesignConfig>) => Promise<void>;
  updateBgImage: (url: string) => Promise<void>;
  updateShowMascot: (show: boolean) => Promise<void>;
  saveAll: (input: SaveAllInput) => Promise<void>;
  resetConfig: () => Promise<void>;
}

const DesignContext = createContext<DesignContextType | null>(null);

export function DesignProvider({ children }: { children: ReactNode }) {
  const [config, setConfig] = useState<DesignConfig>(DEFAULT_CONFIG);
  const [accent, setAccent] = useState(DEFAULT_ACCENT);
  const [mode, setMode] = useState<ThemeMode>(DEFAULT_MODE);
  const [bgImage, setBgImage] = useState('');
  const [showMascot, setShowMascot] = useState(true);

  useEffect(() => {
    applyVars(configToCSSVars(config));
  }, [config]);

  useEffect(() => {
    api.get<{ design: DesignConfig & { _accent?: string; _mode?: ThemeMode; _bg_image?: string; _show_mascot?: boolean } }>('/settings/design')
      .then(data => {
        if (data.design) {
          if (data.design._accent) setAccent(data.design._accent);
          if (data.design._mode) setMode(data.design._mode);
          if (data.design._bg_image !== undefined) setBgImage(data.design._bg_image);
          if (data.design._show_mascot !== undefined) setShowMascot(data.design._show_mascot);
          setConfig(prev => ({ ...prev, ...data.design }));
        }
      })
      .catch(() => {});
  }, []);

  const updateTheme = useCallback(async (input: ThemeInput) => {
    const next = generateTheme(input);
    setAccent(input.accent);
    setMode(input.mode);
    setConfig(prev => ({ ...next, bg_image: prev.bg_image }));
    try {
      await api.put('/settings/design', { ...next, _accent: input.accent, _mode: input.mode, _bg_image: bgImage, _show_mascot: showMascot });
    } catch (e) { console.warn('updateTheme failed:', e); }
  }, [bgImage, showMascot]);

  const updateConfig = useCallback(async (partial: Partial<DesignConfig>) => {
    const next = { ...config, ...partial };
    setConfig(next);
    try {
      await api.put('/settings/design', { ...next, _accent: accent, _mode: mode, _bg_image: bgImage, _show_mascot: showMascot });
    } catch (e) { console.warn('updateConfig failed:', e); }
  }, [config, accent, mode, bgImage, showMascot]);

  const updateBgImage = useCallback(async (url: string) => {
    setBgImage(url);
    try {
      await api.put('/settings/design', { ...config, _accent: accent, _mode: mode, _bg_image: url, _show_mascot: showMascot });
    } catch (e) { console.warn('updateBgImage failed:', e); }
  }, [config, accent, mode, showMascot]);

  const updateShowMascot = useCallback(async (show: boolean) => {
    setShowMascot(show);
    try {
      await api.put('/settings/design', { ...config, _accent: accent, _mode: mode, _bg_image: bgImage, _show_mascot: show });
    } catch (e) { console.warn('updateShowMascot failed:', e); }
  }, [config, accent, mode, bgImage]);

  const saveAll = useCallback(async (input: SaveAllInput) => {
    const theme = generateTheme({ accent: input.accent, mode: input.mode });
    const next = { ...theme, font_family: input.fontFamily, font_scale: input.fontScale };
    setAccent(input.accent);
    setMode(input.mode);
    setBgImage(input.bgImage);
    setShowMascot(input.showMascot);
    setConfig(prev => ({ ...prev, ...next }));
    try {
      await api.put('/settings/design', { ...next, _accent: input.accent, _mode: input.mode, _bg_image: input.bgImage, _show_mascot: input.showMascot });
    } catch (e) { console.warn('saveAll failed:', e); }
  }, []);

  const resetConfig = useCallback(async () => {
    const next = generateTheme({ accent: DEFAULT_ACCENT, mode: DEFAULT_MODE });
    setAccent(DEFAULT_ACCENT);
    setMode(DEFAULT_MODE);
    setBgImage('');
    setShowMascot(true);
    setConfig(next);
    try {
      await api.put('/settings/design', { ...next, _accent: DEFAULT_ACCENT, _mode: DEFAULT_MODE, _bg_image: '', _show_mascot: true });
    } catch (e) { console.warn('resetConfig failed:', e); }
  }, []);

  return (
    <DesignContext.Provider value={{ config, accent, mode, bgImage, showMascot, updateTheme, updateConfig, updateBgImage, updateShowMascot, saveAll, resetConfig }}>
      {children}
    </DesignContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useDesign() {
  const ctx = useContext(DesignContext);
  if (!ctx) throw new Error('useDesign must be used within DesignProvider');
  return ctx;
}
