import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import '@xterm/xterm/css/xterm.css';
import { terminalApi } from './api-helpers';

interface TerminalInstance {
  term: Terminal;
  fitAddon: FitAddon;
  ws: WebSocket | null;
  wsUrl: string;
  containerEl: HTMLDivElement;
  resizeObserver: ResizeObserver | null;
  dataDisposable: { dispose: () => void } | null;
  binaryDisposable: { dispose: () => void } | null;
  onExit: ((sessionId: string) => void) | null;
  fitTimer: ReturnType<typeof setTimeout> | null;
  busyTimer: ReturnType<typeof setTimeout> | null;
  isBusy: boolean;
  initialGrace: boolean;
  dropOverlay: HTMLDivElement | null;
  pasteDisposable: { dispose: () => void } | null;
  reconnectTimer: ReturnType<typeof setTimeout> | null;
  reconnectAttempts: number;
  intentionalClose: boolean;
}

const encoder = new TextEncoder();

function css(prop: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(prop).trim();
}

function buildTheme(): Record<string, string> {
  const bg = css('--op-surface-0') || '#000000';
  const fg = css('--op-text-1') || '#d4d4d4';
  const accent = css('--op-accent') || '#E84BA5';
  const accentText = css('--op-accent-text') || '#F472B6';

  return {
    background: bg,
    foreground: fg,
    cursor: accent,
    cursorAccent: bg,
    selectionBackground: css('--op-accent-muted') || 'rgba(232, 75, 165, 0.15)',
    selectionForeground: css('--op-text-0') || '#f5f5f5',

    // ANSI normal — muted tones that sit well on dark backgrounds
    black: css('--op-surface-3') || '#1f1f1f',
    red: '#f87171',
    green: '#4ade80',
    yellow: '#facc15',
    blue: '#60a5fa',
    magenta: accent,
    cyan: '#22d3ee',
    white: css('--op-text-1') || '#d4d4d4',

    // ANSI bright
    brightBlack: css('--op-text-3') || '#767676',
    brightRed: '#fca5a5',
    brightGreen: '#86efac',
    brightYellow: '#fde68a',
    brightBlue: '#93c5fd',
    brightMagenta: accentText,
    brightCyan: '#67e8f9',
    brightWhite: css('--op-text-0') || '#f5f5f5',
  };
}

const dragCleanupMap = new WeakMap<HTMLElement, () => void>();

class TerminalManager {
  private instances = new Map<string, TerminalInstance>();
  onBusyChange: ((sessionId: string, busy: boolean) => void) | null = null;

