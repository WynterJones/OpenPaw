/**
 * CompanionWizard
 *
 * Modal wizard for creating a pixel-art companion via PixelLab:
 *   apikey -> describe -> pick (3 options) -> animate (4 emotes) -> manage.
 * The finished character is saved to the server and can be pinned as a floating
 * chat companion, assigned to an agent, or extended with more emotes.
 */

import { useEffect, useState } from 'react';
import { Loader2, Sparkles, Pin, PinOff, Plus, Check, ExternalLink } from 'lucide-react';
import { Modal } from '../Modal';
import { Button } from '../Button';
import { Input, Textarea, Select } from '../Input';
import { useToast } from '../Toast';
import { api } from '../../lib/api';
import {
  companionStore,
  DEFAULT_EMOTES,
  type PixelLabCharacter,
} from '../../lib/companionStore';
import {
  getBalance,
  createPixelImageOptions,
  createCharacter,
  pollJob,
  animateAndCollect,
  PixelLabError,
  type BalanceInfo,
} from '../../lib/pixellabClient';
import { SpriteAnimation } from './SpriteAnimation';
import type { AgentRole } from '../../lib/types';

interface CompanionWizardProps {
  open: boolean;
  onClose: () => void;
}

type Step = 'apikey' | 'describe' | 'pick' | 'animate' | 'manage';

interface EmoteProgress {
  name: string;
  status: 'pending' | 'running' | 'done' | 'error';
}

function errorMessage(e: unknown): string {
  if (e instanceof PixelLabError) {
    switch (e.code) {
      case 'auth':
        return 'Invalid API key — check your PixelLab key and try again.';
      case 'credits':
        return 'Not enough PixelLab credits for this request.';
      case 'rate_limit':
        return 'PixelLab is rate-limiting requests — wait a moment and retry.';
      default:
        return e.message || 'PixelLab request failed.';
    }
  }
  return e instanceof Error ? e.message : String(e);
}

