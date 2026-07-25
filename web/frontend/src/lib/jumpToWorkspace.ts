import { workspaces } from './api-helpers';

/**
 * Navigate to something that may live in a different workspace.
 *
 * Most of the app is scoped server-side to the ACTIVE workspace, so opening a
 * chat or terminal belonging to another one has to switch first — otherwise the
 * target screen loads scoped to the wrong workspace and shows nothing. Switching
 * re-scopes the server, so the reliable way to pick up the new scope everywhere
 * is a full load of the destination rather than a client-side route change.
 */
export async function jumpToWorkspace(workspaceId: string | undefined, path: string): Promise<void> {
  if (!workspaceId) {
    window.location.href = path;
    return;
  }
  try {
    const active = await workspaces.getActive();
    if (active?.id === workspaceId) {
      window.location.href = path;
      return;
    }
    await workspaces.setActive(workspaceId);
  } catch {
    // Fall through: still navigate, just without the switch.
  }
  window.location.href = path;
}
