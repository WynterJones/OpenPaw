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
  LayoutDashboard,
  Monitor,
  Globe,
  Zap,
  Bell,
  FileText,
  Palette,
  Shield,
} from 'lucide-react';

interface DocsContext {
  registerToc: (items: TocItem[]) => void;
}

const features = [
  {
    icon: Bot,
    name: 'AI Agents',
    id: 'ai-agents',
    description: 'Create multiple AI agents with unique personalities, system prompts, and tool access. Each agent can specialize in different tasks — from coding to research to customer support.',
  },
  {
    icon: MessageSquare,
    name: 'Threaded Chat',
    id: 'threaded-chat',
    description: 'Have conversations in organized threads. Mention agents with @name to bring them into the conversation. Full markdown support with code highlighting.',
  },
  {
    icon: Wrench,
    name: 'Tool System',
    id: 'tool-system',
    description: 'Install tools from the built-in library or create custom ones. Tools are compiled Go binaries with HTTP endpoints that agents can call during conversations.',
  },
  {
    icon: Sparkles,
    name: 'Skills',
    id: 'skills',
    description: 'Reusable prompt templates that agents can use for common tasks. Install from the library or create your own with custom tool access permissions.',
  },
  {
    icon: BookOpen,
    name: 'Context System',
    id: 'context-system',
    description: 'Upload documents, files, and knowledge that agents can reference. Organize into folders and mark files as "About You" for personal context.',
  },
  {
    icon: KeyRound,
    name: 'Secrets Management',
    id: 'secrets',
    description: 'Securely store API keys and credentials. Secrets are encrypted at rest and scoped to specific tools, preventing unauthorized access.',
  },
  {
    icon: Clock,
    name: 'Scheduler',
    id: 'scheduler',
    description: 'Set up cron-based automation for recurring tasks. Schedule tool actions or agent prompts to run on any interval — hourly, daily, or custom cron expressions.',
  },
  {
    icon: Heart,
    name: 'Heartbeat Monitor',
    id: 'heartbeat',
    description: 'Enable proactive agent check-ins on a schedule. Agents can monitor systems, check for updates, and report back automatically within configured active hours.',
  },
  {
    icon: LayoutDashboard,
    name: 'Dashboards',
    id: 'dashboards',
    description: 'Build custom dashboards with widgets that pull data from tools. Metric cards, charts, tables, and more — all configurable and real-time.',
  },
  {
    icon: Monitor,
    name: 'Browser Automation',
    id: 'browser-automation',
    description: 'AI-driven web browsing with Playwright. Agents can navigate pages, extract data, fill forms, and take screenshots — with a live viewer in the UI.',
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
    description: 'JWT authentication with HttpOnly cookies, CSRF protection, encrypted secrets, tool integrity verification, and rate limiting.',
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
