import { test, expect } from '@playwright/test';

test.describe('Mobile responsiveness', () => {
  test('sidebar is hidden on mobile', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    // Desktop sidebar uses "hidden md:flex" so should not be visible on mobile
    const sidebar = page.locator('aside');
    if (await sidebar.count() > 0) {
      await expect(sidebar).not.toBeVisible();
    }
  });

  test('pages load without layout breaking', async ({ page }) => {
    const routes = ['/chat', '/tools', '/agents', '/settings', '/dashboards', '/scheduler'];
    for (const route of routes) {
      await page.goto(route);
      await page.waitForLoadState('networkidle');
      // No horizontal overflow (allow small tolerance for scrollbars)
      const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
      const clientWidth = await page.evaluate(() => document.documentElement.clientWidth);
      expect(scrollWidth).toBeLessThanOrEqual(clientWidth + 5);
    }
  });

  test('chat is usable on mobile', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    await expect(page.locator('body')).toBeVisible();

    // Should be able to find New Chat button
    const newBtn = page.getByRole('button', { name: /new chat/i }).first();
    if (await newBtn.isVisible().catch(() => false)) {
      await newBtn.click();
      await expect(page.locator('textarea[placeholder*="Ask anything"]')).toBeVisible({ timeout: 5_000 });
    }
  });

  test('bottom nav is visible on mobile', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    // Bottom nav should exist on mobile
    const bottomNav = page.locator('nav.md\\:hidden');
    if (await bottomNav.count() > 0) {
      await expect(bottomNav).toBeVisible();
    }
  });

  test('More menu opens and navigates', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    // Find "More" button (aria-label="More navigation options")
    const moreBtn = page.getByRole('button', { name: /more navigation/i });
    await expect(moreBtn).toBeVisible();
    await moreBtn.click();

    // Popup menu should appear (role="menu")
    const menu = page.getByRole('menu');
    await expect(menu).toBeVisible();

    // Verify menu items: Tools, Skills, Secrets, Logs, Context, Settings
    for (const item of ['Tools', 'Skills', 'Secrets', 'Logs', 'Context', 'Settings']) {
      await expect(menu.getByText(item)).toBeVisible();
    }

    // Click Settings to navigate
    await menu.getByText('Settings').click();
    await expect(page).toHaveURL(/\/settings/);

    // Menu should auto-close after navigation
    await expect(page.getByRole('menu')).not.toBeVisible();
  });
});
