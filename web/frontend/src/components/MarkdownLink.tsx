/**
 * Shared react-markdown overrides for anywhere that renders markdown without
 * the chat's full mention handling — pin summaries, Inbox reports, and so on.
 *
 * Exists so every markdown surface routes links through the same
 * open-in-real-browser path; a surface that forgets ends up with links that do
 * nothing at all inside the Tauri webview.
 */

import type { Components } from 'react-markdown';
import { handleExternalLinkClick } from '../lib/openExternal';

export const markdownLinkComponents: Partial<Components> = {
  a: ({ href, children, ...props }) => (
    <a
      href={href}
      target="_blank"
      rel="noreferrer"
      onClick={(e) => handleExternalLinkClick(e, href)}
      {...props}
    >
      {children}
    </a>
  ),
};
