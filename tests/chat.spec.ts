import { test, expect } from '@playwright/test';

test.describe('Chat', () => {
  test('shows empty state when no chat is selected', async ({ page }) => {
    await page.goto('/chat');
    await page.evaluate(() => localStorage.removeItem('openpaw_active_thread'));
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('No chat selected')).toBeVisible({ timeout: 5_000 });
  });

  test('can create a new chat', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    await page.getByRole('button', { name: /new chat/i }).first().click();

    await expect(page.locator('textarea[placeholder*="Ask anything"]')).toBeVisible({ timeout: 5_000 });
  });

  test('can type a message in the input', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: /new chat/i }).first().click();

    const input = page.locator('textarea[placeholder*="Ask anything"]');
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill('Hello from Playwright test');
    await expect(input).toHaveValue('Hello from Playwright test');
  });

  test('send button is disabled when input is empty', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: /new chat/i }).first().click();

    const input = page.locator('textarea[placeholder*="Ask anything"]');
    await expect(input).toBeVisible({ timeout: 5_000 });

    // Send button should be disabled with empty input
    const sendBtn = page.locator('button:has(svg.lucide-arrow-up)');
    await expect(sendBtn).toBeDisabled();

    // Type something — send should be enabled
    await input.fill('test message');
    await expect(sendBtn).toBeEnabled();
  });

  test('can send a message and it appears in chat', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: /new chat/i }).first().click();

    const input = page.locator('textarea[placeholder*="Ask anything"]');
    await expect(input).toBeVisible({ timeout: 5_000 });

    await input.fill('Hello from E2E test');
    await input.press('Enter');

    await expect(page.getByText('Hello from E2E test')).toBeVisible({ timeout: 10_000 });
  });

  test('chat panel has search', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    const searchInput = page.locator('input[placeholder="Search chats..."]');
    await expect(searchInput).toBeVisible();
  });

  test('can rename a chat thread', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    // Create a new chat
    await page.getByRole('button', { name: /new chat/i }).first().click();

    // Send a message so the thread gets saved and appears in the sidebar
    const input = page.locator('textarea[placeholder*="Ask anything"]');
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill('Hello rename test');
    await input.press('Enter');
    await expect(page.getByText('Hello rename test')).toBeVisible({ timeout: 10_000 });

    // Open the thread list sidebar (may have closed after selecting thread)
    const showThreadsBtn = page.locator('button:has(svg.lucide-panel-left-open)');
    if (await showThreadsBtn.isVisible()) {
      await showThreadsBtn.click();
    }

    // Find the thread row in the sidebar — it is a group div wrapping the thread button.
    // The thread button has pr-16 and contains the thread title text.
    // We hover the row to reveal the Pencil button (opacity-0 → opacity-100 on group-hover).
    const threadRow = page.locator('div.group.relative.rounded-lg').first();
    await expect(threadRow).toBeVisible({ timeout: 5_000 });
    await threadRow.hover();

    // Click the Pencil (rename) button
    const pencilBtn = threadRow.locator('button:has(svg.lucide-pencil)');
    await expect(pencilBtn).toBeVisible({ timeout: 3_000 });
    await pencilBtn.click();

    // An inline text input should now be visible replacing the title
    const renameInput = threadRow.locator('input');
    await expect(renameInput).toBeVisible({ timeout: 3_000 });

    // Clear existing text and type the new name
    await renameInput.fill('My Renamed Thread');

    // Click the Check button to confirm
    const checkBtn = threadRow.locator('button:has(svg.lucide-check)');
    await expect(checkBtn).toBeVisible();
    await checkBtn.click();

    // The new thread title should appear in the sidebar
    await expect(page.getByText('My Renamed Thread')).toBeVisible({ timeout: 5_000 });
  });

  test('long message renders without overflow', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: /new chat/i }).first().click();

    const input = page.locator('textarea[placeholder*="Ask anything"]');
    await expect(input).toBeVisible({ timeout: 5_000 });

    // Generate a 500+ character message
    const longMessage = 'This is a long test message. '.repeat(20).trim();
    await input.fill(longMessage);
    await input.press('Enter');

    // Verify the message appears
    await expect(page.getByText(longMessage.substring(0, 30))).toBeVisible({ timeout: 10_000 });

    // Check no horizontal overflow
    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
    const clientWidth = await page.evaluate(() => document.documentElement.clientWidth);
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth + 5);
  });

  test('can delete a chat thread', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    // Create a new chat
    await page.getByRole('button', { name: /new chat/i }).first().click();

    // Send a message so the thread is saved and visible in the sidebar
    const input = page.locator('textarea[placeholder*="Ask anything"]');
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill('Hello delete test');
    await input.press('Enter');
    await expect(page.getByText('Hello delete test')).toBeVisible({ timeout: 10_000 });

    // Open the thread list sidebar (may have closed after selecting thread)
    const showThreadsBtn = page.locator('button:has(svg.lucide-panel-left-open)');
    if (await showThreadsBtn.isVisible()) {
      await showThreadsBtn.click();
    }

    // Count threads before deletion so we can verify one is removed
    const threadRows = page.locator('div.group.relative.rounded-lg');
    const countBefore = await threadRows.count();
    expect(countBefore).toBeGreaterThan(0);

    // Hover the first thread row to reveal the action buttons
    const firstRow = threadRows.first();
    await expect(firstRow).toBeVisible({ timeout: 5_000 });
    await firstRow.hover();

    // Click the Trash2 (delete) button — this opens the Delete Chat modal
    const trashBtn = firstRow.locator('button:has(svg.lucide-trash-2)');
    await expect(trashBtn).toBeVisible({ timeout: 3_000 });
    await trashBtn.click();

    // The Delete Chat modal should appear (NOT a native confirm dialog)
    const modal = page.getByRole('dialog', { name: /delete chat/i });
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Click the "Delete" danger button inside the modal to confirm deletion
    await modal.getByRole('button', { name: /^delete$/i }).click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 5_000 });

    // If we deleted the only thread (or the active one), the empty state should appear
    if (countBefore === 1) {
      await expect(page.getByText('No chat selected')).toBeVisible({ timeout: 5_000 });
    } else {
      // Otherwise, there should be one fewer thread in the sidebar
      await expect(threadRows).toHaveCount(countBefore - 1, { timeout: 5_000 });
    }
  });
});
