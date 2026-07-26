/**
 * ProviderSwitcher — pick the engine from the composer.
 *
 * Which provider is answering changes cost, speed and capability, so it
 * belongs next to the prompt rather than buried in Settings. Each provider
 * carries its own model choice (an id only means something to the provider it
 * came from), so switching here also swaps the model — the menu shows which,
 * so the consequence is visible before you commit to it.
 *
 * CLI providers appear only when their binary is installed and logged in;
 * unavailable ones stay listed but disabled, since "Codex is missing" is more
 * useful than Codex silently not existing.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { Check, ChevronDown, Cpu, Loader2 } from 'lucide-react';
import { api } from '../../lib/api';
import { useToast } from '../Toast';

interface ProviderStatus {
  configured?: boolean;
  available?: boolean;
  authenticated?: boolean;
  source?: string;
}

interface ProviderInfo {
  active: string;
  providers: Record<string, ProviderStatus>;
}

const LABELS: Record<string, string> = {
  openrouter: 'OpenRouter',
  'claude-code': 'Claude Code',
  codex: 'Codex',
};

const ORDER = ['openrouter', 'claude-code', 'codex'];

function labelFor(name: string) {
  return LABELS[name] ?? name;
}

/** A provider is usable when it has a key (OpenRouter) or a working CLI. */
function isUsable(name: string, s: ProviderStatus | undefined): boolean {
  if (!s) return false;
  if (name === 'openrouter') return Boolean(s.configured);
  return Boolean(s.available ?? s.configured);
}

/** Short model name — the picker is narrow and the vendor prefix is noise. */
function shortModel(id: string) {
  if (!id) return '';
  const slash = id.lastIndexOf('/');
  return slash >= 0 ? id.slice(slash + 1) : id;
}

export function ProviderSwitcher({ onChanged }: { onChanged?: () => void }) {
  const { toast } = useToast();
  const [info, setInfo] = useState<ProviderInfo | null>(null);
  const [models, setModels] = useState<Record<string, string>>({});
  const [open, setOpen] = useState(false);
  const [switching, setSwitching] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  const load = useCallback(async () => {
    try {
      const data = await api.get<ProviderInfo>('/settings/llm-provider');
      setInfo(data);

      // Each provider's own model, so the menu can show what switching would
      // actually run rather than just a provider name.
      const names = Object.keys(data.providers ?? {});
      const pairs = await Promise.all(
        names.map(async n => {
          try {
            const m = await api.get<{ gateway_model: string }>(
              `/settings/models?provider=${encodeURIComponent(n)}`,
            );
            return [n, m.gateway_model] as const;
          } catch {
            return [n, ''] as const;
          }
        }),
      );
      setModels(Object.fromEntries(pairs));
    } catch {
      /* leave the control hidden rather than showing a broken state */
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, []);

  const switchTo = async (name: string) => {
    if (!info || name === info.active) {
      setOpen(false);
      return;
    }
    setSwitching(name);
    try {
      await api.put('/settings/llm-provider', { provider: name });
      await load();
      setOpen(false);
      toast('success', `Switched to ${labelFor(name)}`);
      onChanged?.();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Could not switch provider');
    } finally {
      setSwitching(null);
    }
  };

  if (!info) return null;

  const names = ORDER.filter(n => n in (info.providers ?? {}));
  const activeModel = shortModel(models[info.active] ?? '');

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(o => !o)}
        aria-haspopup="menu"
        aria-expanded={open}
        title="Change the model answering this chat"
        className="flex items-center gap-1.5 px-2 py-1 rounded-lg text-[11px] font-medium text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer max-w-[190px]"
      >
        <Cpu className="w-3.5 h-3.5 flex-shrink-0" aria-hidden="true" />
        <span className="truncate">
          {labelFor(info.active)}
          {activeModel && <span className="text-text-3/70"> · {activeModel}</span>}
        </span>
        <ChevronDown className="w-3 h-3 flex-shrink-0" aria-hidden="true" />
      </button>

      {open && (
        <div
          role="menu"
          className="absolute bottom-full left-0 mb-1.5 w-64 rounded-xl border border-border-0 bg-surface-1 shadow-2xl py-1 z-50"
        >
          <p className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-text-3">
            Engine
          </p>
          {names.map(name => {
            const usable = isUsable(name, info.providers[name]);
            const active = name === info.active;
            const model = shortModel(models[name] ?? '');
            return (
              <button
                key={name}
                role="menuitem"
                disabled={!usable || switching !== null}
                onClick={() => switchTo(name)}
                title={usable ? undefined : `${labelFor(name)} is not installed or not logged in`}
                className={`w-full flex items-center gap-2 px-3 py-2 text-left transition-colors ${
                  usable
                    ? 'cursor-pointer hover:bg-surface-2'
                    : 'opacity-40 cursor-not-allowed'
                }`}
              >
                <span className="w-3.5 flex-shrink-0">
                  {switching === name ? (
                    <Loader2 className="w-3.5 h-3.5 animate-spin text-accent-text" />
                  ) : active ? (
                    <Check className="w-3.5 h-3.5 text-accent-text" />
                  ) : null}
                </span>
                <span className="min-w-0 flex-1">
                  <span
                    className={`block text-sm truncate ${active ? 'text-accent-text' : 'text-text-1'}`}
                  >
                    {labelFor(name)}
                  </span>
                  <span className="block text-[11px] text-text-3 truncate">
                    {usable ? model || 'default model' : 'not available'}
                  </span>
                </span>
              </button>
            );
          })}
          <div className="border-t border-border-0 mt-1 pt-1">
            <p className="px-3 py-1 text-[10px] text-text-3 leading-relaxed">
              Each engine keeps its own model. Change them in Settings → AI Models.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
