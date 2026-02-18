import type { StreamEvent } from './api';

export function timeAgo(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const seconds = Math.floor((now - then) / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  if (days < 30) return `${Math.floor(days / 7)}w ago`;
  return new Date(dateStr).toLocaleDateString();
}

export function cleanToolColons(text: string, hasTools: boolean): string {
  if (!hasTools || !text) return text;
  return text.replace(/:(\s*\n\n)/g, '.$1').replace(/:\s*$/, '.');
}

export function getToolDetail(toolName: string, input: Record<string, unknown>): string {
  switch (toolName) {
    case 'Read':
    case 'Write':
    case 'Edit':
      return (input.file_path as string) || '';
    case 'Bash':
      return ((input.command as string) || '').slice(0, 80);
    case 'Grep':
    case 'Glob':
      return (input.pattern as string) || '';
    case 'WebFetch':
      return (input.url as string) || '';
    case 'WebSearch':
      return (input.query as string) || '';
    default:
      return '';
  }
}

export interface StreamingTool {
  name: string;
  id: string;
  done: boolean;
  detail?: string;
}

export interface CostInfo {
  total_cost_usd: number;
  usage?: StreamEvent['usage'];
  num_turns?: number;
}

interface ToolGroup<T> {
  name: string;
  items: T[];
}

export function groupBy<T>(items: T[], keyFn: (item: T) => string): ToolGroup<T>[] {
  const map = new Map<string, T[]>();
  for (const item of items) {
    const key = keyFn(item);
    const arr = map.get(key);
    if (arr) arr.push(item);
    else map.set(key, [item]);
  }
  return Array.from(map, ([name, items]) => ({ name, items }));
}
