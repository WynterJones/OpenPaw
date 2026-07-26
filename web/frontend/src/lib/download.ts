/**
 * Saving a file to disk from either shell.
 *
 * In a normal browser an <a download> is all it takes. Inside the Tauri webview
 * it does nothing at all — there is no download manager, so the click is
 * silently swallowed. Handing the URL to the real browser via
 * /system/open-external gets a normal download instead.
 */

import { isTauri } from './tauri';
import { openExternal } from './openExternal';

/** Relative API paths have to become absolute before leaving the webview. */
function absolute(url: string): string {
  if (/^https?:\/\//i.test(url)) return url;
  return new URL(url, window.location.origin).toString();
}

/**
 * Downloads a URL. `filename` is only a hint for the browser path — the server
 * sets Content-Disposition, which wins.
 */
export function downloadFile(url: string, filename?: string): void {
  if (!url) return;

  if (isTauri()) {
    openExternal(absolute(url));
    return;
  }

  const a = document.createElement('a');
  a.href = url;
  if (filename) a.download = filename;
  a.rel = 'noreferrer';
  document.body.appendChild(a);
  a.click();
  a.remove();
}
