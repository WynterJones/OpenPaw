import { Children, isValidElement, type ReactNode } from 'react';
import type { Components } from 'react-markdown';
import type { AgentRole } from '../../lib/api';
import { MentionBadge } from './MentionBadge';
import { CollapsibleCode } from './CollapsibleCode';

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

function extractText(node: ReactNode): string {
  if (typeof node === 'string') return node;
  if (typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (isValidElement(node)) {
    const props = node.props as Record<string, unknown>;
    return extractText(props.children as ReactNode);
  }
  return '';
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
    pre: ({ children }) => {
      if (Children.count(children) === 1) {
        const child = Children.toArray(children)[0];
        if (isValidElement(child) && child.type === 'code') {
          const childProps = child.props as Record<string, unknown>;
          const raw = extractText(childProps.children as ReactNode);
          const className = childProps.className as string | undefined;
          const lang = className?.replace('language-', '') || undefined;
          return (
            <CollapsibleCode language={lang} raw={raw}>
              {childProps.children as ReactNode}
            </CollapsibleCode>
          );
        }
      }
      return <pre>{children}</pre>;
    },
  };
  return _mcResult;
}
