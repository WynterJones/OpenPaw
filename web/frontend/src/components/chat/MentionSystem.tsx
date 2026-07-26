import { Children, isValidElement, type ReactNode } from 'react';
import { VideoPlayer, AudioPlayer } from '../studio/MediaPlayer';
import { Download } from 'lucide-react';
import type { Components } from 'react-markdown';
import type { AgentRole } from '../../lib/api';
import { MentionBadge } from './MentionBadge';
import { CollapsibleCode } from './CollapsibleCode';
import { handleExternalLinkClick } from '../../lib/openExternal';
import { downloadFile } from '../../lib/download';

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

const IMAGE_EXT_RE = /\.(png|jpe?g|gif|webp|svg|bmp)$/i;

// imageSrcFor returns a displayable image URL for a markdown link href, or null
// if the href isn't an image. Local file paths (what OpenClaw agents return,
// since they run on the same machine) are routed through the file-serving
// endpoint; remote http(s) image URLs are shown directly.
function imageSrcFor(href: string): string | null {
  if (!href || !IMAGE_EXT_RE.test(href.split(/[?#]/)[0])) return null;
  if (/^https?:\/\//i.test(href)) return href;
  if (/^(data:|blob:)/i.test(href)) return href;
  if (href.startsWith('/') || /^[a-zA-Z]:[\\/]/.test(href)) {
    return `/api/v1/openclaw/file?path=${encodeURIComponent(href)}`;
  }
  return null;
}

const VIDEO_EXT_RE = /\.(mp4|webm|mov|m4v)$/i;
const AUDIO_EXT_RE = /\.(mp3|wav|ogg|m4a|flac)$/i;

/**
 * Detects playable media a message links to.
 *
 * Studio's media URLs (/api/v1/media/{id}/file) carry no extension, so
 * studio_generate appends ?kind=video|audio purely as a rendering hint — the
 * file route ignores unknown query params. Plain file paths still fall back to
 * their extension.
 */
function mediaKindFor(href: string): 'video' | 'audio' | null {
  if (!href) return null;

  const hint = href.match(/[?&]kind=(video|audio)\b/i);
  if (hint) return hint[1].toLowerCase() as 'video' | 'audio';

  const path = href.split(/[?#]/)[0];
  if (VIDEO_EXT_RE.test(path)) return 'video';
  if (AUDIO_EXT_RE.test(path)) return 'audio';
  return null;
}

function renderInlineMedia(src: string, kind: 'video' | 'audio'): ReactNode {
  // <span>, not <div>: this renders inside a markdown paragraph, and a block
  // element there is invalid HTML that React will warn about.
  return (
    <span className="block my-2 max-w-md">
      {kind === 'video' ? (
        <span className="block rounded-xl overflow-hidden border border-border-1 aspect-video bg-black">
          <VideoPlayer src={src} />
        </span>
      ) : (
        <AudioPlayer src={src} />
      )}
      <button
        onClick={() => downloadFile(`${src}${src.includes('?') ? '&' : '?'}download=1`)}
        className="inline-flex items-center gap-1 mt-1.5 text-[11px] text-text-3 hover:text-accent-text transition-colors cursor-pointer"
      >
        <Download className="w-3 h-3" aria-hidden="true" />
        Download
      </button>
    </span>
  );
}

function renderInlineImage(src: string, label: string): ReactNode {
  return (
    <span className="block my-2">
      <span className="relative group inline-block rounded-xl overflow-hidden border border-border-1 max-w-full">
        <img
          src={src}
          alt={label || 'image'}
          className="max-w-full max-h-[400px] rounded-xl object-contain block"
        />
        <button
          onClick={() => downloadFile(src)}
          className="absolute top-2 right-2 inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-black/60 backdrop-blur-sm !text-white text-xs font-medium opacity-0 group-hover:opacity-100 transition-opacity hover:bg-black/80 ring-1 ring-white/10 cursor-pointer"
          title="Download image"
        >
          <Download className="w-3.5 h-3.5" aria-hidden="true" />
          Download
        </button>
      </span>
    </span>
  );
}

let _mcRolesRef: AgentRole[] = [];
let _mcResult: Partial<Components> | null = null;

export function mentionComponents(roles: AgentRole[]): Partial<Components> {
  if (roles === _mcRolesRef && _mcResult) return _mcResult;
  _mcRolesRef = roles;
  _mcResult = {
    a: ({ href, children, ...props }) => {
      const imgSrc = href ? imageSrcFor(href) : null;
      if (imgSrc) {
        const label = typeof children === 'string' ? children : extractText(children as ReactNode);
        return renderInlineImage(imgSrc, label);
      }
      // Generated video and music get real players inline, rather than a bare
      // link the user has to open in a new tab to judge.
      const mediaKind = href ? mediaKindFor(href) : null;
      if (mediaKind && href) {
        return renderInlineMedia(href, mediaKind);
      }
      return (
        <a
          href={href}
          target="_blank"
          rel="noreferrer"
          onClick={(e) => handleExternalLinkClick(e, href)}
          {...props}
        >
          {children}
        </a>
      );
    },
    p: ({ children, ...props }) => <p {...props}>{processChildren(children, roles)}</p>,
    li: ({ children, ...props }) => <li {...props}>{processChildren(children, roles)}</li>,
    td: ({ children, ...props }) => <td {...props}>{processChildren(children, roles)}</td>,
    table: ({ children, ...props }) => (
      <div className="prose-table-wrap"><table {...props}>{children}</table></div>
    ),
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
