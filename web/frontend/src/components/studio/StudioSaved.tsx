/**
 * StudioSaved — the left column's Saved tab.
 *
 * A list of previously-saved editor setups. Clicking one loads it back into
 * the Editor rather than generating immediately: restoring a configuration
 * should never cost money by itself.
 */

import { Image as ImageIcon, Film, Music, Trash2, Clock } from 'lucide-react';
import type { StudioKind, StudioPreset } from '../../lib/types';

const KIND_ICON: Record<StudioKind, typeof ImageIcon> = {
  image: ImageIcon,
  video: Film,
  audio: Music,
};

interface Props {
  presets: StudioPreset[];
  loading: boolean;
  onLoad: (preset: StudioPreset) => void;
  onDelete: (preset: StudioPreset) => void;
}

export function StudioSaved({ presets, loading, onLoad, onDelete }: Props) {
  if (loading) {
    return <p className="px-4 py-8 text-center text-xs text-text-3">Loading…</p>;
  }

  if (presets.length === 0) {
    return (
      <div className="px-5 py-10 text-center">
        <Clock className="w-6 h-6 mx-auto mb-3 text-text-3" aria-hidden="true" />
        <p className="text-xs text-text-2 leading-relaxed">
          Nothing saved yet.
          <br />
          Set up a generation in the Editor and press the save button to keep it here.
        </p>
      </div>
    );
  }

  return (
    <div className="py-1">
      {presets.map(p => {
        const Icon = KIND_ICON[p.media_type] ?? ImageIcon;
        return (
          <div
            key={p.id}
            className="group flex items-start gap-2.5 px-4 py-2.5 border-b border-border-0 last:border-0 hover:bg-surface-2 transition-colors"
          >
            <button
              onClick={() => onLoad(p)}
              className="flex flex-1 min-w-0 items-start gap-2.5 text-left cursor-pointer"
            >
              <Icon className="w-4 h-4 mt-0.5 flex-shrink-0 text-accent-text" aria-hidden="true" />
              <span className="min-w-0 flex-1">
                <span className="block text-sm text-text-1 truncate">{p.name}</span>
                <span className="block text-[11px] text-text-3 truncate">
                  {p.model || 'default model'}
                  {p.count > 1 ? ` · ×${p.count}` : ''}
                </span>
              </span>
            </button>
            <button
              onClick={() => onDelete(p)}
              aria-label={`Delete ${p.name}`}
              className="p-1 rounded-md text-text-3 opacity-0 group-hover:opacity-100 hover:text-danger hover:bg-surface-3 transition-all cursor-pointer flex-shrink-0"
            >
              <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
