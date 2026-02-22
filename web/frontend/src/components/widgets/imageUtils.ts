const BASE64_RE = /^[A-Za-z0-9+/\n\r]+=*$/;

const IMAGE_KEYS = new Set(['data', 'image', 'screenshot', 'base64', 'image_data', 'imageData']);
const FORMAT_HINTS = new Set(['png', 'jpeg', 'jpg', 'gif', 'webp', 'svg', 'bmp', 'ico']);

export function isBase64Image(value: string): boolean {
  if (typeof value !== 'string' || value.length < 500) return false;
  const sample = value.slice(0, 200).replace(/[\n\r\s]/g, '');
  return BASE64_RE.test(sample);
}

export function guessImageMime(data: Record<string, unknown>, base64: string): string {
  const fmt = data.format ?? data.mime ?? data.mimeType ?? data.type;
  if (typeof fmt === 'string') {
    const lower = fmt.toLowerCase();
    if (lower.includes('/')) return lower;
    if (FORMAT_HINTS.has(lower)) return `image/${lower === 'jpg' ? 'jpeg' : lower}`;
  }

  const clean = base64.replace(/[\n\r\s]/g, '');
  if (clean.startsWith('iVBORw0KGgo')) return 'image/png';
  if (clean.startsWith('/9j/')) return 'image/jpeg';
  if (clean.startsWith('R0lGOD')) return 'image/gif';
  if (clean.startsWith('UklGR')) return 'image/webp';
  if (clean.startsWith('PHN2Zy') || clean.startsWith('PD94bW')) return 'image/svg+xml';

  return 'image/png';
}

export function base64ToDataUri(base64: string, mime: string): string {
  const clean = base64.replace(/[\n\r\s]/g, '');
  return `data:${mime};base64,${clean}`;
}

export function findBase64Field(data: Record<string, unknown>): { key: string; value: string } | null {
  for (const key of Object.keys(data)) {
    if (IMAGE_KEYS.has(key.toLowerCase()) && typeof data[key] === 'string' && isBase64Image(data[key] as string)) {
      return { key, value: data[key] as string };
    }
  }
  for (const [key, val] of Object.entries(data)) {
    if (typeof val === 'string' && isBase64Image(val)) {
      return { key, value: val };
    }
  }
  return null;
}

export function hasImageShape(data: Record<string, unknown>): boolean {
  return findBase64Field(data) !== null;
}
