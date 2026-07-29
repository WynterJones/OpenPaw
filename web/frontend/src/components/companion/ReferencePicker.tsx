/**
 * ReferencePicker — an optional starting image for companion generation.
 *
 * PixelLab's pixflux endpoint takes an `init_image` that steers the result, so
 * "make me a pixel version of this" becomes possible instead of describing a
 * look in words and hoping. Two sources, because they answer different needs:
 * an upload for arbitrary art, and the agent's own avatar for the common case
 * of wanting a sprite that will become that agent's animated chat avatar.
 *
 * Strength is exposed because the useful range is wide: low values treat the
 * reference as a hint, high values reproduce it closely in pixel form, and
 * which you want depends entirely on the source image.
 */

import { useRef, useState } from 'react';
import { ImagePlus, X, Bot, Upload } from 'lucide-react';
import type { AgentRole } from '../../lib/types';
import { NO_REFERENCE, type ReferenceState } from '../../lib/companionReference';

const MAX_BYTES = 4 * 1024 * 1024;

/** Read an avatar URL into a data URI so it can be sent as base64. */
async function urlToDataUri(url: string): Promise<string> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`Could not read avatar (${res.status})`);
  const blob = await res.blob();
  return await new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(new Error('Could not read avatar'));
    reader.readAsDataURL(blob);
  });
}

export function ReferencePicker({
  value,
  onChange,
  agents,
  onError,
}: {
  value: ReferenceState;
  onChange: (next: ReferenceState) => void;
  agents: AgentRole[];
  onError: (message: string) => void;
}) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [pickingAgent, setPickingAgent] = useState(false);

  const withAvatars = agents.filter(a => a.avatar_path);

  const onFile = (file: File | undefined) => {
    if (!file) return;
    if (!file.type.startsWith('image/')) {
      onError('Reference must be an image.');
      return;
    }
    if (file.size > MAX_BYTES) {
      onError('Reference image must be under 4MB.');
      return;
    }
    const reader = new FileReader();
    reader.onload = () =>
      onChange({ ...value, image: String(reader.result), label: file.name });
    reader.onerror = () => onError('Could not read that image.');
    reader.readAsDataURL(file);
  };

  const pickAvatar = async (agent: AgentRole) => {
    setPickingAgent(false);
    try {
      const dataUri = await urlToDataUri(agent.avatar_path);
      onChange({ ...value, image: dataUri, label: `${agent.name}'s avatar` });
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Could not read that avatar.');
    }
  };

  if (value.image) {
    return (
      <div className="rounded-lg border border-border-1 bg-surface-2 p-3">
        <div className="flex items-start gap-3">
          <img
            src={value.image}
            alt="Reference"
            className="w-16 h-16 rounded-lg object-cover border border-border-0 flex-shrink-0"
          />
          <div className="min-w-0 flex-1">
            <div className="flex items-center justify-between gap-2">
              <p className="text-xs font-medium text-text-1 truncate">{value.label || 'Reference'}</p>
              <button
                onClick={() => onChange(NO_REFERENCE)}
                className="p-1 rounded text-text-3 hover:text-danger hover:bg-surface-3 transition-colors cursor-pointer flex-shrink-0"
                aria-label="Remove reference image"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
            <label
              htmlFor="ref-strength"
              className="block text-[11px] text-text-3 mt-2 mb-1"
            >
              Influence: {value.strength}
            </label>
            <input
              id="ref-strength"
              type="range"
              min={1}
              max={999}
              step={1}
              value={value.strength}
              onChange={e => onChange({ ...value, strength: Number(e.target.value) })}
              className="w-full accent-[var(--op-accent-primary)]"
            />
            <p className="text-[10px] text-text-3 leading-relaxed">
              Low follows your description; high reproduces the reference.
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-1.5">
      <span className="block text-sm font-medium text-text-1">Reference image (optional)</span>
      <div className="flex flex-wrap gap-2">
        <button
          onClick={() => fileRef.current?.click()}
          className="inline-flex items-center gap-1.5 rounded-lg border border-border-1 bg-surface-2 px-3 py-1.5 text-xs text-text-2 hover:text-text-1 hover:border-border-0 transition-colors cursor-pointer"
        >
          <Upload className="w-3.5 h-3.5" aria-hidden="true" />
          Upload image
        </button>
        {withAvatars.length > 0 && (
          <div className="relative">
            <button
              onClick={() => setPickingAgent(o => !o)}
              className="inline-flex items-center gap-1.5 rounded-lg border border-border-1 bg-surface-2 px-3 py-1.5 text-xs text-text-2 hover:text-text-1 hover:border-border-0 transition-colors cursor-pointer"
            >
              <Bot className="w-3.5 h-3.5" aria-hidden="true" />
              Use an agent's avatar
            </button>
            {pickingAgent && (
              <div className="absolute z-50 mt-1 w-56 max-h-56 overflow-y-auto rounded-xl border border-border-0 bg-surface-1 shadow-2xl py-1">
                {withAvatars.map(a => (
                  <button
                    key={a.slug}
                    onClick={() => pickAvatar(a)}
                    className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-surface-2 transition-colors cursor-pointer"
                  >
                    <img src={a.avatar_path} alt="" className="w-6 h-6 rounded-md flex-shrink-0" />
                    <span className="text-sm text-text-1 truncate">{a.name}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
      <p className="text-[11px] text-text-3 leading-relaxed flex items-center gap-1">
        <ImagePlus className="w-3 h-3 flex-shrink-0" aria-hidden="true" />
        Steers the generation toward an existing image instead of description alone.
      </p>
      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={e => {
          onFile(e.target.files?.[0]);
          e.target.value = '';
        }}
      />
    </div>
  );
}
