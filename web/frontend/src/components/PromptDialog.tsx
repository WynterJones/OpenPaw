/**
 * PromptDialog — themed replacement for window.prompt.
 *
 * window.prompt is a no-op inside the Tauri webview: it never shows anything
 * and returns null, so any flow built on it silently does nothing in the
 * desktop app while working fine in a browser.
 */

import { useState } from 'react';
import { Modal } from './Modal';
import { Button } from './Button';

interface Props {
  open: boolean;
  title: string;
  label?: string;
  placeholder?: string;
  /** Pre-filled value, e.g. the current name when renaming. */
  initialValue?: string;
  confirmLabel?: string;
  busy?: boolean;
  onConfirm: (value: string) => void;
  onCancel: () => void;
}

export function PromptDialog({
  open,
  title,
  label,
  placeholder,
  initialValue = '',
  confirmLabel = 'Save',
  busy = false,
  onConfirm,
  onCancel,
}: Props) {
  const [value, setValue] = useState(initialValue);

  // Reopening — for a different target, or the same one again — must not keep
  // the previous entry. Adjusted during render rather than in an effect, which
  // would cost an extra render pass and show the stale value for a frame.
  const openedFor = `${open}:${initialValue}`;
  const [lastOpenedFor, setLastOpenedFor] = useState(openedFor);
  if (openedFor !== lastOpenedFor) {
    setLastOpenedFor(openedFor);
    setValue(initialValue);
  }

  const submit = () => {
    const trimmed = value.trim();
    if (trimmed) onConfirm(trimmed);
  };

  return (
    <Modal open={open} onClose={onCancel} title={title} size="sm">
      <div className="space-y-1.5">
        {label && <label className="block text-sm font-medium text-text-1">{label}</label>}
        <input
          autoFocus
          value={value}
          placeholder={placeholder}
          onChange={e => setValue(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter') submit();
          }}
          className="block w-full rounded-lg border border-border-1 bg-surface-2 text-text-0 px-3 py-2 text-sm placeholder:text-text-3 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary outline-none"
        />
      </div>

      <div className="flex justify-end gap-2 mt-5">
        <Button variant="secondary" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button onClick={submit} loading={busy} disabled={!value.trim()}>
          {confirmLabel}
        </Button>
      </div>
    </Modal>
  );
}
