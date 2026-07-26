/**
 * StudioEditor — the left column's Editor tab.
 *
 * Ordered the way a generation is actually decided: what am I making, who is
 * making it, with which model, from what prompt, how many. Controls that don't
 * apply to the current media type are hidden rather than disabled, so the panel
 * stays short in the narrow column.
 */

import { useEffect, useMemo, useState } from 'react';
import { Image as ImageIcon, Film, Music, Sparkles, Save, Loader2, FolderPlus } from 'lucide-react';
import { Button } from '../Button';
import { Select, Textarea } from '../Input';
import { ReferenceImages, type ReferenceImage } from './ReferenceImages';
import type {
  StudioFolder,
  StudioKind,
  StudioModel,
  StudioProvider,
} from '../../lib/types';

export interface EditorState {
  refImages: ReferenceImage[];
  type: StudioKind;
  provider: string;
  model: string;
  prompt: string;
  count: number;
  size: string;
  duration: number;
  folderId: string;
}

const KINDS: { value: StudioKind; label: string; icon: typeof ImageIcon }[] = [
  { value: 'image', label: 'Image', icon: ImageIcon },
  { value: 'video', label: 'Video', icon: Film },
  { value: 'audio', label: 'Audio', icon: Music },
];

interface Props {
  state: EditorState;
  onChange: (patch: Partial<EditorState>) => void;
  providers: StudioProvider[];
  supports: Record<StudioKind, boolean>;
  models: StudioModel[];
  modelsLoading: boolean;
  folders: StudioFolder[];
  generating: boolean;
  onGenerate: () => void;
  onSavePreset: () => void;
  onNewFolder: () => void;
  onError: (message: string) => void;
}

