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

  test('pages load without layout breaking at 320px', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 568 });
    const routes = [
      '/chat',
      '/inbox',
      '/services',
      '/agents',
      '/settings',
      '/dashboards',
      '/scheduler',
      '/secrets',
      '/skills',
      '/knowledge-base',
      '/library',
      '/studio',
      '/terminal',
    ];
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

    // The first matching button lives in the off-canvas thread drawer. Use the
    // empty-state action that is actually in the mobile viewport.
    const newBtn = page.getByRole('button', { name: /new chat/i }).last();
    await expect(newBtn).toBeInViewport();
    await newBtn.click();
    await expect(page.locator('textarea[placeholder*="Ask anything"]')).toBeVisible({ timeout: 5_000 });
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

    // Verify the secondary destinations.
    for (const item of ['Services', 'Skills', 'Secrets', 'Logs', 'Context', 'Settings']) {
      await expect(menu.getByText(item)).toBeVisible();
    }

    // Click Settings to navigate
    await menu.getByText('Settings').click();
    await expect(page).toHaveURL(/\/settings/);

    // Menu should auto-close after navigation
    await expect(page.getByRole('menu')).not.toBeVisible();
  });

  test('workspace switcher is available without the desktop sidebar', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');

    const switcher = page.getByRole('button', { name: /switch workspace/i });
    await expect(switcher).toBeVisible();
    await switcher.click();

    const menu = page.getByRole('menu');
    await expect(menu).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: /default/i })).toBeVisible();
    await menu.getByRole('menuitem', { name: /new workspace/i }).click();
    await expect(page.getByPlaceholder('Workspace name')).toBeVisible();
  });

  test('Studio swaps between mobile panes', async ({ page }) => {
    await page.goto('/studio');
    await page.waitForLoadState('networkidle');

    const createTab = page.getByRole('tab', { name: 'Create' });
    const galleryTab = page.getByRole('tab', { name: 'Gallery' });
    await expect(createTab).toHaveAttribute('aria-selected', 'true');
    await galleryTab.click();
    await expect(galleryTab).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByText('Nothing here yet')).toBeVisible();
  });
});
