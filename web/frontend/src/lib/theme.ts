import type { DesignConfig } from './api';

export type ThemeMode = 'dark' | 'light';

export interface ThemeInput {
  accent: string;
  mode: ThemeMode;
}

export const DEFAULT_ACCENT = '#E84BA5';
export const DEFAULT_MODE: ThemeMode = 'dark';

// Convert hex to RGB
function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '');
  return [
    parseInt(h.slice(0, 2), 16) / 255,
    parseInt(h.slice(2, 4), 16) / 255,
    parseInt(h.slice(4, 6), 16) / 255,
  ];
}

// Linear RGB to sRGB
function linearToSrgb(c: number): number {
  return c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055;
}

// sRGB to linear RGB
function srgbToLinear(c: number): number {
  return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

// RGB to OKLCH via OKLab
function rgbToOklch(r: number, g: number, b: number): [number, number, number] {
  const lr = srgbToLinear(r);
  const lg = srgbToLinear(g);
  const lb = srgbToLinear(b);

  const l_ = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m_ = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s_ = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);

  const L = 0.2104542553 * l_ + 0.7936177850 * m_ - 0.0040720468 * s_;
  const a = 1.9779984951 * l_ - 2.4285922050 * m_ + 0.4505937099 * s_;
  const bVal = 0.0259040371 * l_ + 0.7827717662 * m_ - 0.8086757660 * s_;

  const C = Math.sqrt(a * a + bVal * bVal);
  let h = (Math.atan2(bVal, a) * 180) / Math.PI;
  if (h < 0) h += 360;

  return [L, C, h];
}

// OKLCH to RGB
function oklchToRgb(L: number, C: number, h: number): [number, number, number] {
  const hRad = (h * Math.PI) / 180;
  const a = C * Math.cos(hRad);
  const b = C * Math.sin(hRad);

  const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = L - 0.0894841775 * a - 1.2914855480 * b;

  const l = l_ * l_ * l_;
  const m = m_ * m_ * m_;
  const s = s_ * s_ * s_;

  const r = +4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s;
  const g = -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s;
  const bOut = -0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s;

  return [
    Math.max(0, Math.min(1, linearToSrgb(r))),
    Math.max(0, Math.min(1, linearToSrgb(g))),
    Math.max(0, Math.min(1, linearToSrgb(bOut))),
  ];
}

function rgbToHex(r: number, g: number, b: number): string {
  const toHex = (c: number) => Math.round(c * 255).toString(16).padStart(2, '0');
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`;
}

function oklchHex(L: number, C: number, h: number): string {
  const [r, g, b] = oklchToRgb(L, C, h);
  return rgbToHex(r, g, b);
}

export function generateTheme(input: ThemeInput): DesignConfig {
  const [r, g, b] = hexToRgb(input.accent);
  const [accentL, accentC, accentH] = rgbToOklch(r, g, b);
  const dark = input.mode === 'dark';

  // Accent variations using the hue from user's chosen accent
  const isPastel = accentL > 0.75;
  const accent = input.accent;
  const accentHover = isPastel
    ? oklchHex(dark ? accentL - 0.05 : accentL - 0.08, accentC, accentH)
    : oklchHex(dark ? 0.62 : 0.55, accentC, accentH);
  const accentText = oklchHex(dark ? 0.78 : 0.50, Math.min(accentC, 0.18), accentH);
  const accentBtnText = isPastel
    ? oklchHex(0.25, Math.min(accentC, 0.15), accentH)
    : '#ffffff';
  const accentMutedRgb = hexToRgb(accent);
  const accentMuted = `rgba(${Math.round(accentMutedRgb[0] * 255)}, ${Math.round(accentMutedRgb[1] * 255)}, ${Math.round(accentMutedRgb[2] * 255)}, ${dark ? 0.1 : 0.12})`;

  // Neutral surfaces — use a very subtle tint of the accent hue
  const neutralC = 0.005; // barely perceptible chroma
  const neutralH = accentH;

  let surface_0: string, surface_1: string, surface_2: string, surface_3: string;
  let border_0: string, border_1: string;
  let text_0: string, text_1: string, text_2: string, text_3: string;

  if (dark) {
    surface_0 = oklchHex(0.00, 0, neutralH);       // pure black
    surface_1 = oklchHex(0.15, neutralC, neutralH); // near black
    surface_2 = oklchHex(0.20, neutralC, neutralH); // dark
    surface_3 = oklchHex(0.25, neutralC, neutralH); // elevated
    border_0  = oklchHex(0.22, neutralC, neutralH);
    border_1  = oklchHex(0.30, neutralC, neutralH);
    text_0    = oklchHex(0.96, 0, neutralH);        // near white
    text_1    = oklchHex(0.85, 0, neutralH);
    text_2    = oklchHex(0.60, 0, neutralH);
    text_3    = oklchHex(0.42, 0, neutralH);
  } else {
    surface_0 = oklchHex(0.985, 0, neutralH);       // near white
    surface_1 = oklchHex(0.97, neutralC, neutralH);  // light
    surface_2 = oklchHex(0.94, neutralC, neutralH);  // slightly darker
    surface_3 = oklchHex(0.91, neutralC, neutralH);  // elevated
    border_0  = oklchHex(0.92, neutralC, neutralH);
    border_1  = oklchHex(0.85, neutralC, neutralH);
    text_0    = oklchHex(0.13, 0, neutralH);         // near black
    text_1    = oklchHex(0.25, 0, neutralH);
    text_2    = oklchHex(0.45, 0, neutralH);
    text_3    = oklchHex(0.62, 0, neutralH);
  }

  // Danger colors stay consistent
  const danger = dark ? '#dc2626' : '#dc2626';
  const dangerHover = dark ? '#b91c1c' : '#ef4444';

  return {
    surface_0,
    surface_1,
    surface_2,
    surface_3,
    border_0,
    border_1,
    text_0,
    text_1,
    text_2,
    text_3,
    accent,
    accent_hover: accentHover,
    accent_muted: accentMuted,
    accent_text: accentText,
    accent_btn_text: accentBtnText,
    danger,
    danger_hover: dangerHover,
    font_family: "'Vend Sans', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
    font_scale: '100',
    bg_image: '',
  };
}