export function StudioEditor({
  state,
  onChange,
  providers,
  supports,
  models,
  modelsLoading,
  folders,
  generating,
  onGenerate,
  onSavePreset,
  onNewFolder,
  onError,
}: Props) {
  const [customModel, setCustomModel] = useState(false);

  // Providers that can make the selected type. An unconfigured one stays in
  // the list but is labelled, so the fix ("add a key") is discoverable.
  const usableProviders = useMemo(
    () => providers.filter(p => p.kinds.includes(state.type)),
    [providers, state.type],
  );

  const selectedModel = models.find(m => m.id === state.model);
  const multiProvider = useMemo(
    () => new Set(models.map(m => m.provider)).size > 1,
    [models],
  );
  const sizes = selectedModel?.sizes ?? [];
  const durations = selectedModel?.durations ?? [];
  // A custom model id isn't in the catalog, so nothing is known about it —
  // assume a single reference rather than hiding the control entirely.
  const refSlots = customModel ? 1 : (selectedModel?.max_ref_images ?? 0);

  // Switching to a model with fewer slots must drop the extras, or the request
  // would be rejected for carrying more references than allowed.
  useEffect(() => {
    if (state.refImages.length > refSlots) {
      onChange({ refImages: state.refImages.slice(0, refSlots) });
    }
  }, [refSlots, state.refImages, onChange]);

  // Whenever the model list changes under us (type or provider switch), make
  // sure the selection still exists — a stale model id fails at the API.
  useEffect(() => {
    if (customModel || modelsLoading || models.length === 0) return;
    if (!models.some(m => m.id === state.model)) {
      onChange({ model: models[0].id });
    }
  }, [models, modelsLoading, state.model, customModel, onChange]);

  const unsupported = !supports[state.type];

  return (
    <div className="flex flex-col gap-4 p-4" data-studio-editor>
      {/* Media type — a segmented control rather than three separate cards, so
          the three options read as one setting with one value. */}
      <div
        role="radiogroup"
        aria-label="Media type"
        className="flex items-center gap-1 rounded-lg border border-border-0 bg-surface-2 p-1"
      >
        {KINDS.map(k => {
          const active = state.type === k.value;
          const available = supports[k.value];
          return (
            <button
              key={k.value}
              role="radio"
              aria-checked={active}
              onClick={() => onChange({ type: k.value, model: '' })}
              title={available ? k.label : `${k.label} needs a Replicate or fal API key`}
              className={`flex-1 flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
                active
                  ? 'bg-accent-primary/15 text-accent-text'
                  : 'text-text-2 hover:text-text-1 hover:bg-surface-3'
              } ${!available ? 'opacity-50' : ''}`}
            >
              <k.icon className="w-3.5 h-3.5 flex-shrink-0" aria-hidden="true" />
              {k.label}
            </button>
          );
        })}
      </div>

      {unsupported && (
        <p className="rounded-lg border border-border-0 bg-surface-2 px-3 py-2 text-[11px] leading-relaxed text-text-2">
          No configured provider can generate {state.type}. Add a{' '}
          <span className="text-text-1">Replicate</span> or <span className="text-text-1">fal</span>{' '}
          API key in Settings to enable it.
        </p>
      )}

      <Select
        label="Provider"
        value={state.provider}
        onChange={e => onChange({ provider: e.target.value, model: '' })}
        options={[
          { value: '', label: 'Auto' },
          ...usableProviders.map(p => ({
            value: p.name,
            label: p.configured ? p.name : `${p.name} — no API key`,
          })),
        ]}
      />

      <div className="space-y-1.5">
        <div className="flex items-center justify-between">
          <label htmlFor="studio-model" className="block text-sm font-medium text-text-1">
            Model
          </label>
          <button
            onClick={() => setCustomModel(v => !v)}
            className="text-[11px] text-text-3 hover:text-accent-text transition-colors cursor-pointer"
          >
            {customModel ? 'Pick from list' : 'Enter custom'}
          </button>
        </div>

        {customModel ? (
          <>
            <input
              id="studio-model"
              value={state.model}
              onChange={e => onChange({ model: e.target.value })}
              placeholder="owner/model-name"
              className="block w-full rounded-lg border border-border-1 bg-surface-2 text-text-0 px-3 py-2 text-sm placeholder:text-text-3 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors"
            />
            <p className="text-[11px] text-text-3 leading-relaxed">
              Any model id the chosen provider accepts.
            </p>
          </>
        ) : modelsLoading ? (
          <div className="flex items-center gap-2 rounded-lg border border-border-1 bg-surface-2 px-3 py-2 text-sm text-text-3">
            <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" />
            Loading models…
          </div>
        ) : models.length === 0 ? (
          <p className="rounded-lg border border-border-1 bg-surface-2 px-3 py-2 text-sm text-text-3">
            No models available.
          </p>
        ) : (
          <>
            <select
              id="studio-model"
              value={state.model}
              onChange={e => onChange({ model: e.target.value })}
              className="block w-full rounded-lg border border-border-1 bg-surface-2 text-text-0 px-3 py-2 text-sm focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors"
            >
              {/* The provider is only worth the width when more than one is
                  offering models for this media type — otherwise every row
                  ends in the same word. */}
              {models.map(m => (
                <option key={`${m.provider}:${m.id}`} value={m.id}>
                  {multiProvider ? `${m.name} · ${m.provider}` : m.name}
                </option>
              ))}
            </select>
            {selectedModel?.description && (
              <p className="text-[11px] text-text-3 leading-relaxed">{selectedModel.description}</p>
            )}
          </>
        )}
      </div>

      <Textarea
        label="Prompt"
        value={state.prompt}
        onChange={e => onChange({ prompt: e.target.value })}
        onKeyDown={e => {
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && !generating) onGenerate();
        }}
        rows={6}
        placeholder={
          state.type === 'audio'
            ? 'Describe the music or speech…'
            : 'Describe what to create. Detail helps: subject, style, lighting, mood.'
        }
        className="resize-y"
      />

      {refSlots > 0 && (
        <ReferenceImages
          images={state.refImages}
          max={refSlots}
          onChange={refImages => onChange({ refImages })}
          onError={onError}
        />
      )}

      <div className="grid grid-cols-2 gap-3">
        <Select
          label="Count"
          value={String(state.count)}
          onChange={e => onChange({ count: Number(e.target.value) })}
          options={[1, 2, 3, 4, 6, 8].map(n => ({ value: String(n), label: String(n) }))}
        />

        {sizes.length > 0 ? (
          <Select
            label="Size"
            value={state.size}
            onChange={e => onChange({ size: e.target.value })}
            options={sizes.map(s => ({ value: s, label: s }))}
          />
        ) : durations.length > 0 ? (
          <Select
            label="Length"
            value={String(state.duration)}
            onChange={e => onChange({ duration: Number(e.target.value) })}
            options={durations.map(d => ({ value: String(d), label: `${d}s` }))}
          />
        ) : (
          <div />
        )}
      </div>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between">
          <label htmlFor="studio-folder" className="block text-sm font-medium text-text-1">
            Save to
          </label>
          <button
            onClick={onNewFolder}
            className="flex items-center gap-1 text-[11px] text-text-3 hover:text-accent-text transition-colors cursor-pointer"
          >
            <FolderPlus className="w-3 h-3" aria-hidden="true" />
            New folder
          </button>
        </div>
        <select
          id="studio-folder"
          value={state.folderId}
          onChange={e => onChange({ folderId: e.target.value })}
          className="block w-full rounded-lg border border-border-1 bg-surface-2 text-text-0 px-3 py-2 text-sm focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors"
        >
          <option value="">Unfiled</option>
          {folders.map(f => (
            <option key={f.id} value={f.id}>
              {f.name}
            </option>
          ))}
        </select>
      </div>

      <div className="flex gap-2 pt-1">
        <Button
          onClick={onGenerate}
          disabled={generating || !state.prompt.trim() || unsupported}
          icon={
            generating ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Sparkles className="w-4 h-4" />
            )
          }
          className="flex-1"
        >
          {generating ? 'Generating…' : `Generate ${state.count > 1 ? state.count : ''}`.trim()}
        </Button>
        <Button
          variant="secondary"
          onClick={onSavePreset}
          disabled={!state.prompt.trim()}
          title="Save these settings to the Saved tab"
          aria-label="Save settings"
        >
          <Save className="w-4 h-4" />
        </Button>
      </div>

      {generating && (
        <p className="text-[11px] text-text-3 leading-relaxed text-center">
          Video and music can take a few minutes. Leaving this page cancels the run.
        </p>
      )}
    </div>
  );
}
