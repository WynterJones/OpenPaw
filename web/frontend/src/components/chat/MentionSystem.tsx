import type { ReactNode } from 'react';
import type { Components } from 'react-markdown';
import type { AgentRole } from '../../lib/api';
import { MentionBadge } from './MentionBadge';

let _cachedPatternRoles: AgentRole[] = [];
let _cachedPattern = `@([A-Za-z][A-Za-z0-9_-]*)`;

function getMentionPattern(roles: AgentRole[]): string {
  if (roles === _cachedPatternRoles) return _cachedPattern;
  _cachedPatternRoles = roles;
  const roleNames = roles
    .flatMap(r => [r.name, r.slug])
    .filter(Boolean)
    .sort((a, b) => b.length - a.length);
  const escaped = roleNames.map(n => n.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
  _cachedPattern = escaped.length > 0
    ? `@(${escaped.join('|')}|[A-Za-z][A-Za-z0-9_-]*)`
    : `@([A-Za-z][A-Za-z0-9_-]*)`;
  return _cachedPattern;
}

function parseMentions(text: string, roles: AgentRole[]): ReactNode[] {
  const mentionRegex = new RegExp(getMentionPattern(roles), 'gi');
  const parts: ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = mentionRegex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }
    const mentionName = match[1].trim();
    const role = roles.find(r =>
      r.name.toLowerCase() === mentionName.toLowerCase() ||
      r.slug.toLowerCase() === mentionName.toLowerCase()
    );
    if (role) {
      parts.push(<MentionBadge key={`${match.index}-${mentionName}`} name={role.name} role={role} />);
    } else {
      parts.push(<MentionBadge key={`${match.index}-${mentionName}`} name={mentionName} />);
    }
    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }

  return parts.length > 0 ? parts : [text];
}

function processChildren(children: ReactNode, roles: AgentRole[]): ReactNode {
  if (typeof children === 'string') {
    return parseMentions(children, roles);
  }
  if (Array.isArray(children)) {
    return children.map((child, i) =>
      typeof child === 'string' ? <span key={i}>{parseMentions(child, roles)}</span> : child
    );
  }
  return children;
}

let _mcRolesRef: AgentRole[] = [];
let _mcResult: Partial<Components> | null = null;

export function mentionComponents(roles: AgentRole[]): Partial<Components> {
  if (roles === _mcRolesRef && _mcResult) return _mcResult;
  _mcRolesRef = roles;
  _mcResult = {
    p: ({ children, ...props }) => <p {...props}>{processChildren(children, roles)}</p>,
    li: ({ children, ...props }) => <li {...props}>{processChildren(children, roles)}</li>,
    td: ({ children, ...props }) => <td {...props}>{processChildren(children, roles)}</td>,
  };
  return _mcResult;
}
