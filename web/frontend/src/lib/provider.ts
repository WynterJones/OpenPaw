import type { BalanceData } from '../hooks/useOpenRouterBalance';

// Context window for CLI subscription providers (Claude Code / Codex).
// These run on the local CLI (OpenAI/Claude) with a 1,000,000-token window.
export const CLI_CONTEXT_LIMIT = 1_000_000;

// Human-readable provider name, consistent across header, sidebar, and usage bar.
// `balance.subscription === true` means a local CLI provider (no per-token billing);
// otherwise the active provider is OpenRouter.
export function providerName(balance: BalanceData): string {
  if (balance.subscription) {
    if (balance.provider === 'codex') return 'Codex';
    if (balance.provider === 'claude-code') return 'Claude Code';
    return balance.provider ?? 'CLI';
  }
  return 'OpenRouter';
}
