/**
 * AI-generated UI backgrounds.
 *
 * Sits inside the Background Image card in Settings → Design, under the shipped
 * presets. A generation composes four things into one request: the OpenPaw
 * mascot, an existing background as the style reference, the chosen agent's
 * avatar, and the user's prompt — so what comes back belongs to the same world
 * as the presets instead of looking like a stock wallpaper.
 *
 * The heavy lifting is server-side (internal/handlers/backgrounds.go); this is
 * the recipe form plus a picker that behaves exactly like the preset row.
 */

import { useEffect, useState } from 'react';
import { Sparkles, Trash2, Loader2 } from 'lucide-react';
import { api, ApiError } from '../../lib/api';
import { useToast } from '../Toast';
import type { AgentRole } from '../../lib/types';

interface GeneratedBackground {
  id: string;
  name: string;
  prompt: string;
  agent_slug: string;
  style_ref: string;
  model: string;
  url: string;
  created_at: string;
}

interface BackgroundPreset {
  url: string;
  name: string;
}

interface BackgroundGeneratorProps {
  /** The shipped presets, offered as style references. */
  presets: BackgroundPreset[];
  /** The background currently selected in the parent picker. */
  selected: string;
  onSelect: (url: string) => void;
}

export function BackgroundGenerator({ presets, selected, onSelect }: BackgroundGeneratorProps) {
  const { toast } = useToast();
  const [items, setItems] = useState<GeneratedBackground[]>([]);
  const [agents, setAgents] = useState<AgentRole[]>([]);
  const [open, setOpen] = useState(false);
  const [prompt, setPrompt] = useState('');
  const [agentSlug, setAgentSlug] = useState('');
  const [styleRef, setStyleRef] = useState(presets[0]?.url ?? '');
  const [generating, setGenerating] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  useEffect(() => {
    api
      .get<{ backgrounds: GeneratedBackground[] }>('/backgrounds')
      .then(r => setItems(r.backgrounds ?? []))
      .catch(() => {});
    api
      .get<AgentRole[]>('/agent-roles')
      .then(list => setAgents(list.filter(a => a.avatar_path)))
      .catch(() => {});
  }, []);

  const generate = async () => {
    const text = prompt.trim();
    if (!text) {
      toast('error', 'Describe the background you want');
      return;
    }
    setGenerating(true);
    try {
      const created = await api.post<GeneratedBackground>('/backgrounds/generate', {
        prompt: text,
        agent_slug: agentSlug,
        style_ref: styleRef,
      });
      setItems(prev => [created, ...prev]);
      onSelect(created.url);
      setPrompt('');
      toast('success', 'Background generated — remember to Save');
    } catch (e) {
      toast('error', e instanceof ApiError ? e.message : 'Failed to generate background');
    } finally {
      setGenerating(false);
    }
  };

  const remove = async (item: GeneratedBackground) => {
    setDeleting(item.id);
    try {
      await api.delete(`/backgrounds/${item.id}`);
      setItems(prev => prev.filter(b => b.id !== item.id));
      // The server clears the saved setting when the active background is
      // deleted; mirror that locally so the picker doesn't stay on a dead URL.
      if (selected === item.url) onSelect('');
      toast('success', 'Background deleted');
    } catch {
      toast('error', 'Failed to delete background');
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <label className="block text-xs font-medium text-text-2">Generated</label>
        <button
          type="button"
          onClick={() => setOpen(o => !o)}
          aria-expanded={open}
          className="inline-flex items-center gap-1.5 text-xs font-medium text-accent-text hover:opacity-80 cursor-pointer transition-opacity"
        >
          <Sparkles className="w-3.5 h-3.5" />
          {open ? 'Close generator' : 'Generate with AI'}
        </button>
      </div>

      {items.length === 0 && !open && (
        <p className="text-[11px] text-text-3">
          None yet. Generate one in the style of a preset, starring the mascot and an agent of
          your choice.
        </p>
      )}

      {items.length > 0 && (
        <div className="flex flex-wrap gap-3">
          {items.map(item => (
            <div key={item.id} className="relative group">
              <button
                onClick={() => onSelect(item.url)}
                aria-label={item.name}
                aria-pressed={selected === item.url}
                className={`relative w-20 h-14 rounded-lg overflow-hidden border-2 transition-all cursor-pointer bg-cover bg-center ${
                  selected === item.url
                    ? 'border-accent-primary ring-2 ring-accent-primary/30 scale-105'
                    : 'border-border-1 hover:border-border-0 hover:scale-105'
                }`}
                style={{ backgroundImage: `url(${item.url})` }}
                title={item.prompt || item.name}
              >
                {selected === item.url && (
                  <div className="absolute inset-0 bg-black/40 flex items-center justify-center">
                    <svg
                      className="w-4 h-4 text-white drop-shadow-sm"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                      strokeWidth={3}
                    >
                      <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  </div>
                )}
              </button>
              <button
                onClick={() => remove(item)}
                disabled={deleting === item.id}
                aria-label={`Delete ${item.name}`}
                title="Delete"
                className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full bg-surface-2 border border-border-1 text-text-3 hover:text-red-400 hover:border-red-400/40 flex items-center justify-center opacity-0 group-hover:opacity-100 focus-visible:opacity-100 transition-opacity cursor-pointer disabled:opacity-40"
              >
                {deleting === item.id ? (
                  <Loader2 className="w-3 h-3 animate-spin" />
                ) : (
                  <Trash2 className="w-3 h-3" />
                )}
              </button>
            </div>
          ))}
        </div>
      )}

      {open && (
        <div className="mt-3 p-3 rounded-xl border border-border-1 bg-surface-2 space-y-3">
          <div>
            <label
              htmlFor="bg-gen-prompt"
              className="block text-xs font-medium text-text-2 mb-1.5"
            >
              What should it show?
            </label>
            <textarea
              id="bg-gen-prompt"
              value={prompt}
              onChange={e => setPrompt(e.target.value)}
              rows={3}
              maxLength={1500}
              placeholder="A misty rooftop garden at dusk, string lights, the cat napping on a server rack"
              className="w-full px-3 py-2 rounded-lg bg-surface-1 border border-border-1 text-sm text-text-1 placeholder:text-text-3 focus:border-accent-primary focus:outline-none resize-y"
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label
                htmlFor="bg-gen-style"
                className="block text-xs font-medium text-text-2 mb-1.5"
              >
                Style reference
              </label>
              <select
                id="bg-gen-style"
                value={styleRef}
                onChange={e => setStyleRef(e.target.value)}
                className="w-full px-3 py-2 rounded-lg bg-surface-1 border border-border-1 text-sm text-text-1 focus:border-accent-primary focus:outline-none cursor-pointer"
              >
                {presets.map(p => (
                  <option key={p.url} value={p.url}>
                    {p.name}
                  </option>
                ))}
                {items.map(b => (
                  <option key={b.id} value={b.url}>
                    {b.name || 'Generated'}
                  </option>
                ))}
              </select>
              <p className="text-[11px] text-text-3 mt-1">Its art style gets copied.</p>
            </div>

            <div>
              <label
                htmlFor="bg-gen-agent"
                className="block text-xs font-medium text-text-2 mb-1.5"
              >
                Agent cameo
              </label>
              <select
                id="bg-gen-agent"
                value={agentSlug}
                onChange={e => setAgentSlug(e.target.value)}
                className="w-full px-3 py-2 rounded-lg bg-surface-1 border border-border-1 text-sm text-text-1 focus:border-accent-primary focus:outline-none cursor-pointer"
              >
                <option value="">None</option>
                {agents.map(a => (
                  <option key={a.slug} value={a.slug}>
                    {a.name}
                  </option>
                ))}
              </select>
              <p className="text-[11px] text-text-3 mt-1">
                Their avatar joins the mascot in the scene.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={generate}
              disabled={generating || !prompt.trim()}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-xl bg-accent-muted border-2 border-accent-primary text-accent-text text-sm font-medium transition-opacity hover:opacity-90 cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {generating ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : (
                <Sparkles className="w-4 h-4" />
              )}
              {generating ? 'Generating…' : 'Generate background'}
            </button>
            <p className="text-[11px] text-text-3">
              Uses your OpenRouter key. Takes up to a minute.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
