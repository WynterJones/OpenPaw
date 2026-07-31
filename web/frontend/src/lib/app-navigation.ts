import type { LucideIcon } from 'lucide-react';
import {
  Bot,
  BookOpen,
  Clapperboard,
  Clock,
  Database,
  FileText,
  Heart,
  Inbox,
  KeyRound,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  Settings,
  Sparkles,
  Store,
  TerminalSquare,
  Wrench,
} from 'lucide-react';

export type AppNavGroup = 'Workspace' | 'Knowledge' | 'System' | 'More';

export interface AppNavItem {
  id: string;
  to: string;
  icon: LucideIcon;
  label: string;
  description: string;
  group: AppNavGroup;
  defaultKey: string;
  keywords?: string[];
}

export const APP_NAV_ITEMS: AppNavItem[] = [
  { id: 'chat', to: '/chat', icon: MessageSquare, label: 'Chats', description: 'Conversations with your agents', group: 'Workspace', defaultKey: '1', keywords: ['messages', 'new chat'] },
  { id: 'inbox', to: '/inbox', icon: Inbox, label: 'Inbox', description: 'Reports and agent posts', group: 'Workspace', defaultKey: '2', keywords: ['posts', 'reports', 'notifications'] },
  { id: 'terminal', to: '/terminal', icon: TerminalSquare, label: 'Terminal', description: 'Global terminals and processes', group: 'Workspace', defaultKey: '3', keywords: ['shell', 'workbench', 'global'] },
  { id: 'studio', to: '/studio', icon: Clapperboard, label: 'Studio', description: 'Generate and manage media', group: 'Workspace', defaultKey: '4', keywords: ['images', 'video', 'audio', 'media'] },
  { id: 'dashboards', to: '/dashboards', icon: LayoutDashboard, label: 'Dashboards', description: 'Views powered by workspace data', group: 'Workspace', defaultKey: '8', keywords: ['reports', 'widgets'] },
  { id: 'context', to: '/knowledge-base', icon: BookOpen, label: 'Context', description: 'Knowledge files and workspace directory', group: 'Knowledge', defaultKey: '5', keywords: ['knowledge', 'files', 'directory', 'memory'] },
  { id: 'databases', to: '/databases', icon: Database, label: 'Databases', description: 'Structured workspace data', group: 'Knowledge', defaultKey: '6', keywords: ['tables', 'rows', 'airtable'] },
  { id: 'tasks', to: '/todo-lists', icon: ListTodo, label: 'Tasks', description: 'Workspace todo lists', group: 'Knowledge', defaultKey: '7', keywords: ['todos', 'lists'] },
  { id: 'agents', to: '/agents', icon: Bot, label: 'Agents', description: 'Manage agents and the Gateway', group: 'System', defaultKey: 'a' },
  { id: 'services', to: '/services', icon: Wrench, label: 'Services', description: 'Tools and connected services', group: 'System', defaultKey: 's', keywords: ['tools'] },
  { id: 'skills', to: '/skills', icon: Sparkles, label: 'Skills', description: 'Agent skills and instructions', group: 'System', defaultKey: 'k' },
  { id: 'scheduler', to: '/scheduler', icon: Clock, label: 'Scheduler', description: 'Automations and scheduled reports', group: 'System', defaultKey: 'r', keywords: ['schedule', 'automation'] },
  { id: 'heartbeat', to: '/heartbeat', icon: Heart, label: 'Heartbeat', description: 'Background agent activity', group: 'System', defaultKey: 'h' },
  { id: 'templates', to: '/library', icon: Store, label: 'Templates', description: 'Reusable agents, skills, and tools', group: 'More', defaultKey: 'l', keywords: ['library'] },
  { id: 'secrets', to: '/secrets', icon: KeyRound, label: 'Secrets', description: 'Workspace credentials and keys', group: 'More', defaultKey: 'e' },
  { id: 'logs', to: '/logs', icon: FileText, label: 'Logs', description: 'Application and agent logs', group: 'More', defaultKey: 'o' },
  { id: 'settings', to: '/settings', icon: Settings, label: 'Settings', description: 'Configure OpenPaw', group: 'More', defaultKey: ',' },
];

export const DEFAULT_HOTKEY_BINDINGS = Object.fromEntries(
  APP_NAV_ITEMS.map(item => [item.id, item.defaultKey]),
) as Record<string, string>;

export type HotkeyModifier = 'ctrl' | 'meta' | 'alt';

export const modifierLabel = (modifier: HotkeyModifier) => {
  if (modifier === 'meta') return '⌘';
  if (modifier === 'alt') return 'Alt';
  return 'Ctrl';
};

export const hotkeyLabel = (modifier: HotkeyModifier, key: string) =>
  `${modifierLabel(modifier)} ${key.length === 1 ? key.toUpperCase() : key}`;

// Letter bindings use Shift so app navigation never hijacks editing staples
// such as Ctrl+A (select all), Ctrl+S (save), or Ctrl+H (delete/back).
export const navigationHotkeyLabel = (modifier: HotkeyModifier, key: string) =>
  `${modifierLabel(modifier)} ${/^[a-z]$/i.test(key) ? '⇧ ' : ''}${key.toUpperCase()}`;
