/**
 * HeartbeatOverride — one per-agent setting that may fall back to the global one.
 *
 * Blank means "inherit", and the placeholder shows what inheriting currently
 * gets you. That matters more than it sounds: the alternative is pre-filling
 * the global value, which silently turns every agent into an override the
 * moment anyone opens the page, and then changing the global setting stops
 * affecting anybody.
 *
 * Saves on blur rather than behind a button, matching the rest of the agent
 * sidebar — but only when the value actually changed, so tabbing through the
 * fields doesn't fire three writes.
 */

import { useState } from 'react';
import { parseDuration, secondsToDurationStr } from '../lib/duration';

interface Props {
  label: string;
  hint: string;
  /** Current stored value in seconds (or a raw count). 0 = inherit. */
  value: number;
  /** What inheriting resolves to right now, shown as the placeholder. */
  globalHint: number;
  kind: 'duration' | 'count';
  onSave: (value: number) => void;
}

export function HeartbeatOverride({ label, hint, value, globalHint, kind, onSave }: Props) {
  const format = (v: number) => (v <= 0 ? '' : kind === 'duration' ? secondsToDurationStr(v) : String(v));
  const [text, setText] = useState(() => format(value));
  const [error, setError] = useState('');

  // Follow the saved value when it changes underneath us (a save landing, or
  // the agent reloading). Adjusted during render rather than in an effect: an
  // effect would paint the stale text for a frame first, and React re-runs this
  // render before committing anything.
  const [seenValue, setSeenValue] = useState(value);
  if (seenValue !== value) {
    setSeenValue(value);
    setText(format(value));
    setError('');
  }

  const inheritLabel = kind === 'duration' ? secondsToDurationStr(globalHint) : String(globalHint);

  const commit = () => {
    const raw = text.trim();
    if (raw === '') {
      setError('');
      if (value !== 0) onSave(0);
      return;
    }
    const parsed = kind === 'duration' ? parseDuration(raw) : Number(raw);
    if (parsed === null || !Number.isFinite(parsed) || parsed <= 0) {
      setError(kind === 'duration' ? 'Use h, m, s — e.g. 30m' : 'Whole number above 0');
      setText(format(value));
      return;
    }
    const next = Math.floor(parsed);
    if (kind === 'duration' && next < 60) {
      setError('Minimum 1m');
      setText(format(value));
      return;
    }
    setError('');
    if (next !== value) onSave(next);
    else setText(format(next));
  };

  const id = `hb-${label.toLowerCase().replace(/\s+/g, '-')}`;

  return (
    <div>
      <label htmlFor={id} className="block text-xs font-medium text-text-2 mb-1.5">{label}</label>
      <input
        id={id}
        type="text"
        value={text}
        onChange={e => { setText(e.target.value); setError(''); }}
        onBlur={commit}
        onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
        placeholder={`Global (${inheritLabel})`}
        className={`w-full rounded-lg border px-3 py-2 text-sm text-text-1 bg-surface-0 placeholder:text-text-3 focus:outline-none focus:ring-2 focus:ring-accent-primary/50 ${
          error ? 'border-red-500/50' : 'border-border-1'
        }`}
      />
      <p className={`text-[11px] mt-1 leading-relaxed ${error ? 'text-red-400' : 'text-text-3'}`}>
        {error || hint}
      </p>
    </div>
  );
}
