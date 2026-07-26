/**
 * ConfirmDialog — themed replacement for window.confirm.
 *
 * The native dialog is drawn by the browser, ignores the app's theme, and in
 * the Tauri window looks like a system error rather than part of OpenPaw.
 * It also blocks the event loop, which is a real problem for anything that
 * needs to keep rendering behind it.
 */

import type { ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';
import { Modal } from './Modal';
import { Button } from './Button';

export interface ConfirmDialogProps {
  open: boolean;
  title: string;
  /** The consequence, in plain language. Say what is lost and whether it returns. */
  message: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  /** Styles the action as destructive. Defaults to true — this is a confirm. */
  destructive?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Delete',
  cancelLabel = 'Cancel',
  destructive = true,
  busy = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <Modal open={open} onClose={onCancel} title={title} size="sm">
      <div className="flex gap-3">
        {destructive && (
          <span className="flex-shrink-0 flex items-center justify-center w-9 h-9 rounded-full bg-danger/10">
            <AlertTriangle className="w-4 h-4 text-danger" aria-hidden="true" />
          </span>
        )}
        <div className="flex-1 min-w-0 text-sm text-text-2 leading-relaxed">{message}</div>
      </div>

      <div className="flex justify-end gap-2 mt-5">
        <Button variant="secondary" onClick={onCancel} disabled={busy}>
          {cancelLabel}
        </Button>
        <Button
          onClick={onConfirm}
          loading={busy}
          className={destructive ? '!bg-danger hover:!bg-danger-hover' : ''}
        >
          {confirmLabel}
        </Button>
      </div>
    </Modal>
  );
}
