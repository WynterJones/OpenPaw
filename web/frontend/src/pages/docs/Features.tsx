import { useEffect } from 'react';
import { useOutletContext } from 'react-router';
import type { TocItem } from '../../components/docs/DocsTableOfContents';
import {
  Bot,
  MessageSquare,
  Wrench,
  Sparkles,
  BookOpen,
  KeyRound,
  Clock,
  Heart,
  Moon,
  LayoutDashboard,
  Globe,
  Zap,
  Bell,
  FileText,
  Palette,
  Shield,
  Boxes,
  Database,
  TerminalSquare,
  Search,
} from 'lucide-react';

interface DocsContext {
  registerToc: (items: TocItem[]) => void;
}

const features = [
  {
    icon: Boxes,
    name: 'Workspaces',
    id: 'workspaces',
    description: 'Organize your work into isolated workspaces, each scoping its own chats, dashboards, context, tasks, and a real on-disk files directory shown in the Directory tab. Switch the active workspace from the sidebar, attach existing folders (like cloned repos) for agents to work in, and set a workspace image. Agents, services, and skills can each be shared across all workspaces or bound to a single one.',
  },
  {
    icon: Bot,
    name: 'AI Agents',
    id: 'ai-agents',
    description: 'Create specialist agents with unique identities, skills, and service access. Each agent can inherit the app default or use its own OpenRouter, Claude Code, or Codex provider.',
  },
  {
    icon: MessageSquare,
    name: 'Threaded Chat',
    id: 'threaded-chat',
    description: 'Have organized conversations with markdown, files, and @mentions. Branch any message into a resizable, Slack-style focused thread with its own context window; agent mentions inside it remain in the same thread.',
  },
  {
    icon: Wrench,
    name: 'Service System',
    id: 'service-system',
    description: 'Install services from the built-in library or create custom ones. Services are compiled Go binaries with HTTP endpoints that agents can call during conversations.',
  },
  {
    icon: Sparkles,
    name: 'Skills',
    id: 'skills',
    description: 'Reusable prompt templates that agents can use for common tasks. Install from the library or create your own with custom service access permissions.',
  },
  {
    icon: BookOpen,
    name: 'Context System',
    id: 'context-system',
    description: 'Organize reference files and durable documents that agents can read, create, and revise. Open a document beside chat in Canvas to work on it with AI while keeping unrelated chat threads out of that focused mode.',
  },
  {
    icon: Database,
    name: 'Workspace Databases',
    id: 'workspace-databases',
    description: 'Create Airtable-style structured databases with multiple tables, typed columns, sorting, resizing, pagination, CSV import/export, inline editing, and search. Agents and scheduled automations can fully manage rows and schema, while dashboards can use any table as a live data source.',
  },
  {
    icon: TerminalSquare,
    name: 'Terminal Workspace',
    id: 'terminal-workspace',
    description: 'Run persistent shells, dev servers, tmux sessions, Claude Code, and Codex inside OpenPaw. Terminals are global rather than tied to the selected project workspace, restore after relaunch, and blend with your custom app background.',
  },
  {
    icon: Search,
    name: 'Command Palette',
    id: 'command-palette',
    description: 'Jump to app screens with customizable keyboard shortcuts or type ! to fuzzy-search workspace files and folders. Open a result, copy its path, or insert the path into chat, Context, or a terminal.',
  },
  {
    icon: KeyRound,
    name: 'Secrets Management',
    id: 'secrets',
    description: 'Securely store API keys and credentials. Secrets are encrypted at rest and scoped to specific services, preventing unauthorized access.',
  },
  {
    icon: Clock,
    name: 'Scheduler',
    id: 'scheduler',
    description: 'Set up cron-based automation for recurring tasks. Schedule service actions or agent prompts to run on any interval — hourly, daily, or custom cron expressions.',
  },
  {
    icon: Heart,
    name: 'Heartbeat Monitor',
    id: 'heartbeat',
    description: 'Enable proactive agent check-ins on a schedule. Agents can monitor systems, check for updates, and report back automatically within configured active hours.',
  },
  {
    icon: Moon,
    name: 'Dreaming',
    id: 'dreaming',
    description: 'Agents remember. On a schedule, each agent re-reads the chats it has not read yet, pulls out anything durable — preferences, decisions, corrections — and consolidates it against what it already knows: merging duplicates, correcting what changed, dropping what no longer holds. Optionally, it can also capture from every reply as it happens.',
  },
  {
    icon: LayoutDashboard,
    name: 'Dashboards',
    id: 'dashboards',
    description: 'Build custom dashboards with widgets that pull live data from services or workspace databases. Metric cards, charts, tables, and more — all configurable and real-time.',
  },
  {
    icon: Globe,
    name: 'Network Access',
    id: 'network-access',
    description: 'Access OpenPaw from your local network or securely over the internet via Tailscale integration. Share your AI factory with trusted devices.',
  },
  {
    icon: Zap,
    name: 'Real-time Streaming',
    id: 'streaming',
    description: 'WebSocket-based streaming for instant feedback. See agent responses appear token-by-token, tool calls execute live, and status updates in real-time.',
  },
  {
    icon: Bell,
    name: 'Notifications',
    id: 'notifications',
    description: 'Push notifications for agent activity, scheduled task completions, and system events. Supports browser notifications and in-app alerts.',
  },
  {
    icon: FileText,
    name: 'Audit Logs',
    id: 'audit-logs',
    description: 'Full audit trail of all system activity. Track who did what, when, and how much it cost. Filter by category, action, or time range.',
  },
  {
    icon: Palette,
    name: 'Theming',
    id: 'theming',
    description: 'Customizable appearance with OKLCH color system. Choose an accent color, light or dark mode, custom fonts, and background images.',
  },
  {
    icon: Shield,
    name: 'Security',
    id: 'security',
    description: 'JWT authentication with HttpOnly cookies, CSRF protection, encrypted secrets, service integrity verification, and rate limiting.',
  },
];

export function Features() {
  const { registerToc } = useOutletContext<DocsContext>();

  useEffect(() => {
    registerToc([
      { id: 'overview', text: 'Overview', level: 2 },
      ...features.map(f => ({ id: f.id, text: f.name, level: 2 })),
    ]);
  }, [registerToc]);

  return (
    <>
      <h1>Features</h1>
      <p className="text-lg text-text-2 mb-8">
        OpenPaw ships with everything you need to build and manage an AI-powered assistant factory.
        Here&apos;s a comprehensive look at every feature.
      </p>

      <h2 id="overview">Overview</h2>
      <p>
        OpenPaw includes <strong>{features.length} core features</strong> that work together to
        create a complete AI agent platform. Each feature is accessible through the web interface
        and the REST API.
      </p>

      {features.map((feature) => (
        <div key={feature.id}>
          <h2 id={feature.id}>
            <span className="inline-flex items-center gap-2">
              <feature.icon className="w-5 h-5 text-accent-text" />
              {feature.name}
            </span>
          </h2>
          <p>{feature.description}</p>
        </div>
      ))}
    </>
  );
}
