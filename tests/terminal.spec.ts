import { test, expect } from '@playwright/test';

test.describe('Global terminal', () => {
  test('keeps the open terminal when the app workspace changes', async ({ page }) => {
    await page.goto('/terminal');

    const newTerminal = page.locator('button.btn-primary').filter({ hasText: 'New Terminal' });
    const xterm = page.locator('.openpaw-terminal .xterm');
    await Promise.race([
      newTerminal.waitFor({ state: 'visible' }),
      xterm.waitFor({ state: 'visible' }),
    ]);
    if (await newTerminal.isVisible()) await newTerminal.click();
    await expect(xterm).toBeVisible();

    await page.getByTitle('Switch workspace').click();
    await page.getByRole('menu').getByRole('button', { name: 'New workspace' }).click();
    await page.getByPlaceholder('Workspace name').fill('Terminal regression workspace');
    await page.getByRole('button', { name: 'Create', exact: true }).click();

    await expect(page).toHaveURL(/\/terminal/);
    await expect(xterm).toBeVisible();
    await expect(newTerminal).not.toBeVisible();
  });
});
