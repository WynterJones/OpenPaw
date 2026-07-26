/**
 * Studio provider keys.
 *
 * OpenRouter covers image generation, but it has no video or music models at
 * all — those need one of these providers. Keys are stored encrypted and are
 * never returned to the browser.
 *
 * Lives in its own file (and its own page) rather than buried in the AI section
 * of Settings: it is the thing you go looking for the moment Studio says a
 * media type is unavailable, and it was three screens down a page about models.
 */

import { useCallback, useEffect, useState } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { api } from '../../lib/api';
import { useToast } from '../Toast';

type MediaKeyState = Record<string, { configured: boolean; source: string }>;

const MEDIA_PROVIDERS = [
  {
    id: 'replicate',
    name: 'Replicate',
    blurb:
      'Video and music generation, plus extra image models. Get a token at replicate.com/account/api-tokens.',
  },
  {
    id: 'fal',
    name: 'fal.ai',
    blurb: 'Faster, cheaper video and audio. Get a key at fal.ai/dashboard/keys.',
  },
  {
    id: 'elevenlabs',
    name: 'ElevenLabs',
    blurb:
      "Text to speech. Your voices appear in Studio's Audio model picker — the prompt becomes the script. Get a key at elevenlabs.io.",
  },
];

export function StudioProviders({ showHeading = true }: { showHeading?: boolean }) {
  const { toast } = useToast();
  const [state, setState] = useState<MediaKeyState>({});
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState<string | null>(null);

  const reload = useCallback(() => {
    api.get<MediaKeyState>('/settings/media-keys').then(setState).catch(() => {});
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const saveKey = async (id: string) => {
    setSaving(id);
    try {
      await api.put(`/settings/media-keys/${id}`, { api_key: inputs[id] ?? '' });
      setInputs(p => ({ ...p, [id]: '' }));
      reload();
      toast('success', inputs[id]?.trim() ? 'Key saved' : 'Key removed');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to save key');
    } finally {
      setSaving(null);
    }
  };

  return (
    <Card>
      {showHeading && (
        <>
          <h3 className="text-sm font-semibold text-text-1 mb-1">Media Providers</h3>
          <p className="text-xs text-text-3 mb-4">
            OpenRouter generates images. Video, music and voice need one of these — add any of
            them, then pick per generation in Studio. Stored encrypted, never sent to the browser.
          </p>
        </>
      )}

      <div className="space-y-4 max-w-md">
        {MEDIA_PROVIDERS.map(p => {
          const st = state[p.id];
          const fromEnv = st?.source === 'env';
          return (
            <div key={p.id} className="p-3 rounded-lg border border-border-0 bg-surface-2">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-sm font-medium text-text-1">{p.name}</span>
                <span
                  className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${
                    st?.configured ? 'bg-green-500/10 text-green-400' : 'bg-surface-3 text-text-3'
                  }`}
                >
                  {st?.configured ? (fromEnv ? 'Set via env' : 'Configured') : 'Not set'}
                </span>
              </div>
              <p className="text-[11px] text-text-3 mb-2.5 leading-relaxed">{p.blurb}</p>
              {fromEnv ? (
                <p className="text-[11px] text-text-3">
                  Set by an environment variable, which takes priority over anything saved here.
                </p>
              ) : (
                <div className="flex gap-2">
                  <input
                    type="password"
                    value={inputs[p.id] ?? ''}
                    onChange={e => setInputs(prev => ({ ...prev, [p.id]: e.target.value }))}
                    placeholder={
                      st?.configured ? 'Replace key (blank to remove)' : `${p.name} API key`
                    }
                    className="flex-1 rounded-lg border border-border-1 bg-surface-0 text-text-0 px-3 py-2 text-sm placeholder:text-text-3 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary outline-none"
                  />
                  <Button
                    onClick={() => saveKey(p.id)}
                    loading={saving === p.id}
                    variant="secondary"
                    size="sm"
                  >
                    Save
                  </Button>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </Card>
  );
}
