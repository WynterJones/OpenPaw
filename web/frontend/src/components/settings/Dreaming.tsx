/**
 * Dreaming — scheduled memory consolidation.
 *
 * Two switches, and normally only one of them should be on. Both read the same
 * conversations looking for the same facts:
 *
 * - Dreaming runs on a schedule and is the default. Its cost is bounded per run
 *   however busy the day was, and its consolidation pass is the half with no
 *   substitute — without it, capture is append-only and memory rots.
 * - Capture runs after every reply, catching the same material sooner at a model
 *   call per message. Off by default: on most exchanges it correctly finds
 *   nothing, and you paid for that answer.
 *
 * The panel says this out loud, because two toggles that quietly do overlapping
 * work at very different prices is exactly the kind of thing a user turns both
 * on and only discovers on the bill.
 *
 * The run history is shown here rather than on its own page because the numbers
 * (added / updated / forgotten) are the only way to tell a dream that tidied up
 * from one that quietly threw things away.
 */

import { useCallback, useEffect, useState } from 'react';
import { BrainCircuit, Loader2, Moon, Play, Save } from 'lucide-react';
import { Card } from '../Card';
import { Button } from '../Button';
import { Input, Select } from '../Input';
import { Toggle } from '../Toggle';
import { useToast } from '../Toast';
import { dreamingApi, type DreamingConfig, type DreamRun, type WSMessage } from '../../lib/api';
import { useWebSocket } from '../../lib/useWebSocket';
import { timeAgo } from '../../lib/chatUtils';

/** Must match the constants in internal/dreaming — the server parses these. */
const PRESETS = [
  { value: '0 0 * * * *', label: 'Hourly' },
  { value: '0 0 3 * * *', label: 'Daily at 3am' },
  { value: '0 0 3 * * 0', label: 'Weekly (Sunday, 3am)' },
  { value: '0 0 3 1 * *', label: 'Monthly (1st, 3am)' },
];

const CUSTOM = 'custom';

const isTrue = (v: string | undefined) => v === 'true' || v === '1';

