/**
 * RemoteAccessCard — reach this OpenPaw from another device.
 *
 * Works in both the desktop app and the npx build. The server always keeps a
 * loopback listener and *adds* one for the chosen mode, so turning this on can
 * never cut the app off from its own backend.
 *
 * Tailscale is the recommended mode because it opens a listener bound to the
 * tailnet address alone — devices on the same café wifi cannot see it. "Local
 * network" binds every interface, which is broader than most people expect.
 */

import { useCallback, useEffect, useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { Globe, Copy, Check, AlertTriangle, Smartphone } from 'lucide-react';
import { Card } from './Card';
import { Button } from './Button';
import { api } from '../lib/api';
import { useToast } from './Toast';
import type { SystemInfo } from '../lib/types';

type Mode = 'off' | 'tailscale' | 'lan';

const MODES: { value: Mode; label: string; blurb: string }[] = [
  { value: 'off', label: 'Off', blurb: 'This machine only. Nothing else can connect.' },
  {
    value: 'tailscale',
    label: 'Tailscale',
    blurb: 'Reachable from your devices on the tailnet, and nothing else. Recommended.',
  },
  {
    value: 'lan',
    label: 'Local network',
    blurb: 'Reachable from anything on the same network — including untrusted wifi.',
  },
];

export function RemoteAccessCard() {
  const { toast } = useToast();
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [mode, setMode] = useState<Mode>('off');
  const [saving, setSaving] = useState(false);
  const [copied, setCopied] = useState(false);
  const [dirty, setDirty] = useState(false);

  const load = useCallback(() => {
    api
      .get<SystemInfo>('/system/info')
      .then(data => {
        setInfo(data);
        setMode((data.remote_access as Mode) ?? 'off');
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const save = async (next: Mode) => {
    setMode(next);
    setSaving(true);
    try {
      await api.put('/settings', { remote_access: next });
      // The listener set is decided at startup, so the new mode is stored but
      // not live until the server restarts. Say so rather than showing a URL
      // that will not answer yet.
      setDirty(true);
      load();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Could not save');
      load();
    } finally {
      setSaving(false);
    }
  };

  const url = info?.remote_url ?? '';
  const live = Boolean(info?.remote_active) && !dirty;

  const copy = () => {
    if (!url) return;
    navigator.clipboard.writeText(url).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      },
      () => toast('error', 'Could not copy'),
    );
  };

  const tailscaleMissing = mode === 'tailscale' && !info?.tailscale_ip;

  return (
    <Card>
      <div className="flex items-center gap-3 mb-4">
        <div className="w-8 h-8 rounded-lg bg-accent-muted flex items-center justify-center">
          <Smartphone className="w-4 h-4 text-accent-primary" />
        </div>
        <div>
          <h3 className="text-sm font-semibold text-text-1">Remote access</h3>
          <p className="text-xs text-text-3">Open OpenPaw from your phone or another computer</p>
        </div>
      </div>

      <div className="space-y-2 mb-4">
        {MODES.map(m => (
          <button
            key={m.value}
            onClick={() => save(m.value)}
            disabled={saving}
            className={`w-full flex items-start gap-3 rounded-lg border px-3 py-2.5 text-left transition-colors cursor-pointer disabled:opacity-60 ${
              mode === m.value
                ? 'border-accent-primary bg-accent-primary/10'
                : 'border-border-0 hover:bg-surface-2'
            }`}
          >
            <span
              className={`mt-0.5 w-3.5 h-3.5 rounded-full border flex-shrink-0 flex items-center justify-center ${
                mode === m.value ? 'border-accent-primary' : 'border-border-1'
              }`}
            >
              {mode === m.value && <span className="w-1.5 h-1.5 rounded-full bg-accent-primary" />}
            </span>
            <span className="min-w-0">
              <span
                className={`block text-sm font-medium ${
                  mode === m.value ? 'text-accent-text' : 'text-text-1'
                }`}
              >
                {m.label}
              </span>
              <span className="block text-[11px] text-text-3 leading-relaxed">{m.blurb}</span>
            </span>
          </button>
        ))}
      </div>

      {tailscaleMissing && (
        <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 mb-4 text-[11px] text-amber-400 leading-relaxed">
          <AlertTriangle className="w-3.5 h-3.5 flex-shrink-0 mt-0.5" aria-hidden="true" />
          <span>
            No tailnet address found. Install Tailscale and sign in on this machine, then restart
            OpenPaw. Until then it stays on localhost.
          </span>
        </div>
      )}

      {mode === 'lan' && (
        <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 mb-4 text-[11px] text-amber-400 leading-relaxed">
          <AlertTriangle className="w-3.5 h-3.5 flex-shrink-0 mt-0.5" aria-hidden="true" />
          <span>
            Every device on your current network can reach the login page. There is a password, but
            prefer Tailscale on networks you do not control.
          </span>
        </div>
      )}

      {dirty && mode !== 'off' && (
        <p className="rounded-lg border border-border-0 bg-surface-2 px-3 py-2 mb-4 text-[11px] text-text-2 leading-relaxed">
          Saved. <span className="text-text-1">Restart OpenPaw</span> to open the new listener — the
          address is chosen at startup.
        </p>
      )}

      {live && url && (
        <div className="flex flex-col sm:flex-row items-center gap-4 rounded-xl border border-border-0 bg-surface-2 p-4">
          {/* White plate: a QR rendered on a dark surface will not scan. */}
          <div className="bg-white p-2 rounded-lg flex-shrink-0">
            <QRCodeSVG value={url} size={116} level="M" />
          </div>
          <div className="min-w-0 flex-1 text-center sm:text-left">
            <p className="text-[11px] text-text-3 mb-1">Scan from your phone, or open</p>
            <p className="text-sm font-mono text-text-0 break-all mb-2">{url}</p>
            <Button
              size="sm"
              variant="secondary"
              onClick={copy}
              icon={copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
            >
              {copied ? 'Copied' : 'Copy link'}
            </Button>
            <p className="text-[11px] text-text-3 mt-2 leading-relaxed">
              {mode === 'tailscale'
                ? 'Your phone needs the Tailscale app, signed in to the same tailnet.'
                : 'Your phone must be on this same network.'}
            </p>
          </div>
        </div>
      )}

      {mode === 'off' && (
        <p className="flex items-center gap-2 text-[11px] text-text-3">
          <Globe className="w-3.5 h-3.5 flex-shrink-0" aria-hidden="true" />
          OpenPaw is only reachable from this machine.
        </p>
      )}
    </Card>
  );
}
