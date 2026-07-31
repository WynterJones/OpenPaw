/**
 * Splits the AI-only pasted-image footer off a user message.
 *
 * Sending a pasted image appends absolute on-disk paths so every provider can
 * read the file. Those paths are transport metadata rather than message copy,
 * so chat surfaces render the images and hide the footer.
 */
const PASTED_BLOCK_RE = /\n\n---\n\*\*Pasted image\(s\)\*\*[^\n]*\n([\s\S]*)$/;
const PASTED_LINK_RE = /\[([^\]]*)\]\(([^)]+)\)/g;

export function splitPastedImages(content: string): {
  text: string;
  images: { name: string; path: string }[];
} {
  const match = content.match(PASTED_BLOCK_RE);
  if (!match) return { text: content, images: [] };

  const images: { name: string; path: string }[] = [];
  for (const matchedLink of match[1].matchAll(PASTED_LINK_RE)) {
    images.push({ name: matchedLink[1] || 'image', path: matchedLink[2] });
  }
  if (images.length === 0) return { text: content, images: [] };

  return { text: content.slice(0, match.index).trimEnd(), images };
}

/** Local absolute paths are served through the guarded OpenClaw file route. */
export function localFileSrc(path: string) {
  return `/api/v1/openclaw/file?path=${encodeURIComponent(path)}`;
}