export function Dreaming() {
  const { toast } = useToast();
  const [cfg, setCfg] = useState<DreamingConfig | null>(null);
  const [runs, setRuns] = useState<DreamRun[]>([]);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  // Tracked separately from cfg.dreaming_cron so typing a custom expression
  // doesn't get snapped back to a preset on every keystroke.
  const [mode, setMode] = useState<string>(PRESETS[1].value);

  const loadRuns = useCallback(() => {
    dreamingApi.listRuns({ limit: 20 }).then(setRuns).catch(() => {});
  }, []);

  useEffect(() => {
    dreamingApi
      .getConfig()
      .then((c) => {
        setCfg(c);
        setRunning(isTrue(c.dreaming_running));
        setMode(PRESETS.some((p) => p.value === c.dreaming_cron) ? c.dreaming_cron : CUSTOM);
      })
      .catch(() => toast('error', 'Could not load dreaming settings'));
    loadRuns();
  }, [loadRuns, toast]);

  // A dream runs for minutes and finishes with no page interaction, so without
  // this the panel would sit on a stale "running" state until a manual reload.
  useWebSocket({
    onMessage: (msg: WSMessage) => {
      if (msg.type === 'dreaming_state') {
        setRunning(!!(msg.payload as { running?: boolean })?.running);
      }
      if (msg.type === 'dream_run_finished') {
        loadRuns();
      }
    },
  });

  const patch = (next: Partial<DreamingConfig>) =>
    setCfg((prev) => (prev ? { ...prev, ...next } : prev));

  const save = async () => {
    if (!cfg) return;
    setSaving(true);
    try {
      const saved = await dreamingApi.updateConfig({
        dreaming_enabled: cfg.dreaming_enabled,
        dreaming_reflex_enabled: cfg.dreaming_reflex_enabled,
        dreaming_cron: cfg.dreaming_cron,
        dreaming_max_threads: cfg.dreaming_max_threads,
        dreaming_review_limit: cfg.dreaming_review_limit,
      });
      setCfg(saved);
      toast('success', 'Dreaming settings saved');
    } catch (err) {
      // The server rejects an unparseable cron rather than silently ignoring
      // it, so surface exactly what it said.
      toast('error', err instanceof Error ? err.message : 'Could not save dreaming settings');
    } finally {
      setSaving(false);
    }
  };

  const dreamNow = async () => {
    try {
      await dreamingApi.runNow();
      setRunning(true);
      toast('success', 'Dreaming started — this runs in the background');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Could not start dreaming');
    }
  };

  if (!cfg) {
    return (
      <Card>
        <div className="flex items-center gap-2 text-sm text-text-3">
          <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />
          Loading dreaming settings…
        </div>
      </Card>
    );
  }

  const enabled = isTrue(cfg.dreaming_enabled);

  return (
    <div className="space-y-6">
      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">Memory capture</h3>
        <div className="flex items-center justify-between gap-4 p-4 rounded-lg bg-surface-2">
          <div>
            <p className="text-sm font-medium text-text-1">Remember after every reply</p>
            <p className="text-xs text-text-3">
              After each answer, the gateway reviews what was said and saves anything worth
              keeping to that agent's memory — immediately, rather than waiting for the next
              dream.
            </p>
            <p className="text-xs text-text-3 mt-1.5">
              <strong className="text-text-2">Mostly redundant with dreaming</strong>, which
              already reads these same chats. It costs a model call on every message, and on
              most of them the honest answer is that there was nothing to save. Worth turning
              on only if agents recalling things the same day matters to you — or if your
              conversations run long enough that the middle gets truncated out of the nightly
              transcript.
            </p>
          </div>
          <Toggle
            enabled={isTrue(cfg.dreaming_reflex_enabled)}
            onChange={(v) => patch({ dreaming_reflex_enabled: v ? 'true' : 'false' })}
            label="Remember after every reply"
          />
        </div>
      </Card>

      <Card>
        <div className="flex items-start justify-between gap-4 mb-4">
          <div>
            <h3 className="text-sm font-semibold text-text-1 flex items-center gap-2">
              <Moon className="w-4 h-4 text-accent-primary" aria-hidden="true" />
              Dreaming
            </h3>
            <p className="text-xs text-text-3 mt-1 max-w-xl">
              On a schedule, each agent reads the chats it hasn't read yet, pulls the durable
              facts out of them, then reviews its existing memories — merging duplicates,
              correcting what changed, and dropping what no longer holds. Scanned chats are
              marked with a brain in the chat list and are not read again unless they continue.
              This is the main way agents remember; leave it on unless you have a reason not to.
            </p>
          </div>
          <Toggle
            enabled={enabled}
            onChange={(v) => patch({ dreaming_enabled: v ? 'true' : 'false' })}
            label="Dreaming"
          />
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <Select
            label="Frequency"
            value={mode}
            disabled={!enabled}
            onChange={(e) => {
              const next = e.target.value;
              setMode(next);
              if (next !== CUSTOM) patch({ dreaming_cron: next });
            }}
            options={[...PRESETS, { value: CUSTOM, label: 'Custom cron…' }]}
          />
          {mode === CUSTOM && (
            <Input
              label="Cron expression"
              value={cfg.dreaming_cron}
              disabled={!enabled}
              onChange={(e) => patch({ dreaming_cron: e.target.value })}
              placeholder="0 0 3 * * *"
            />
          )}
          <Input
            label="Chats read per agent, per dream"
            type="number"
            min={1}
            max={100}
            value={cfg.dreaming_max_threads}
            disabled={!enabled}
            onChange={(e) => patch({ dreaming_max_threads: e.target.value })}
          />
          <Input
            label="Memories reviewed per dream"
            type="number"
            min={10}
            max={500}
            value={cfg.dreaming_review_limit}
            disabled={!enabled}
            onChange={(e) => patch({ dreaming_review_limit: e.target.value })}
          />
        </div>

        <p className="text-xs text-text-3 mt-3">
          Dreaming runs on the Gateway model, not each agent's own — it is summarising, not
          working.
          {cfg.dreaming_next_run && enabled && (
            <> Next dream {new Date(cfg.dreaming_next_run).toLocaleString()}.</>
          )}
        </p>

        <div className="flex items-center gap-2 mt-4">
          <Button onClick={save} loading={saving} icon={<Save className="w-4 h-4" />}>
            {saving ? 'Saving…' : 'Save'}
          </Button>
          <Button
            variant="secondary"
            onClick={dreamNow}
            loading={running}
            disabled={running}
            icon={<Play className="w-4 h-4" />}
          >
            {running ? 'Dreaming…' : 'Dream now'}
          </Button>
        </div>
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">Recent dreams</h3>
        {runs.length === 0 ? (
          <p className="text-sm text-text-3">
            No dreams yet. Run one now, or turn dreaming on and wait for the schedule.
          </p>
        ) : (
          <ul className="space-y-2">
            {runs.map((run) => (
              <li key={run.id} className="p-3 rounded-lg bg-surface-2">
                <div className="flex items-center gap-2 flex-wrap">
                  <BrainCircuit
                    className={`w-4 h-4 flex-shrink-0 ${run.status === 'error' ? 'text-red-400' : 'text-accent-primary'}`}
                    aria-hidden="true"
                  />
                  <span className="text-sm font-medium text-text-1">{run.agent_name}</span>
                  <span className="text-xs text-text-3">{timeAgo(run.started_at)}</span>
                  {run.status !== 'error' && (
                    <span className="text-xs text-text-3 ml-auto font-mono">
                      +{run.memories_added} · ~{run.memories_updated} · −{run.memories_pruned}
                    </span>
                  )}
                </div>
                <p className={`text-xs mt-1 ${run.status === 'error' ? 'text-red-400' : 'text-text-3'}`}>
                  {run.status === 'error'
                    ? run.error || 'This dream failed.'
                    : run.summary ||
                      `Read ${run.threads_scanned} chat(s), found ${run.facts_found} fact(s).`}
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