  acquire(sessionId: string): TerminalInstance {
    let instance = this.instances.get(sessionId);
    if (instance) return instance;

    const theme = buildTheme();
    const isMobile = window.innerWidth < 768;
    const term = new Terminal({
      scrollback: 5000,
      cursorBlink: true,
      fontSize: isMobile ? 11 : 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme,
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    // Create a container div owned by the manager
    const containerEl = document.createElement('div');
    containerEl.style.width = '100%';
    containerEl.style.height = '100%';

    // Open terminal into its own container
    term.open(containerEl);

    // Try WebGL addon
    try {
      const webglAddon = new WebglAddon();
      webglAddon.onContextLoss(() => webglAddon.dispose());
      term.loadAddon(webglAddon);
    } catch {
      // WebGL not available
    }

    // Initial fit
    try { fitAddon.fit(); } catch { /* container may not be sized */ }

    instance = {
      term,
      fitAddon,
      ws: null,
      wsUrl: '',
      containerEl,
      resizeObserver: null,
      dataDisposable: null,
      binaryDisposable: null,
      onExit: null,
      fitTimer: null,
      busyTimer: null,
      isBusy: false,
      initialGrace: true,
      dropOverlay: null,
      pasteDisposable: null,
      reconnectTimer: null,
      reconnectAttempts: 0,
      intentionalClose: false,
    };

    this.instances.set(sessionId, instance);

    // Connect WebSocket
    this.connectWS(sessionId);

    return instance;
  }

  connectWS(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;
    if (instance.ws && (instance.ws.readyState === WebSocket.OPEN || instance.ws.readyState === WebSocket.CONNECTING)) return;

    const { term } = instance;
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const host = window.location.host;
    const wsUrl = `${proto}://${host}/api/v1/terminal/ws/${sessionId}`;
    instance.wsUrl = wsUrl;

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';
    instance.ws = ws;

    ws.onopen = () => {
      instance.reconnectAttempts = 0;
      try { instance.fitAddon.fit(); } catch { /* ignore */ }
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
      // Grace period for initial shell prompt output
      instance.initialGrace = true;
      setTimeout(() => { instance.initialGrace = false; }, 2000);
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
        // Track activity for busy indicator
        if (!instance.initialGrace) {
          this.markBusy(sessionId, instance);
        }
      } else {
        try {
          const msg = JSON.parse(event.data as string);
          if (msg.type === 'exit') {
            instance.onExit?.(sessionId);
          }
        } catch {
          term.write(event.data as string);
        }
      }
    };

    ws.onerror = () => {
      // Error will be followed by onclose, reconnect handled there
    };

    ws.onclose = () => {
      if (instance.intentionalClose) return;
      instance.ws = null;
      this.scheduleReconnect(sessionId);
    };

    // Wire up terminal input -> WS
    if (instance.dataDisposable) instance.dataDisposable.dispose();
    instance.dataDisposable = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(encoder.encode(data));
      }
    });

    if (instance.binaryDisposable) instance.binaryDisposable.dispose();
    instance.binaryDisposable = term.onBinary((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        const buffer = new Uint8Array(data.length);
        for (let i = 0; i < data.length; i++) {
          buffer[i] = data.charCodeAt(i) & 0xff;
        }
        ws.send(buffer);
      }
    });
  }

  private scheduleReconnect(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance || instance.intentionalClose) return;
    if (instance.reconnectTimer) return; // already scheduled

    const delay = Math.min(1000 * Math.pow(2, instance.reconnectAttempts), 30000);
    instance.reconnectAttempts++;

    instance.reconnectTimer = setTimeout(() => {
      instance.reconnectTimer = null;
      if (instance.intentionalClose) return;
      this.connectWS(sessionId);
    }, delay);
  }

  ensureConnected(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;
    if (instance.ws && instance.ws.readyState === WebSocket.OPEN) return;
    // Cancel any pending reconnect and connect immediately
    if (instance.reconnectTimer) {
      clearTimeout(instance.reconnectTimer);
      instance.reconnectTimer = null;
    }
    instance.ws = null;
    this.connectWS(sessionId);
  }

  reconnectAllIfNeeded(): void {
    for (const [sessionId, instance] of this.instances) {
      if (instance.intentionalClose) continue;
      if (!instance.ws || instance.ws.readyState !== WebSocket.OPEN) {
        this.ensureConnected(sessionId);
      }
    }
  }

  disconnectWS(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;
    instance.intentionalClose = true;
    if (instance.reconnectTimer) {
      clearTimeout(instance.reconnectTimer);
      instance.reconnectTimer = null;
    }
    if (instance.ws && (instance.ws.readyState === WebSocket.OPEN || instance.ws.readyState === WebSocket.CONNECTING)) {
      instance.ws.close();
    }
    instance.ws = null;
    if (instance.dataDisposable) { instance.dataDisposable.dispose(); instance.dataDisposable = null; }
    if (instance.binaryDisposable) { instance.binaryDisposable.dispose(); instance.binaryDisposable = null; }
  }

  attach(sessionId: string, container: HTMLElement): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;

    // Reparent the terminal's container div
    if (instance.containerEl.parentElement !== container) {
      container.appendChild(instance.containerEl);
    }

    // Set up ResizeObserver
    if (instance.resizeObserver) {
      instance.resizeObserver.disconnect();
    }

    instance.resizeObserver = new ResizeObserver(() => {
      this.debouncedFit(sessionId);
    });
    instance.resizeObserver.observe(container);

    // Set up drag-and-drop and image paste
    this.setupDragDrop(sessionId, container);
    this.setupImagePaste(sessionId);
  }

  detach(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;

    // Disconnect resize observer
    if (instance.resizeObserver) {
      instance.resizeObserver.disconnect();
      instance.resizeObserver = null;
    }

    // Clear pending fit timer
    if (instance.fitTimer) {
      clearTimeout(instance.fitTimer);
      instance.fitTimer = null;
    }

    // Clean up drag-and-drop listeners
    const parent = instance.containerEl.parentElement;
    if (parent) {
      dragCleanupMap.get(parent)?.();
      dragCleanupMap.delete(parent);
    }
    instance.dropOverlay = null;

    // Clean up paste listener
    if (instance.pasteDisposable) {
      instance.pasteDisposable.dispose();
      instance.pasteDisposable = null;
    }

    // Remove from parent but keep alive
    if (parent) {
      parent.removeChild(instance.containerEl);
    }
  }

  private markBusy(sessionId: string, instance: TerminalInstance): void {
    if (!instance.isBusy) {
      instance.isBusy = true;
      this.onBusyChange?.(sessionId, true);
    }
    if (instance.busyTimer) clearTimeout(instance.busyTimer);
    instance.busyTimer = setTimeout(() => {
      instance.isBusy = false;
      instance.busyTimer = null;
      this.onBusyChange?.(sessionId, false);
    }, 1500);
  }

  private debouncedFit(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;

    if (instance.fitTimer) clearTimeout(instance.fitTimer);
    instance.fitTimer = setTimeout(() => {
      try {
        instance.fitAddon.fit();
      } catch {
        return;
      }
      if (instance.ws?.readyState === WebSocket.OPEN) {
        instance.ws.send(JSON.stringify({
          type: 'resize',
          cols: instance.term.cols,
          rows: instance.term.rows,
        }));
      }
    }, 100);
  }

  fit(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;
    try { instance.fitAddon.fit(); } catch { /* ignore */ }
    if (instance.ws?.readyState === WebSocket.OPEN) {
      instance.ws.send(JSON.stringify({
        type: 'resize',
        cols: instance.term.cols,
        rows: instance.term.rows,
      }));
    }
  }

  focus(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;
    instance.term.focus();
  }

  setOnExit(sessionId: string, callback: ((sessionId: string) => void) | null): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;
    instance.onExit = callback;
  }

  release(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;

    if (instance.busyTimer) clearTimeout(instance.busyTimer);
    if (instance.reconnectTimer) clearTimeout(instance.reconnectTimer);
    if (instance.isBusy) this.onBusyChange?.(sessionId, false);
    this.detach(sessionId);
    this.disconnectWS(sessionId);
    instance.term.dispose();
    this.instances.delete(sessionId);
  }

  releaseAll(sessionIds: string[]): void {
    for (const id of sessionIds) {
      this.release(id);
    }
  }

  has(sessionId: string): boolean {
    return this.instances.has(sessionId);
  }

  private writeToTerminal(sessionId: string, text: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance?.ws || instance.ws.readyState !== WebSocket.OPEN) return;
    instance.ws.send(encoder.encode(text));
  }

  private shellEscape(path: string): string {
    if (/^[\w./@:-]+$/.test(path)) return path;
    return "'" + path.replace(/'/g, "'\\''") + "'";
  }

  private createDropOverlay(container: HTMLElement): HTMLDivElement {
    const overlay = document.createElement('div');
    overlay.style.cssText = `
      position: absolute; inset: 0; z-index: 50;
      display: flex; align-items: center; justify-content: center;
      background: rgba(0,0,0,0.6); backdrop-filter: blur(4px);
      border: 2px dashed var(--op-accent, #E84BA5);
      border-radius: 8px; pointer-events: none; opacity: 0;
      transition: opacity 150ms ease;
    `;
    const label = document.createElement('span');
    label.style.cssText = `
      color: var(--op-accent-text, #F472B6);
      font-size: 14px; font-weight: 600; font-family: inherit;
    `;
    label.textContent = 'Drop to insert path';
    overlay.appendChild(label);
    container.style.position = 'relative';
    container.appendChild(overlay);
    return overlay;
  }

  private setupDragDrop(sessionId: string, container: HTMLElement): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;

    if (instance.dropOverlay) {
      instance.dropOverlay.remove();
    }
    const overlay = this.createDropOverlay(container);
    instance.dropOverlay = overlay;

    let dragCounter = 0;

    const onDragEnter = (e: DragEvent) => {
      e.preventDefault();
      dragCounter++;
      if (e.dataTransfer?.types.includes('Files')) {
        overlay.style.opacity = '1';
      }
    };

    const onDragOver = (e: DragEvent) => {
      e.preventDefault();
      if (e.dataTransfer) {
        e.dataTransfer.dropEffect = 'copy';
      }
    };

    const onDragLeave = (e: DragEvent) => {
      e.preventDefault();
      dragCounter--;
      if (dragCounter <= 0) {
        dragCounter = 0;
        overlay.style.opacity = '0';
      }
    };

    const onDrop = async (e: DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      dragCounter = 0;
      overlay.style.opacity = '0';

      if (!e.dataTransfer) return;

      const paths: string[] = [];
      const items = e.dataTransfer.items;
      const files = e.dataTransfer.files;

      if (items.length > 0 || files.length > 0) {
        const labelEl = overlay.querySelector('span')!;
        labelEl.textContent = 'Resolving...';
        overlay.style.opacity = '1';

        for (let i = 0; i < items.length; i++) {
          try {
            const entry = items[i]?.webkitGetAsEntry?.();
            if (entry?.isDirectory) {
              // Resolve directory path via server-side Spotlight/fallback
              const result = await terminalApi.resolvePath(entry.name, true);
              paths.push(result.path);
            } else if (files[i]) {
              const result = await terminalApi.upload(files[i]);
              paths.push(result.path);
            }
          } catch {
            // Skip failed items
          }
        }

        overlay.style.opacity = '0';
        labelEl.textContent = 'Drop to insert path';
      }

      if (paths.length > 0) {
        const escaped = paths.map(p => this.shellEscape(p)).join(' ');
        this.writeToTerminal(sessionId, escaped);
        instance.term.focus();
      }
    };

    container.addEventListener('dragenter', onDragEnter);
    container.addEventListener('dragover', onDragOver);
    container.addEventListener('dragleave', onDragLeave);
    container.addEventListener('drop', onDrop);

    dragCleanupMap.set(container, () => {
      container.removeEventListener('dragenter', onDragEnter);
      container.removeEventListener('dragover', onDragOver);
      container.removeEventListener('dragleave', onDragLeave);
      container.removeEventListener('drop', onDrop);
      overlay.remove();
    });
  }

  private setupImagePaste(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;

    if (instance.pasteDisposable) {
      instance.pasteDisposable.dispose();
    }

    const textarea = instance.containerEl.querySelector('.xterm-helper-textarea') as HTMLTextAreaElement | null;
    if (!textarea) return;

    const onPaste = async (e: ClipboardEvent) => {
      if (!e.clipboardData) return;

      const items = e.clipboardData.items;
      const imageItems: DataTransferItem[] = [];

      for (let i = 0; i < items.length; i++) {
        if (items[i].type.startsWith('image/')) {
          imageItems.push(items[i]);
        }
      }

      if (imageItems.length === 0) return;

      // Prevent xterm from handling the paste (it would insert garbled binary)
      e.preventDefault();
      e.stopPropagation();

      const paths: string[] = [];
      for (const item of imageItems) {
        const blob = item.getAsFile();
        if (!blob) continue;

        const ext = blob.type.split('/')[1] || 'png';
        const file = new File([blob], `pasted-image.${ext}`, { type: blob.type });

        try {
          const result = await terminalApi.upload(file);
          paths.push(result.path);
        } catch {
          // Skip failed uploads
        }
      }

      if (paths.length > 0) {
        const escaped = paths.map(p => this.shellEscape(p)).join(' ');
        this.writeToTerminal(sessionId, escaped);
        instance.term.focus();
      }
    };

    textarea.addEventListener('paste', onPaste, true);
    instance.pasteDisposable = {
      dispose: () => textarea.removeEventListener('paste', onPaste, true),
    };
  }

  refreshThemes(): void {
    const theme = buildTheme();
    for (const instance of this.instances.values()) {
      instance.term.options.theme = theme;
    }
  }
}

export const terminalManager = new TerminalManager();