export function CompanionWizard({ open, onClose }: CompanionWizardProps) {
  const { toast } = useToast();

  const [step, setStep] = useState<Step>('apikey');
  const [keyDraft, setKeyDraft] = useState('');
  const [balance, setBalance] = useState<BalanceInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [options, setOptions] = useState<string[]>([]);
  const [selected, setSelected] = useState<number | null>(null);

  const [emoteProgress, setEmoteProgress] = useState<EmoteProgress[]>([]);
  const [saved, setSaved] = useState<PixelLabCharacter | null>(null);
  const [newEmote, setNewEmote] = useState('');
  const [agentRoles, setAgentRoles] = useState<AgentRole[]>([]);

  // On open, decide whether the API key step is needed.
  useEffect(() => {
    if (!open) return;
    setError(null);
    setBusy(false);
    api
      .get<{ configured: boolean }>('/settings/pixellab-api-key')
      .then((d) => setStep(d.configured ? 'describe' : 'apikey'))
      .catch(() => setStep('apikey'));
    api.get<AgentRole[]>('/agent-roles').then(setAgentRoles).catch(() => {});
  }, [open]);

  const handleSaveKey = async () => {
    setError(null);
    setBusy(true);
    try {
      await api.put('/settings/pixellab-api-key', { api_key: keyDraft.trim() });
      const info = await getBalance().catch(() => null);
      if (info) setBalance(info);
      setStep('describe');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save key');
    } finally {
      setBusy(false);
    }
  };

  const handleGenerate = async () => {
    if (!description.trim()) return;
    setError(null);
    setBusy(true);
    setOptions([]);
    setSelected(null);
    setStep('pick');
    try {
      const imgs = await createPixelImageOptions({
        description: `pixel art character, ${description.trim()}`,
      });
      setOptions(imgs);
    } catch (e) {
      setError(errorMessage(e));
      setStep('describe');
    } finally {
      setBusy(false);
    }
  };

  const handleAnimate = async () => {
    if (selected === null) return;
    const sprite = options[selected];
    setError(null);
    setBusy(true);
    setStep('animate');
    setEmoteProgress(DEFAULT_EMOTES.map((n) => ({ name: n, status: 'pending' })));

    try {
      const job = await createCharacter({
        description: `pixel art character, ${description.trim()}`,
        referenceImage: sprite,
      });
      await pollJob(job.jobId);
      const characterId = job.characterId;
      if (!characterId) throw new PixelLabError(0, 'PixelLab did not return a character id');

      const animations: { name: string; fps: number; frames: string[] }[] = [];
      for (const emote of DEFAULT_EMOTES) {
        setEmoteProgress((prev) =>
          prev.map((p) => (p.name === emote ? { ...p, status: 'running' } : p))
        );
        try {
          const frames = await animateAndCollect({
            characterId,
            action: emote === 'idle' ? 'idle breathing' : emote,
          });
          animations.push({
            name: emote,
            fps: emote === 'idle' ? 4 : 6,
            frames: frames.length > 0 ? frames : [sprite],
          });
          setEmoteProgress((prev) =>
            prev.map((p) => (p.name === emote ? { ...p, status: 'done' } : p))
          );
        } catch {
          animations.push({ name: emote, fps: 4, frames: [sprite] });
          setEmoteProgress((prev) =>
            prev.map((p) => (p.name === emote ? { ...p, status: 'error' } : p))
          );
        }
      }

      const character = await api.post<PixelLabCharacter>('/pixellab/characters', {
        name: name.trim() || 'Companion',
        pixellab_id: characterId,
        base_sprite: sprite,
        animations,
      });
      await companionStore.load();
      setSaved(character);
      setStep('manage');
      toast('success', 'Companion created');
    } catch (e) {
      setError(errorMessage(e));
      setStep('pick');
    } finally {
      setBusy(false);
    }
  };

  const refreshSaved = async () => {
    if (!saved) return;
    const list = await companionStore.load();
    const updated = list.find((c) => c.id === saved.id);
    if (updated) setSaved(updated);
  };

  const handleAddEmote = async () => {
    if (!newEmote.trim() || !saved) return;
    const action = newEmote.trim();
    setError(null);
    setBusy(true);
    try {
      const frames = await animateAndCollect({ characterId: saved.pixellab_id, action });
      await api.post(`/pixellab/characters/${saved.id}/animations`, {
        name: action,
        fps: 6,
        frames: frames.length > 0 ? frames : saved.base_url ? [saved.base_url] : [],
      });
      await refreshSaved();
      setNewEmote('');
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const togglePin = async () => {
    if (!saved) return;
    await api.put(`/pixellab/characters/${saved.id}`, { pinned: !saved.pinned });
    await refreshSaved();
  };

  const assignAgent = async (slug: string) => {
    if (!saved) return;
    await api.put(`/pixellab/characters/${saved.id}`, { agent_slug: slug });
    await refreshSaved();
  };

  const startOver = () => {
    setSaved(null);
    setName('');
    setDescription('');
    setOptions([]);
    setSelected(null);
    setStep('describe');
  };

  return (
    <Modal open={open} onClose={onClose} title="Create companion" size="xl">
      <div className="flex flex-col gap-4">
        {error && (
          <button
            onClick={() => setError(null)}
            className="text-left p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-sm text-red-400"
          >
            {error}
          </button>
        )}

        {step === 'apikey' && (
          <div className="flex flex-col gap-3 max-w-lg">
            <p className="text-sm text-text-3">
              Paste your PixelLab API key to start creating pixel-art companions.
            </p>
            <Input
              type="password"
              placeholder="PixelLab API key"
              value={keyDraft}
              onChange={(e) => setKeyDraft(e.target.value)}
            />
            <div className="flex items-center gap-3">
              <Button onClick={handleSaveKey} loading={busy} disabled={!keyDraft.trim()}>
                Validate & Continue
              </Button>
              <a
                href="https://www.pixellab.ai/account"
                target="_blank"
                rel="noreferrer"
                className="text-xs text-accent-primary inline-flex items-center gap-1 hover:underline"
              >
                Get a key <ExternalLink className="w-3 h-3" />
              </a>
            </div>
          </div>
        )}

        {step === 'describe' && (
          <div className="flex flex-col gap-3 max-w-lg">
            {balance?.generations !== undefined && (
              <p className="text-xs text-text-3">
                {balance.generations} generations remaining
                {balance.plan ? ` · ${balance.plan}` : ''}
              </p>
            )}
            <Input label="Name" placeholder="Sir Reginald" value={name} onChange={(e) => setName(e.target.value)} />
            <Textarea
              label="Describe your companion"
              placeholder="a brave knight with a green cloak and a tiny sword"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
            />
            <div>
              <Button onClick={handleGenerate} loading={busy} disabled={!description.trim()} icon={<Sparkles className="w-4 h-4" />}>
                Generate 3 options
              </Button>
            </div>
          </div>
        )}

        {step === 'pick' && (
          <div className="flex flex-col gap-3">
            <p className="text-sm text-text-3">Pick your favourite, then animate it.</p>
            <div className="grid grid-cols-3 gap-3">
              {busy && options.length === 0
                ? Array.from({ length: 3 }).map((_, i) => (
                    <div key={i} className="aspect-square rounded-lg border border-border-1 bg-surface-2 flex items-center justify-center">
                      <Loader2 className="w-6 h-6 animate-spin text-text-3" />
                    </div>
                  ))
                : options.map((img, i) => (
                    <button
                      key={i}
                      onClick={() => setSelected(i)}
                      className={`relative aspect-square rounded-lg border-2 bg-surface-2 flex items-center justify-center transition-colors ${
                        selected === i ? 'border-accent-primary' : 'border-border-1 hover:border-accent-primary/50'
                      }`}
                    >
                      <img src={img} alt={`Option ${i + 1}`} className="w-3/4 h-3/4 object-contain" style={{ imageRendering: 'pixelated' }} />
                      {selected === i && (
                        <span className="absolute top-1 right-1 w-5 h-5 rounded-full bg-accent-primary flex items-center justify-center">
                          <Check className="w-3 h-3 text-white" />
                        </span>
                      )}
                    </button>
                  ))}
            </div>
            <div className="flex items-center gap-2">
              <Button onClick={handleAnimate} loading={busy} disabled={selected === null} icon={<Sparkles className="w-4 h-4" />}>
                Animate (4 emotes)
              </Button>
              <Button variant="ghost" onClick={handleGenerate} disabled={busy}>
                Regenerate
              </Button>
            </div>
          </div>
        )}

        {step === 'animate' && (
          <div className="flex flex-col gap-3 max-w-sm">
            <p className="text-sm text-text-3">Animating your companion…</p>
            <ul className="flex flex-col gap-2">
              {emoteProgress.map((p) => (
                <li key={p.name} className="flex items-center gap-2 text-sm text-text-1">
                  {p.status === 'running' && <Loader2 className="w-4 h-4 animate-spin text-accent-primary" />}
                  {p.status === 'done' && <Check className="w-4 h-4 text-green-400" />}
                  {p.status === 'error' && <span className="w-4 h-4 text-red-400">!</span>}
                  {p.status === 'pending' && <span className="w-4 h-4 text-text-3">·</span>}
                  <span className="capitalize">{p.name}</span>
                </li>
              ))}
            </ul>
          </div>
        )}

        {step === 'manage' && saved && (
          <div className="flex flex-col gap-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-text-0">{saved.name}</h3>
              <Button variant={saved.pinned ? 'secondary' : 'primary'} onClick={togglePin} icon={saved.pinned ? <PinOff className="w-4 h-4" /> : <Pin className="w-4 h-4" />}>
                {saved.pinned ? 'Unpin' : 'Pin to chat'}
              </Button>
            </div>

            <div className="grid grid-cols-4 gap-3">
              {saved.animations.map((clip) => (
                <div key={clip.id} className="flex flex-col items-center gap-1 rounded-lg border border-border-1 bg-surface-2 p-2">
                  <SpriteAnimation frames={clip.frames} fps={clip.fps} size={64} />
                  <span className="text-xs capitalize text-text-3">{clip.name}</span>
                </div>
              ))}
            </div>

            <Select
              label="React as agent (optional)"
              value={saved.agent_slug}
              onChange={(e) => assignAgent(e.target.value)}
              options={[
                { value: '', label: 'Any agent (global)' },
                ...agentRoles.map((a) => ({ value: a.slug, label: a.name })),
              ]}
            />

            <div className="flex flex-col gap-2 border-t border-border-0 pt-3">
              <label className="text-xs text-text-3">Add another emote</label>
              <div className="flex items-center gap-2">
                <Input
                  placeholder="dance, sit, facepalm…"
                  value={newEmote}
                  onChange={(e) => setNewEmote(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleAddEmote()}
                  disabled={busy}
                />
                <Button onClick={handleAddEmote} loading={busy} disabled={!newEmote.trim()} icon={<Plus className="w-4 h-4" />}>
                  Add
                </Button>
              </div>
            </div>

            <div className="flex items-center gap-2 border-t border-border-0 pt-3">
              <Button variant="ghost" onClick={startOver}>
                Create another
              </Button>
              <Button onClick={onClose}>Done</Button>
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
