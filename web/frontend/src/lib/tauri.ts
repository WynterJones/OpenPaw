// Desktop-app window dragging.
// The desktop app loads this UI from the local Go server (a remote origin), so
// Tauri's automatic `data-tauri-drag-region` handling is not injected. Instead we
// call the window's startDragging() manually via the global Tauri API
// (enabled with `withGlobalTauri: true`, and the remote origin is granted in the
// desktop capability's `remote.urls`). No-ops in a normal browser.

/* eslint-disable @typescript-eslint/no-explicit-any */

const INTERACTIVE =
  'button, a, input, textarea, select, label, [role="button"], [role="switch"], [data-no-drag]';

export function isTauri(): boolean {
  return (
    typeof window !== "undefined" &&
    ("__TAURI__" in window || "__TAURI_INTERNALS__" in window)
  );
}

function invokeStartDragging(): void {
  const w = window as any;
  const tauri = w.__TAURI__;

  // Preferred: global api — window module.
  try {
    const win = tauri?.window?.getCurrentWindow?.() ?? tauri?.window?.getCurrent?.();
    if (win?.startDragging) {
      win.startDragging();
      return;
    }
  } catch {
    /* fall through */
  }

  // Alt: webviewWindow module.
  try {
    const wv =
      tauri?.webviewWindow?.getCurrentWebviewWindow?.() ??
      tauri?.webviewWindow?.getCurrent?.();
    if (wv?.startDragging) {
      wv.startDragging();
      return;
    }
  } catch {
    /* fall through */
  }

  // Last resort: raw IPC command.
  try {
    const invoke = tauri?.core?.invoke ?? w.__TAURI_INTERNALS__?.invoke;
    invoke?.("plugin:window|start_dragging");
  } catch {
    /* no-op */
  }
}

export function startWindowDrag(e: React.MouseEvent): void {
  // Only primary button, and never when starting on an interactive control.
  if (e.button !== 0) return;
  if (!isTauri()) return;
  const target = e.target as HTMLElement | null;
  if (target && target.closest(INTERACTIVE)) return;
  invokeStartDragging();
}

// Native OS folder/file drag-drop (desktop app only). Tauri delivers real
// absolute paths via a global webview event — the browser's HTML drop event
// can't expose filesystem paths. No-ops (returns a no-op unlisten) in a normal
// browser, where dragging a folder can't yield a usable path anyway.
export function listenFolderDrop(handlers: {
  onEnter?: () => void;
  onLeave?: () => void;
  onDrop?: (paths: string[]) => void;
}): () => void {
  if (!isTauri()) return () => {};
  const ev = (window as any).__TAURI__?.event;
  if (!ev?.listen) return () => {};
  const pending: Array<Promise<() => void>> = [
    ev.listen("tauri://drag-enter", () => handlers.onEnter?.()),
    ev.listen("tauri://drag-leave", () => handlers.onLeave?.()),
    ev.listen("tauri://drag-drop", (e: any) => {
      handlers.onLeave?.();
      const paths: string[] = e?.payload?.paths ?? [];
      if (paths.length > 0) handlers.onDrop?.(paths);
    }),
  ];
  return () => {
    pending.forEach((p) => p.then((u) => u()).catch(() => {}));
  };
}
