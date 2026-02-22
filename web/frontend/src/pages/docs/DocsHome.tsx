import { useEffect } from 'react';
import { Link, useOutletContext } from 'react-router';
import {
  Rocket,
  Cpu,
  LayoutGrid,
  Monitor,
  Lightbulb,
  Building2,
  Bot,
  Wrench,
  Sparkles,
  BookOpen,
  Clock,
  Heart,
  MessageSquare,
  Shield,
  Globe,
  Zap,
} from 'lucide-react';
import type { TocItem } from '../../components/docs/DocsTableOfContents';

interface DocsContext {
  registerToc: (items: TocItem[]) => void;
}

const quickLinks = [
  { label: 'Get Started', href: '/docs/get-started', icon: Rocket, description: 'Install and set up OpenPaw in minutes' },
  { label: 'How It Works', href: '/docs/how-it-works', icon: Cpu, description: 'Understand the architecture and agent lifecycle' },
  { label: 'Features', href: '/docs/features', icon: LayoutGrid, description: 'Explore all capabilities' },
  { label: 'Desktop App', href: '/docs/desktop', icon: Monitor, description: 'Native desktop experience with Tauri' },
  { label: 'Use Cases', href: '/docs/use-cases', icon: Lightbulb, description: 'Real-world examples and workflows' },
  { label: 'Architecture', href: '/docs/architecture', icon: Building2, description: 'Technical deep dive' },
];

const keyFeatures = [
  { icon: Bot, label: 'AI Agents', description: 'Multi-agent system with customizable roles and prompts' },
  { icon: Wrench, label: 'Tool Library', description: 'Install and manage tools from the built-in catalog' },
  { icon: Sparkles, label: 'Skills', description: 'Reusable prompt templates for common tasks' },
  { icon: BookOpen, label: 'Context System', description: 'Upload files and knowledge for agents to reference' },
  { icon: Clock, label: 'Scheduler', description: 'Cron-based automation for recurring tasks' },
  { icon: Heart, label: 'Heartbeat', description: 'Proactive agent check-ins on a schedule' },
  { icon: MessageSquare, label: 'Chat', description: 'Threaded conversations with multiple agents' },
  { icon: Shield, label: 'Secrets', description: 'Encrypted credential management for tools' },
  { icon: Globe, label: 'Browser Automation', description: 'AI-driven web browsing and data extraction' },
  { icon: Zap, label: 'Real-time', description: 'WebSocket streaming for live agent output' },
];

export function DocsHome() {
  const { registerToc } = useOutletContext<DocsContext>();

  useEffect(() => {
    registerToc([
      { id: 'what-is-openpaw', text: 'What is OpenPaw?', level: 2 },
      { id: 'quick-start', text: 'Quick Start', level: 2 },
      { id: 'key-features', text: 'Key Features', level: 2 },
      { id: 'explore-docs', text: 'Explore the Docs', level: 2 },
    ]);
  }, [registerToc]);

  return (
    <>
      <h1>OpenPaw Documentation</h1>
      <p className="text-lg text-text-2 mb-8">
        Build your own AI-powered assistant factory. OpenPaw is a self-hosted platform for managing
        AI agents, tools, and automations — all from a single Go binary.
      </p>

      <h2 id="what-is-openpaw">What is OpenPaw?</h2>
      <p>
        OpenPaw is an <strong>agentic factory</strong> — a single-binary application that lets you create,
        configure, and orchestrate AI agents. Each agent can have its own personality, tools, and
        capabilities. Connect them to external APIs, schedule recurring tasks, and manage everything
        through a beautiful web interface.
      </p>
      <p>
        Unlike cloud-only AI platforms, OpenPaw runs entirely on your hardware. Your data stays
        local, your conversations are private, and you have full control over every aspect of the system.
      </p>

      <h2 id="quick-start">Quick Start</h2>
      <p>Get running in under a minute:</p>
      <pre><code>{`# Download the latest release
curl -fsSL https://get.openpaw.dev | sh

# Start OpenPaw
./openpaw

# Open http://localhost:41295 and follow the setup wizard`}</code></pre>
      <p>
        For more installation options, see the{' '}
        <Link to="/docs/get-started" className="text-accent-text hover:text-accent-hover underline">
          Get Started guide
        </Link>.
      </p>

      <h2 id="key-features">Key Features</h2>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 not-prose my-4">
        {keyFeatures.map((f) => (
          <div key={f.label} className="flex items-start gap-3 p-3 rounded-lg border border-border-0 bg-surface-1/50">
            <f.icon className="w-5 h-5 text-accent-text shrink-0 mt-0.5" />
            <div>
              <p className="text-sm font-medium text-text-0">{f.label}</p>
              <p className="text-xs text-text-2 mt-0.5">{f.description}</p>
            </div>
          </div>
        ))}
      </div>

      <h2 id="explore-docs">Explore the Docs</h2>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 not-prose my-4">
        {quickLinks.map((link) => (
          <Link
            key={link.href}
            to={link.href}
            className="group flex items-start gap-3 p-4 rounded-xl border border-border-0 bg-surface-1/50 hover:border-accent-primary/30 hover:bg-accent-muted/30 transition-colors"
          >
            <link.icon className="w-5 h-5 text-accent-text shrink-0 mt-0.5" />
            <div>
              <p className="text-sm font-semibold text-text-0 group-hover:text-accent-text transition-colors">
                {link.label}
              </p>
              <p className="text-xs text-text-2 mt-0.5">{link.description}</p>
            </div>
          </Link>
        ))}
      </div>
    </>
  );
}
