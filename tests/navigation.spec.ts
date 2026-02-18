import { test, expect } from '@playwright/test';

test.describe('Navigation', () => {
  test('sidebar shows all nav items', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    const sidebar = page.locator('aside');
    // Sidebar nav items in current UI
    for (const item of ['Dashboard', 'Chats', 'Agents', 'Browsers', 'Tools', 'Skills', 'Context', 'Scheduler', 'Heartbeat', 'Secrets', 'Logs', 'Settings']) {
      await expect(sidebar.getByText(item, { exact: true })).toBeVisible();
    }
  });

  test('navigates to each page without errors', async ({ page }) => {
    const routes = [
      '/chat', '/tools', '/agents', '/secrets', '/dashboards',
      '/scheduler', '/logs', '/settings', '/skills', '/context',
      '/browser', '/heartbeat',
    ];

    for (const route of routes) {
      await page.goto(route);
      await page.waitForLoadState('networkidle');
      await expect(page).toHaveURL(new RegExp(route));
      await expect(page.locator('body')).toBeVisible();
    }
  });

  test('sidebar navigation links work', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    const sidebar = page.locator('aside');

    // Click Tools in sidebar
    await sidebar.getByText('Tools', { exact: true }).click();
    await expect(page).toHaveURL(/\/tools/);

    // Click Agents
    await sidebar.getByText('Agents', { exact: true }).click();
    await expect(page).toHaveURL(/\/agents/);

    // Click Settings
    await sidebar.getByText('Settings', { exact: true }).click();
    await expect(page).toHaveURL(/\/settings/);

    // Click back to Chats
    await sidebar.getByText('Chats', { exact: true }).click();
    await expect(page).toHaveURL(/\/chat/);
  });

  test('redirects unknown routes to /chat', async ({ page }) => {
    await page.goto('/nonexistent-page-xyz');
    await expect(page).toHaveURL(/\/chat/);
  });
});
