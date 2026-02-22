import { useEffect } from 'react';
import { useOutletContext } from 'react-router';
import type { TocItem } from '../../components/docs/DocsTableOfContents';

interface DocsContext {
  registerToc: (items: TocItem[]) => void;
}

export function GetStarted() {
  const { registerToc } = useOutletContext<DocsContext>();

  useEffect(() => {
    registerToc([
      { id: 'requirements', text: 'Requirements', level: 2 },
      { id: 'installation', text: 'Installation', level: 2 },
      { id: 'quick-install', text: 'Quick Install', level: 3 },
      { id: 'from-source', text: 'From Source', level: 3 },
      { id: 'desktop-app', text: 'Desktop App', level: 3 },
      { id: 'setup-wizard', text: 'Setup Wizard', level: 2 },
      { id: 'first-chat', text: 'Your First Chat', level: 2 },
      { id: 'next-steps', text: 'Next Steps', level: 2 },
    ]);
  }, [registerToc]);

  return (
    <>
      <h1>Get Started</h1>
      <p className="text-lg text-text-2 mb-8">
        Get OpenPaw running in under a minute. This guide covers installation, the setup wizard,
        and sending your first message.
      </p>

      <h2 id="requirements">Requirements</h2>
      <ul>
        <li><strong>Operating System</strong> — macOS, Linux, or Windows</li>
        <li><strong>Anthropic API Key</strong> — required for AI functionality (you can also set this during setup)</li>
        <li><strong>Disk Space</strong> — ~50 MB for the binary, plus space for your SQLite database</li>
      </ul>
      <p>
        OpenPaw is a single binary with no external dependencies. SQLite is embedded, and the
        React frontend is compiled into the binary at build time.
      </p>

      <h2 id="installation">Installation</h2>

      <h3 id="quick-install">Quick Install</h3>
      <p>The fastest way to get started:</p>
      <pre><code>{`# Download and install
curl -fsSL https://get.openpaw.dev | sh

# Start the server
./openpaw`}</code></pre>
      <p>
        OpenPaw will start on <code>http://localhost:41295</code> by default. The port can be
        changed with the <code>--port</code> flag.
      </p>

      <h3 id="from-source">From Source</h3>
      <p>If you prefer to build from source, you&apos;ll need Go 1.25+ and Node.js 18+:</p>
      <pre><code>{`git clone https://github.com/OpenPaw/openpaw.git
cd openpaw

# Install frontend dependencies and build
cd web/frontend && npm install && npm run build && cd ../..

# Build the Go binary (CGO required for SQLite)
CGO_ENABLED=1 go build -o openpaw ./cmd/openpaw

# Run it
./openpaw`}</code></pre>

      <h3 id="desktop-app">Desktop App</h3>
      <p>
        OpenPaw also ships as a native desktop application built with Tauri. Download the latest
        release for your platform from the{' '}
        <a href="https://github.com/OpenPaw/openpaw/releases" target="_blank" rel="noopener noreferrer">
          releases page
        </a>.
      </p>
      <p>
        See the <a href="/docs/desktop">Desktop App</a> docs for more details on the Tauri integration.
      </p>

      <h2 id="setup-wizard">Setup Wizard</h2>
      <p>
        On first launch, OpenPaw shows a setup wizard that guides you through:
      </p>
      <ol>
        <li><strong>Create an admin account</strong> — username and password for the web interface</li>
        <li><strong>Configure your API key</strong> — enter your Anthropic API key (or set it as an environment variable)</li>
        <li><strong>Choose a theme</strong> — pick an accent color and light/dark mode</li>
      </ol>
      <p>
        After completing setup, you&apos;ll land on the dashboard where you can start exploring agents,
        tools, and chats.
      </p>

      <h2 id="first-chat">Your First Chat</h2>
      <p>Once setup is complete:</p>
      <ol>
        <li>Navigate to <strong>Chats</strong> in the sidebar</li>
        <li>Click <strong>New Chat</strong> to create a thread</li>
        <li>Type a message and press Enter</li>
        <li>Watch the AI agent respond in real-time via WebSocket streaming</li>
      </ol>
      <p>
        By default, the <strong>General Assistant</strong> agent handles your messages. You can
        mention other agents with <code>@agent-name</code> to bring them into the conversation.
      </p>

      <h2 id="next-steps">Next Steps</h2>
      <ul>
        <li><a href="/docs/how-it-works">How It Works</a> — understand the architecture</li>
        <li><a href="/docs/features">Features</a> — explore all capabilities</li>
        <li><a href="/docs/use-cases">Use Cases</a> — real-world workflows and examples</li>
      </ul>
    </>
  );
}
