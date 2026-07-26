/**
 * Human durations for the heartbeat settings.
 *
 * Intervals are stored as seconds but are only ever *thought about* in minutes
 * and hours, so every field that edits one accepts "30m" or "1h 30m" and shows
 * the same back. Shared rather than duplicated because the global interval and
 * each agent's override must round-trip identically — a field that accepted a
 * format the other rejected would look like data loss.
 */

/** Parse "1h 30m", "45s", or a bare number of seconds. Null if unparseable. */
export function parseDuration(input: string): number | null {
  const trimmed = input.trim().toLowerCase();
  if (!trimmed) return null;

  // Pure number = treat as seconds
  if (/^\d+$/.test(trimmed)) return parseInt(trimmed, 10);

  let total = 0;
  let matched = false;
  const regex = /(\d+)\s*(h|m|s)/g;
  let match;
  while ((match = regex.exec(trimmed)) !== null) {
    matched = true;
    const val = parseInt(match[1], 10);
    switch (match[2]) {
      case 'h': total += val * 3600; break;
      case 'm': total += val * 60; break;
      case 's': total += val; break;
    }
  }
  return matched ? total : null;
}

/** Render seconds the way the parser accepts them: "1h 30m". */
export function secondsToDurationStr(sec: number): string {
  if (sec <= 0) return '0s';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const parts: string[] = [];
  if (h > 0) parts.push(`${h}h`);
  if (m > 0) parts.push(`${m}m`);
  if (s > 0) parts.push(`${s}s`);
  return parts.join(' ');
}

/** Compact form for read-only display — one unit of precision, not three. */
export function formatInterval(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m`;
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return m > 0 ? `${h}h ${m}m` : `${h}h`;
}
