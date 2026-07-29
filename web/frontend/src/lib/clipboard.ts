// execCommand is deprecated, but remains the only clipboard option in older
// browsers and non-secure contexts. Keep it as a fallback for copy failures.
export function copyWithExecCommand(value: string): boolean {
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  textarea.style.pointerEvents = 'none';
  document.body.appendChild(textarea);
  textarea.select();
  textarea.setSelectionRange(0, value.length);

  try {
    return document.execCommand('copy');
  } finally {
    textarea.remove();
  }
}

/** Copy text already available during a user gesture, with a legacy fallback. */
export async function copyText(value: string): Promise<void> {
  let clipboardError: unknown;

  try {
    if (!navigator.clipboard?.writeText) throw new Error('Clipboard API unavailable');
    await navigator.clipboard.writeText(value);
    return;
  } catch (err) {
    clipboardError = err;
  }

  if (!copyWithExecCommand(value)) {
    throw clipboardError instanceof Error ? clipboardError : new Error('Could not copy text');
  }
}
