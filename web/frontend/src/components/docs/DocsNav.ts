import type { LucideIcon } from 'lucide-react';
import {
  Home,
  Rocket,
  Cpu,
  Building2,
  LayoutGrid,
  Monitor,
  Lightbulb,
} from 'lucide-react';

export interface DocsNavItem {
  label: string;
  href: string;
  icon: LucideIcon;
}

export interface DocsNavGroup {
  title: string;
  items: DocsNavItem[];
}

export const docsNav: DocsNavGroup[] = [
  {
    title: 'Home',
    items: [
      { label: 'Overview', href: '/docs', icon: Home },
    ],
  },
  {
    title: 'Getting Started',
    items: [
      { label: 'Get Started', href: '/docs/get-started', icon: Rocket },
    ],
  },
  {
    title: 'Core Concepts',
    items: [
      { label: 'How It Works', href: '/docs/how-it-works', icon: Cpu },
      { label: 'Architecture', href: '/docs/architecture', icon: Building2 },
    ],
  },
  {
    title: 'Features',
    items: [
      { label: 'Features', href: '/docs/features', icon: LayoutGrid },
      { label: 'Desktop App', href: '/docs/desktop', icon: Monitor },
    ],
  },
  {
    title: 'Guides',
    items: [
      { label: 'Use Cases', href: '/docs/use-cases', icon: Lightbulb },
    ],
  },
];
