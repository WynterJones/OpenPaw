import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import '@xterm/xterm/css/xterm.css';

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

class TerminalManager {
  private instances = new Map<string, TerminalInstance>();

  acquire(sessionId: string): TerminalInstance {
    let instance = this.instances.get(sessionId);
    if (instance) return instance;

    const theme = buildTheme();
    const term = new Terminal({
      scrollback: 5000,
      cursorBlink: true,
      fontSize: 14,
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
      try { instance.fitAddon.fit(); } catch { /* ignore */ }
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
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

    ws.onerror = () => {};
    ws.onclose = () => {};

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

  disconnectWS(sessionId: string): void {
    const instance = this.instances.get(sessionId);
    if (!instance) return;
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

    // Remove from parent but keep alive
    if (instance.containerEl.parentElement) {
      instance.containerEl.parentElement.removeChild(instance.containerEl);
    }
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

  refreshThemes(): void {
    const theme = buildTheme();
    for (const instance of this.instances.values()) {
      instance.term.options.theme = theme;
    }
  }
}

export const terminalManager = new TerminalManager();
