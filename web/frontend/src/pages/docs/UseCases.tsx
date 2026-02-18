import { useEffect } from 'react';
import { useOutletContext } from 'react-router';
import type { TocItem } from '../../components/docs/DocsTableOfContents';
import { User, Code, FileText, Search, Cog } from 'lucide-react';

interface DocsContext {
  registerToc: (items: TocItem[]) => void;
}

const useCases = [
  {
    id: 'personal-assistant',
    icon: User,
    title: 'Personal Assistant',
    description: 'Use OpenPaw as your private AI assistant that knows your preferences, schedule, and context.',
    examples: [
      'Upload your resume and personal docs to Context — agents will reference them automatically',
      'Set up a daily heartbeat to check your calendar and summarize upcoming events',
      'Create a "Personal" agent with a system prompt that knows your communication style',
      'Use the scheduler to send daily briefings at your preferred time',
    ],
    agents: ['General Assistant', 'Personal Agent'],
    tools: ['Calendar API', 'Email API', 'Weather API'],
  },
  {
    id: 'dev-workflow',
    icon: Code,
    title: 'Developer Workflow',
    description: 'Accelerate software development with AI-powered code review, documentation, and automation.',
    examples: [
      'Create a "Code Reviewer" agent with coding standards in its system prompt',
      'Install the GitHub tool to let agents create issues, review PRs, and check CI status',
      'Set up scheduled code quality reports that run nightly',
      'Use browser automation to monitor staging deployments',
    ],
    agents: ['Code Reviewer', 'DevOps Agent', 'Documentation Writer'],
    tools: ['GitHub', 'GitLab', 'Docker Hub'],
  },
  {
    id: 'content-creation',
    icon: FileText,
    title: 'Content Creation',
    description: 'Generate, edit, and manage content across multiple platforms with specialized agents.',
    examples: [
      'Create a "Blog Writer" agent with your brand voice in its system prompt',
      'Upload your style guide to Context so all agents follow your tone',
      'Schedule weekly content generation for social media posts',
      'Use a "Copy Editor" agent to review and polish drafts',
    ],
    agents: ['Blog Writer', 'Copy Editor', 'Social Media Manager'],
    tools: ['WordPress API', 'Notion API'],
  },
  {
    id: 'research',
    icon: Search,
    title: 'Research & Analysis',
    description: 'Conduct deep research, analyze data, and generate reports using multiple specialized agents.',
    examples: [
      'Create a "Research Analyst" agent with access to search and academic tools',
      'Install Semantic Scholar and ArXiv tools for academic paper access',
      'Set up automated literature reviews that run weekly',
      'Use browser automation to scrape and analyze competitor websites',
    ],
    agents: ['Research Analyst', 'Data Analyst'],
    tools: ['Semantic Scholar', 'ArXiv', 'Wikipedia', 'Brave Search'],
  },
  {
    id: 'automation',
    icon: Cog,
    title: 'Task Automation',
    description: 'Automate repetitive tasks with scheduled jobs, heartbeat monitoring, and tool integrations.',
    examples: [
      'Schedule a daily tool action to check system health and report via notifications',
      'Set up heartbeat monitoring so agents proactively check for issues',
      'Create workflows where one agent&apos;s output triggers another agent&apos;s action',
      'Use the browser automation to fill forms, download reports, or monitor dashboards',
    ],
    agents: ['Automation Agent', 'Monitor Agent'],
    tools: ['HTTP Client', 'Cron Scheduler', 'Notification API'],
  },
];

export function UseCases() {
  const { registerToc } = useOutletContext<DocsContext>();

  useEffect(() => {
    registerToc([
      { id: 'overview', text: 'Overview', level: 2 },
      ...useCases.map(u => ({ id: u.id, text: u.title, level: 2 })),
      { id: 'getting-started', text: 'Getting Started', level: 2 },
    ]);
  }, [registerToc]);

  return (
    <>
      <h1>Use Cases</h1>
      <p className="text-lg text-text-2 mb-8">
        OpenPaw is flexible enough for a wide range of workflows. Here are some common ways
        people use it, with practical examples for each.
      </p>

      <h2 id="overview">Overview</h2>
      <p>
        The combination of customizable agents, installable tools, scheduled automation, and
        browser control makes OpenPaw suitable for everything from personal productivity to
        team-wide workflow automation.
      </p>

      {useCases.map((useCase) => (
        <div key={useCase.id}>
          <h2 id={useCase.id}>
            <span className="inline-flex items-center gap-2">
              <useCase.icon className="w-5 h-5 text-accent-text" />
              {useCase.title}
            </span>
          </h2>
          <p>{useCase.description}</p>

          <h3>Examples</h3>
          <ul>
            {useCase.examples.map((example, i) => (
              <li key={i}>{example}</li>
            ))}
          </ul>

          <div className="flex flex-wrap gap-4 my-4 not-prose">
            <div>
              <p className="text-xs font-semibold text-text-3 uppercase tracking-wider mb-1.5">Suggested Agents</p>
              <div className="flex flex-wrap gap-1.5">
                {useCase.agents.map((agent) => (
                  <span key={agent} className="px-2 py-0.5 rounded-full bg-accent-muted text-accent-text text-xs font-medium">
                    {agent}
                  </span>
                ))}
              </div>
            </div>
            <div>
              <p className="text-xs font-semibold text-text-3 uppercase tracking-wider mb-1.5">Useful Tools</p>
              <div className="flex flex-wrap gap-1.5">
                {useCase.tools.map((tool) => (
                  <span key={tool} className="px-2 py-0.5 rounded-full bg-surface-2 text-text-2 text-xs font-medium border border-border-0">
                    {tool}
                  </span>
                ))}
              </div>
            </div>
          </div>
        </div>
      ))}

      <h2 id="getting-started">Getting Started</h2>
      <p>
        The best way to start is to pick the use case closest to your needs and set up one agent
        with the appropriate tools. As you get comfortable, add more agents and automate with
        the scheduler.
      </p>
      <p>
        See the <a href="/docs/get-started">Get Started</a> guide for installation, or
        the <a href="/docs/features">Features</a> page for a complete list of capabilities.
      </p>
    </>
  );
}
