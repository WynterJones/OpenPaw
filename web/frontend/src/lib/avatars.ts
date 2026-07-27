/**
 * The preset avatar set, shared by every picker.
 *
 * Files live at `web/frontend/public/avatars/avatar-<n>.webp` (256x256), numbered
 * contiguously from 1 — the list is generated from the count, so a gap in the
 * numbering renders as a broken image.
 *
 * This is deliberately ONE constant rather than a copy per page. It used to be
 * duplicated across Setup, AgentEdit, GatewayEdit and Agents, and they drifted:
 * the gateway picker was left on 45 avatars while the agent picker had 141, and
 * the create-agent modal offered only a hardcoded 6.
 *
 * To add avatars: drop the files in and raise AVATAR_COUNT.
 */
const AVATAR_COUNT = 225;

export const PRESET_AVATARS: string[] = Array.from(
  { length: AVATAR_COUNT },
  (_, i) => `/avatars/avatar-${i + 1}.webp`,
);
