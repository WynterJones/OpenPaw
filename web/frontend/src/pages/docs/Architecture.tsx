import { useEffect } from 'react';
import { useOutletContext } from 'react-router';
import type { TocItem } from '../../components/docs/DocsTableOfContents';
import {
  Diagram,
  DiagramBox,
  DiagramArrow,
  DiagramStack,
  DiagramRow,
} from '../../components/docs/DocsDiagram';

interface DocsContext {
  registerToc: (items: TocItem[]) => void;
}

export function Architecture() {
  const { registerToc } = useOutletContext<DocsContext>();

  useEffect(() => {
    registerToc([
      { id: 'tech-stack', text: 'Tech Stack', level: 2 },
      { id: 'project-structure', text: 'Project Structure', level: 2 },
      { id: 'build-pipeline', text: 'Build Pipeline', level: 2 },
      { id: 'database', text: 'Database', level: 2 },
      { id: 'api-overview', text: 'API Overview', level: 2 },
      { id: 'websocket-protocol', text: 'WebSocket Protocol', level: 2 },
      { id: 'security-model', text: 'Security Model', level: 2 },
      { id: 'design-system', text: 'Design System', level: 2 },
    ]);
  }, [registerToc]);

  return (
    <>
      <h1>Architecture</h1>
      <p className="text-lg text-text-2 mb-8">
        A technical deep dive into OpenPaw&apos;s architecture, from the build pipeline to the
        database schema to the real-time protocol.
      </p>

      <h2 id="tech-stack">Tech Stack</h2>
      <table>
        <thead>
          <tr>
            <th>Layer</th>
            <th>Technology</th>
            <th>Location</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Backend</td>
            <td>Go 1.25 + chi router + SQLite</td>
            <td><code>cmd/openpaw/</code>, <code>internal/</code></td>
          </tr>
          <tr>
            <td>Frontend</td>
            <td>React 19 + TypeScript 5.9 + Tailwind v4</td>
            <td><code>web/frontend/src/</code></td>
          </tr>
          <tr>
            <td>Embedding</td>
            <td><code>go:embed all:frontend/dist</code></td>
            <td><code>web/embed.go</code></td>
          </tr>
          <tr>
            <td>AI Engine</td>
            <td>Claude API via HTTP</td>
            <td><code>internal/agents/</code></td>
          </tr>
          <tr>
            <td>Database</td>
            <td>SQLite (WAL mode)</td>
            <td><code>internal/database/</code></td>
          </tr>
          <tr>
            <td>Desktop</td>
            <td>Tauri (Rust)</td>
            <td><code>desktop/</code></td>
          </tr>
        </tbody>
      </table>

      <h2 id="project-structure">Project Structure</h2>
      <pre><code>{`openpaw/
├── cmd/openpaw/          # Main entry point
├── internal/
│   ├── agents/           # AI agent execution engine
│   ├── database/         # SQLite connection + migrations
│   │   └── migrations/   # Numbered SQL migration files
│   ├── handlers/         # HTTP route handlers
│   ├── models/           # Data structures
│   ├── server/           # HTTP server setup + middleware
│   ├── toollibrary/      # Tool catalog and management
│   └── skilllibrary/     # Skill catalog
├── web/
│   ├── embed.go          # go:embed for frontend dist
│   └── frontend/
│       ├── src/
│       │   ├── components/   # Reusable UI components
│       │   ├── contexts/     # React contexts (Auth, Design)
│       │   ├── lib/          # API client, utils, types
│       │   └── pages/        # Page-level components
│       └── dist/             # Built frontend (embedded)
├── desktop/              # Tauri desktop app
└── data/                 # SQLite database (runtime)`}</code></pre>

      <h2 id="build-pipeline">Build Pipeline</h2>
      <p>
        The frontend is embedded into the Go binary via <code>go:embed</code>. This means the
        frontend must be built <strong>before</strong> the Go binary:
      </p>

      <Diagram title="Build Pipeline">
        <DiagramStack>
          <DiagramRow>
            <DiagramBox variant="primary">TypeScript + React Source</DiagramBox>
            <DiagramBox variant="muted">Go Source</DiagramBox>
          </DiagramRow>
          <DiagramArrow direction="down" label="vite build" />
          <DiagramBox variant="accent">web/frontend/dist/</DiagramBox>
          <DiagramArrow direction="down" label="go:embed" />
          <DiagramBox variant="primary">Single Binary (openpaw)</DiagramBox>
        </DiagramStack>
      </Diagram>

      <pre><code>{`# Full build pipeline
cd web/frontend && npm run build    # React → dist/
CGO_ENABLED=1 go build -o openpaw   # Go + embed → binary`}</code></pre>

      <h2 id="database">Database</h2>
      <p>
        OpenPaw uses <strong>SQLite</strong> in WAL (Write-Ahead Logging) mode for the best
        balance of read/write performance. The database file lives at <code>./data/openpaw.db</code>.
      </p>
      <h3>Migrations</h3>
      <p>
        Migrations are numbered SQL files in <code>internal/database/migrations/</code>. They
        run automatically on startup and are tracked in a <code>schema_migrations</code> table.
      </p>
      <pre><code>{`internal/database/migrations/
├── 001_init.sql
├── 002_tools.sql
├── 003_secrets.sql
├── ...
└── 023_agent_library.sql`}</code></pre>
      <h3>Key Tables</h3>
      <table>
        <thead>
          <tr>
            <th>Table</th>
            <th>Purpose</th>
          </tr>
        </thead>
        <tbody>
          <tr><td><code>users</code></td><td>User accounts and authentication</td></tr>
          <tr><td><code>agent_roles</code></td><td>Agent configurations and system prompts</td></tr>
          <tr><td><code>threads</code></td><td>Chat conversation threads</td></tr>
          <tr><td><code>messages</code></td><td>Chat messages with cost/token metadata</td></tr>
          <tr><td><code>tools</code></td><td>Installed tools and their status</td></tr>
          <tr><td><code>secrets</code></td><td>Encrypted API keys and credentials</td></tr>
          <tr><td><code>schedules</code></td><td>Cron jobs and scheduled tasks</td></tr>
          <tr><td><code>audit_logs</code></td><td>Full activity audit trail</td></tr>
        </tbody>
      </table>

      <h2 id="api-overview">API Overview</h2>
      <p>
        All API routes are prefixed with <code>/api</code> and require JWT authentication
        (except <code>/api/auth/*</code> and <code>/api/system/health</code>).
      </p>
      <table>
        <thead>
          <tr>
            <th>Group</th>
            <th>Routes</th>
            <th>Purpose</th>
          </tr>
        </thead>
        <tbody>
          <tr><td><code>/api/auth</code></td><td>POST login, setup, logout</td><td>Authentication</td></tr>
          <tr><td><code>/api/agents</code></td><td>CRUD</td><td>Agent management</td></tr>
          <tr><td><code>/api/threads</code></td><td>CRUD + messages</td><td>Chat threads</td></tr>
          <tr><td><code>/api/tools</code></td><td>CRUD + start/stop</td><td>Tool management</td></tr>
          <tr><td><code>/api/secrets</code></td><td>CRUD</td><td>Secrets vault</td></tr>
          <tr><td><code>/api/schedules</code></td><td>CRUD + trigger</td><td>Scheduled tasks</td></tr>
          <tr><td><code>/api/settings</code></td><td>GET/PUT</td><td>App configuration</td></tr>
          <tr><td><code>/api/ws</code></td><td>WebSocket</td><td>Real-time streaming</td></tr>
        </tbody>
      </table>

      <h2 id="websocket-protocol">WebSocket Protocol</h2>
      <p>
        The WebSocket connection at <code>/api/ws</code> handles all real-time communication.
        Messages are JSON-encoded with a <code>type</code> and <code>payload</code> structure:
      </p>
      <pre><code>{`// Client → Server
{
  "type": "chat_message",
  "payload": {
    "thread_id": "uuid",
    "content": "Hello!",
    "agent_id": "general"
  }
}

// Server → Client (streaming)
{ "type": "stream", "payload": { "event": { "type": "text_delta", "text": "Hi" } } }
{ "type": "stream", "payload": { "event": { "type": "tool_start", "tool_name": "weather" } } }
{ "type": "stream", "payload": { "event": { "type": "turn_complete" } } }
{ "type": "stream", "payload": { "event": { "type": "result", "total_cost_usd": 0.003 } } }`}</code></pre>

      <h2 id="security-model">Security Model</h2>
      <ul>
        <li><strong>Authentication</strong> — JWT tokens stored in HttpOnly cookies</li>
        <li><strong>CSRF Protection</strong> — double-submit cookie pattern for state-changing requests</li>
        <li><strong>Secrets Encryption</strong> — all secrets encrypted at rest in SQLite</li>
        <li><strong>Tool Integrity</strong> — SHA-256 hashes verify tool binaries haven&apos;t been modified</li>
        <li><strong>Rate Limiting</strong> — configurable per-endpoint rate limits</li>
        <li><strong>Audit Logging</strong> — every significant action is logged with user, action, and target</li>
      </ul>

      <h2 id="design-system">Design System</h2>
      <p>
        OpenPaw uses an <strong>OKLCH-based color system</strong> with CSS custom properties.
        All colors are generated from an accent hue and applied via <code>var(--op-*)</code> tokens:
      </p>
      <table>
        <thead>
          <tr>
            <th>Token</th>
            <th>Purpose</th>
          </tr>
        </thead>
        <tbody>
          <tr><td><code>--op-surface-0..3</code></td><td>Background layers (darkest to lightest)</td></tr>
          <tr><td><code>--op-border-0..1</code></td><td>Border colors</td></tr>
          <tr><td><code>--op-text-0..3</code></td><td>Text colors (brightest to dimmest)</td></tr>
          <tr><td><code>--op-accent</code></td><td>Primary accent color</td></tr>
          <tr><td><code>--op-accent-text</code></td><td>Accent color for text</td></tr>
          <tr><td><code>--op-accent-muted</code></td><td>Accent color at low opacity</td></tr>
        </tbody>
      </table>
      <p>
        The theme is controlled by the <code>DesignContext</code> which generates all token values
        from a single accent color and light/dark mode selection.
      </p>
    </>
  );
}
