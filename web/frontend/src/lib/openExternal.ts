/**
 * Opening links in the user's real browser.
 *
 * `target="_blank"` is enough in a normal browser but does nothing inside the
 * Tauri webview, which has no tabs and blocks new windows — so links in chat
 * simply didn't respond in the desktop app. The Go server already exposes
 * /system/open-external (it runs on the user's own machine in both the desktop
 * and npx cases), so routing the click through it hands the URL to the real
 * default browser.
 */

import { api } from './api';
import { isTauri } from './tauri';

/** Only http(s) is handed off — the server rejects anything else anyway. */
function isExternalUrl(url: string): boolean {
  return /^https?:\/\//i.test(url);
}

/**
 * Opens a URL outside the app. Returns true if it handled the click, so callers
 * can leave the browser to its own default behaviour when it didn't.
 */
export function openExternal(url: string): boolean {
  if (!url || !isExternalUrl(url)) return false;

  if (isTauri()) {
    api.post('/system/open-external', { url }).catch(() => {
      // Last resort: better a webview navigation than a link that does nothing.
      window.open(url, '_blank', 'noopener,noreferrer');
    });
    return true;
  }

  window.open(url, '_blank', 'noopener,noreferrer');
  return true;
}

/**
 * Click handler for rendered markdown anchors. Leaves modified clicks
 * (cmd/ctrl/shift/middle) alone so browser users keep "open in new tab" and
 * friends.
 */
export function handleExternalLinkClick(
  e: React.MouseEvent<HTMLAnchorElement>,
  href: string | undefined,
): void {
  if (!href || e.defaultPrevented) return;
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
  if (!isExternalUrl(href)) return;

  // Only intercept in the desktop shell. In a real browser the anchor's own
  // target="_blank" already does the right thing, and hijacking it into
  // window.open() would just invite popup blocking.
  if (!isTauri()) return;

  e.preventDefault();
  openExternal(href);
}
