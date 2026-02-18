import { useEffect } from 'react';
import { useOutletContext } from 'react-router';
import type { TocItem } from '../../components/docs/DocsTableOfContents';
import { Diagram, DiagramBox, DiagramArrow, DiagramStack, DiagramRow } from '../../components/docs/DocsDiagram';

interface DocsContext {
  registerToc: (items: TocItem[]) => void;
}

export function DesktopApp() {
  const { registerToc } = useOutletContext<DocsContext>();

  useEffect(() => {
    registerToc([
      { id: 'overview', text: 'Overview', level: 2 },
      { id: 'platforms', text: 'Supported Platforms', level: 2 },
      { id: 'how-it-works', text: 'How It Works', level: 2 },
      { id: 'installation', text: 'Installation', level: 2 },
      { id: 'network-access', text: 'Network Access', level: 2 },
      { id: 'building', text: 'Building from Source', level: 2 },
    ]);
  }, [registerToc]);

  return (
    <>
      <h1>Desktop App</h1>
      <p className="text-lg text-text-2 mb-8">
        OpenPaw ships as a native desktop application built with Tauri, giving you a lightweight,
        system-native experience without the overhead of Electron.
      </p>

      <h2 id="overview">Overview</h2>
      <p>
        The desktop app wraps the same Go backend and React frontend into a native application
        using <strong>Tauri</strong>. Tauri uses the operating system&apos;s built-in WebView (WebKit on macOS,
        WebView2 on Windows, WebKitGTK on Linux) rather than bundling a full Chromium browser.
      </p>
      <p>
        This means the desktop app is <strong>significantly smaller</strong> than Electron-based
        alternatives — typically under 15 MB compared to 100+ MB.
      </p>

      <h2 id="platforms">Supported Platforms</h2>
      <table>
        <thead>
          <tr>
            <th>Platform</th>
            <th>Architecture</th>
            <th>Format</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>macOS</td>
            <td>Intel, Apple Silicon</td>
            <td><code>.dmg</code></td>
          </tr>
          <tr>
            <td>Windows</td>
            <td>x64</td>
            <td><code>.msi</code>, <code>.exe</code></td>
          </tr>
          <tr>
            <td>Linux</td>
            <td>x64</td>
            <td><code>.deb</code>, <code>.AppImage</code></td>
          </tr>
        </tbody>
      </table>

      <h2 id="how-it-works">How It Works</h2>
      <p>
        The desktop app embeds the Go server as a sidecar process. When you launch the app:
      </p>
      <ol>
        <li>Tauri starts the native window</li>
        <li>The Go binary is launched as a child process</li>
        <li>The WebView points to <code>localhost:41295</code></li>
        <li>The frontend communicates with the backend via the same REST/WebSocket APIs</li>
      </ol>

      <Diagram title="Desktop Architecture">
        <DiagramStack>
          <DiagramBox variant="primary">Tauri Native Window</DiagramBox>
          <DiagramArrow direction="down" />
          <DiagramRow>
            <DiagramBox variant="accent">System WebView</DiagramBox>
            <DiagramBox variant="muted">Go Sidecar Process</DiagramBox>
          </DiagramRow>
          <DiagramArrow direction="down" />
          <DiagramRow>
            <DiagramBox variant="muted">React Frontend</DiagramBox>
            <DiagramBox variant="muted">REST API + WebSocket</DiagramBox>
            <DiagramBox variant="muted">SQLite Database</DiagramBox>
          </DiagramRow>
        </DiagramStack>
      </Diagram>

      <h2 id="installation">Installation</h2>
      <p>
        Download the latest release for your platform from the{' '}
        <a href="https://github.com/OpenPaw/openpaw/releases" target="_blank" rel="noopener noreferrer">
          GitHub releases page
        </a>.
      </p>
      <h3>macOS</h3>
      <p>
        Open the <code>.dmg</code> file and drag OpenPaw to your Applications folder. On first
        launch, you may need to right-click and select &quot;Open&quot; to bypass Gatekeeper.
      </p>
      <h3>Windows</h3>
      <p>
        Run the <code>.msi</code> installer or launch the portable <code>.exe</code> directly.
        WebView2 is required (pre-installed on Windows 10/11).
      </p>
      <h3>Linux</h3>
      <p>
        Install the <code>.deb</code> package or run the <code>.AppImage</code>. WebKitGTK
        must be installed on your system.
      </p>

      <h2 id="network-access">Network Access</h2>
      <p>
        The desktop app runs locally by default. To access OpenPaw from other devices:
      </p>
      <ul>
        <li><strong>LAN access</strong> — enabled by default, accessible at your machine&apos;s IP on port 41295</li>
        <li><strong>Tailscale</strong> — when detected, OpenPaw automatically binds to your Tailscale IP for secure remote access</li>
      </ul>
      <p>
        The Settings page shows your current LAN IP and Tailscale IP (if available).
      </p>

      <h2 id="building">Building from Source</h2>
      <p>To build the desktop app from source:</p>
      <pre><code>{`# Prerequisites: Rust, Node.js, Go, and platform-specific deps

# Install Tauri CLI
cargo install tauri-cli

# Build the frontend
cd web/frontend && npm install && npm run build && cd ../..

# Build the Go binary
CGO_ENABLED=1 go build -o openpaw ./cmd/openpaw

# Build the Tauri app
cd desktop && cargo tauri build`}</code></pre>
    </>
  );
}
