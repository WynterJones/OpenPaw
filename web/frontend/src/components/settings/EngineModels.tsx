/**
 * EngineModels — a model choice per engine.
 *
 * A model id only means something to the engine it came from
 * ("anthropic/claude-sonnet-5" vs "sonnet" vs "gpt-5.5"), so one shared setting
 * could not survive a provider switch. Each engine stores its own pair, and
 * switching engines — here or from the composer — swaps to that engine's
 * models.
 *
 * The custom field exists because the CLI providers ship no discoverable model
 * list: the Codex binary has no "list models" command, so anything newer than
 * the curated tiers has to be typeable rather than waiting on a release here.
 */

import { useCallback, useEffect, useState } from 'react';
import { Cpu, Save, Loader2, Check } from 'lucide-react';
import { Card } from '../Card';
import { Button } from '../Button';
import { api } from '../../lib/api';
import { useToast } from '../Toast';

interface ProviderStatus {
  configured?: boolean;
  available?: boolean;
}

const LABELS: Record<string, string> = {
  openrouter: 'OpenRouter',
  'claude-code': 'Claude Code',
  codex: 'Codex',
};

const ORDER = ['openrouter', 'claude-code', 'codex'];

function usable(name: string, s: ProviderStatus | undefined): boolean {
  if (!s) return false;
  return name === 'openrouter' ? Boolean(s.configured) : Boolean(s.available ?? s.configured);
}

export function EngineModels() {
  const { toast } = useToast();

  const [active, setActive] = useState('openrouter');
  const [statuses, setStatuses] = useState<Record<string, ProviderStatus>>({});
  const [selected, setSelected] = useState('openrouter');

  const [models, setModels] = useState<{ id: string; name: string }[]>([]);
  const [loadingModels, setLoadingModels] = useState(false);
  const [gateway, setGateway] = useState('');
  const [builder, setBuilder] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    api
      .get<{ active: string; providers: Record<string, ProviderStatus> }>('/settings/llm-provider')
      .then(d => {
        setActive(d.active);
        setStatuses(d.providers ?? {});
        setSelected(d.active);
      })
      .catch(() => {});
  }, []);

  const loadFor = useCallback(async (provider: string) => {
    setLoadingModels(true);
    try {
      const [list, current] = await Promise.all([
        api
          .get<{ id: string; name: string }[]>(
            `/settings/available-models?provider=${encodeURIComponent(provider)}`,
          )
          .catch(() => []),
        api.get<{ gateway_model: string; builder_model: string }>(
          `/settings/models?provider=${encodeURIComponent(provider)}`,
        ),
      ]);
      setModels(Array.isArray(list) ? list : []);
      setGateway(current.gateway_model ?? '');
      setBuilder(current.builder_model ?? '');
    } catch {
      setModels([]);
    } finally {
      setLoadingModels(false);
    }
  }, []);

  useEffect(() => {
    loadFor(selected);
  }, [selected, loadFor]);

  const save = async () => {
    setSaving(true);
    try {
      await api.put('/settings/models', {
        provider: selected,
        gateway_model: gateway.trim(),
        builder_model: builder.trim(),
      });
      setSaved(true);
      setTimeout(() => setSaved(false), 1800);
      toast('success', `Saved for ${LABELS[selected] ?? selected}`);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Could not save');
    } finally {
      setSaving(false);
    }
  };

  const names = ORDER.filter(n => n in statuses);

  const field = (
    id: string,
    label: string,
    hint: string,
    value: string,
    onChange: (v: string) => void,
  ) => (
    <div className="space-y-1.5">
      <label htmlFor={id} className="block text-sm font-medium text-text-1">
        {label}
      </label>
      <p className="text-xs text-text-3 leading-relaxed">{hint}</p>
      <input
        id={id}
        list={`${id}-options`}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={loadingModels ? 'Loading…' : 'Model id'}
        className="block w-full rounded-lg border border-border-1 bg-surface-2 text-text-0 px-3 py-2 text-sm placeholder:text-text-3 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary outline-none"
      />
      {/* A datalist rather than a select: it suggests the known models while
          still accepting anything the engine supports. */}
      <datalist id={`${id}-options`}>
        {models.map(m => (
          <option key={m.id} value={m.id}>
            {m.name}
          </option>
        ))}
      </datalist>
    </div>
  );

  return (
    <Card>
      <div className="flex items-center gap-3 mb-1">
        <div className="w-8 h-8 rounded-lg bg-accent-muted flex items-center justify-center">
          <Cpu className="w-4 h-4 text-accent-primary" />
        </div>
        <div>
          <h3 className="text-sm font-semibold text-text-1">Models per engine</h3>
          <p className="text-xs text-text-3">
            Each engine remembers its own models. Switching engine switches models with it.
          </p>
        </div>
      </div>

      <div
        role="radiogroup"
        aria-label="Engine"
        className="flex items-center gap-1 rounded-lg border border-border-0 bg-surface-2 p-1 my-4"
      >
        {names.map(name => {
          const isSel = selected === name;
          const ok = usable(name, statuses[name]);
          return (
            <button
              key={name}
              role="radio"
              aria-checked={isSel}
              onClick={() => setSelected(name)}
              title={ok ? undefined : `${LABELS[name] ?? name} is not installed or not logged in`}
              className={`flex-1 flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium transition-colors cursor-pointer ${
                isSel ? 'bg-accent-primary/15 text-accent-text' : 'text-text-2 hover:text-text-1 hover:bg-surface-3'
              } ${ok ? '' : 'opacity-50'}`}
            >
              {LABELS[name] ?? name}
              {name === active && (
                <span className="text-[9px] uppercase tracking-wide text-text-3">in use</span>
              )}
            </button>
          );
        })}
      </div>

      {!usable(selected, statuses[selected]) && (
        <p className="rounded-lg border border-border-0 bg-surface-2 px-3 py-2 mb-4 text-[11px] text-text-2 leading-relaxed">
          {LABELS[selected] ?? selected} is not available right now. You can still set its models —
          they apply once it is installed and logged in.
        </p>
      )}

      <div className="space-y-4 max-w-md">
        {field(
          'engine-gateway-model',
          'Gateway model',
          'Routes every message and writes summaries. Runs constantly, so a cheaper model pays off — but too weak a model routes badly.',
          gateway,
          setGateway,
        )}
        {field(
          'engine-builder-model',
          'Builder model',
          'Does the actual work: writing code, running microservices, long tasks. Worth the strongest model you are willing to pay for.',
          builder,
          setBuilder,
        )}

        <Button
          onClick={save}
          loading={saving}
          disabled={!gateway.trim() || !builder.trim()}
          icon={
            saved ? (
              <Check className="w-4 h-4" />
            ) : saving ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Save className="w-4 h-4" />
            )
          }
        >
          {saved ? 'Saved' : `Save for ${LABELS[selected] ?? selected}`}
        </Button>
      </div>
    </Card>
  );
}
