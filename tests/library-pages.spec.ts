import { test, expect } from '@playwright/test';

// ---------------------------------------------------------------------------
// Tool Library
// ---------------------------------------------------------------------------

test.describe('Tool Library page', () => {
  test('loads and shows heading', async ({ page }) => {
    await page.goto('/library/tools');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Tool Library' })).toBeVisible({ timeout: 5_000 });
  });

  test('displays catalog items', async ({ page }) => {
    await page.goto('/library/tools');
    await page.waitForLoadState('networkidle');

    // The page renders tool cards in a grid — verify at least one tool name is visible
    // "Weather" is the first tool in the hardcoded list and requires no API key
    await expect(page.getByText('Weather').first()).toBeVisible({ timeout: 5_000 });

    // Verify the subtitle shows a count of tools available
    await expect(page.getByText(/\d+ tools available/)).toBeVisible({ timeout: 5_000 });
  });

  test('category filter narrows results', async ({ page }) => {
    await page.goto('/library/tools');
    await page.waitForLoadState('networkidle');

    // Count cards in the grid before filtering
    const cards = page.locator('.grid > div');
    const allCount = await cards.count();
    expect(allCount).toBeGreaterThan(0);

    // Click the "AI" category filter button
    await page.getByRole('button', { name: 'AI', exact: true }).click();
    await page.waitForTimeout(300);

    // After filtering, fewer cards should be shown
    const filteredCount = await cards.count();
    expect(filteredCount).toBeLessThan(allCount);
    expect(filteredCount).toBeGreaterThan(0);

    // Click "All" to restore
    await page.getByRole('button', { name: 'All', exact: true }).click();
    await page.waitForTimeout(300);
    const restoredCount = await cards.count();
    expect(restoredCount).toBe(allCount);
  });

  test('has cross-links to other libraries', async ({ page }) => {
    await page.goto('/library/tools');
    await page.waitForLoadState('networkidle');

    // Footer has links to Agent Library and Skill Library
    await expect(page.getByRole('link', { name: /Agent Library/ })).toBeVisible({ timeout: 5_000 });
    await expect(page.getByRole('link', { name: /Skill Library/ })).toBeVisible({ timeout: 5_000 });
  });
});

// ---------------------------------------------------------------------------
// Agent Library
// ---------------------------------------------------------------------------

test.describe('Agent Library page', () => {
  test('loads and shows heading', async ({ page }) => {
    await page.goto('/library/agents');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Agent Library' })).toBeVisible({ timeout: 5_000 });
  });

  test('displays catalog items', async ({ page }) => {
    await page.goto('/library/agents');
    await page.waitForLoadState('networkidle');

    // "Whiskers" is the first agent in the hardcoded list
    await expect(page.getByText('Whiskers').first()).toBeVisible({ timeout: 5_000 });

    // Verify the subtitle shows a count of agents
    await expect(page.getByText(/\d+ agents ready/)).toBeVisible({ timeout: 5_000 });
  });

  test('displays model badges', async ({ page }) => {
    await page.goto('/library/agents');
    await page.waitForLoadState('networkidle');

    // Agents show either "Sonnet" or "Haiku" model badges
    await expect(page.getByText('Sonnet').first()).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText('Haiku').first()).toBeVisible({ timeout: 5_000 });
  });

  test('category filter narrows results', async ({ page }) => {
    await page.goto('/library/agents');
    await page.waitForLoadState('networkidle');

    const cards = page.locator('.grid > div');
    const allCount = await cards.count();
    expect(allCount).toBeGreaterThan(0);

    // Click the "Engineering" category filter
    await page.getByRole('button', { name: 'Engineering', exact: true }).click();
    await page.waitForTimeout(300);

    const filteredCount = await cards.count();
    expect(filteredCount).toBeLessThan(allCount);
    expect(filteredCount).toBeGreaterThan(0);

    // Restore
    await page.getByRole('button', { name: 'All', exact: true }).click();
    await page.waitForTimeout(300);
    const restoredCount = await cards.count();
    expect(restoredCount).toBe(allCount);
  });

  test('has cross-links to other libraries', async ({ page }) => {
    await page.goto('/library/agents');
    await page.waitForLoadState('networkidle');

    await expect(page.getByRole('link', { name: /Tool Library/ })).toBeVisible({ timeout: 5_000 });
    await expect(page.getByRole('link', { name: /Skill Library/ })).toBeVisible({ timeout: 5_000 });
  });
});

// ---------------------------------------------------------------------------
// Skill Library
// ---------------------------------------------------------------------------

test.describe('Skill Library page', () => {
  test('loads and shows heading', async ({ page }) => {
    await page.goto('/library/skills');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { name: 'Skill Library' })).toBeVisible({ timeout: 5_000 });
  });

  test('displays catalog items', async ({ page }) => {
    await page.goto('/library/skills');
    await page.waitForLoadState('networkidle');

    // "Skill Creator" is the first skill in the hardcoded list
    await expect(page.getByText('Skill Creator').first()).toBeVisible({ timeout: 5_000 });

    // Verify the subtitle shows a count of skills
    await expect(page.getByText(/\d+ skills to extend/)).toBeVisible({ timeout: 5_000 });
  });

  test('displays "Uses tools" badges', async ({ page }) => {
    await page.goto('/library/skills');
    await page.waitForLoadState('networkidle');

    // Some skills show a "Uses tools" badge (e.g., Image Processing, Video Processing)
    await expect(page.getByText('Uses tools').first()).toBeVisible({ timeout: 5_000 });
  });

  test('displays required tools info', async ({ page }) => {
    await page.goto('/library/skills');
    await page.waitForLoadState('networkidle');

    // Skills like Image Processing require "imagemagick"
    await expect(page.getByText('imagemagick').first()).toBeVisible({ timeout: 5_000 });
  });

  test('category filter narrows results', async ({ page }) => {
    await page.goto('/library/skills');
    await page.waitForLoadState('networkidle');

    const cards = page.locator('.grid > div');
    const allCount = await cards.count();
    expect(allCount).toBeGreaterThan(0);

    // Click the "Coding" category filter
    await page.getByRole('button', { name: 'Coding', exact: true }).click();
    await page.waitForTimeout(300);

    const filteredCount = await cards.count();
    expect(filteredCount).toBeLessThan(allCount);
    expect(filteredCount).toBeGreaterThan(0);

    // Restore
    await page.getByRole('button', { name: 'All', exact: true }).click();
    await page.waitForTimeout(300);
    const restoredCount = await cards.count();
    expect(restoredCount).toBe(allCount);
  });

  test('has cross-links to other libraries', async ({ page }) => {
    await page.goto('/library/skills');
    await page.waitForLoadState('networkidle');

    await expect(page.getByRole('link', { name: /Tool Library/ })).toBeVisible({ timeout: 5_000 });
    await expect(page.getByRole('link', { name: /Agent Library/ })).toBeVisible({ timeout: 5_000 });
  });
});

// ---------------------------------------------------------------------------
// Cross-navigation between libraries
// ---------------------------------------------------------------------------

test.describe('Library cross-navigation', () => {
  test('can navigate from Tool Library to Agent Library via link', async ({ page }) => {
    await page.goto('/library/tools');
    await page.waitForLoadState('networkidle');

    // Click the "Agent Library" cross-link at the bottom
    await page.getByRole('link', { name: /Agent Library/ }).click();
    await page.waitForLoadState('networkidle');

    // Should now be on Agent Library page
    await expect(page.getByRole('heading', { name: 'Agent Library' })).toBeVisible({ timeout: 5_000 });
  });

  test('can navigate from Agent Library to Skill Library via link', async ({ page }) => {
    await page.goto('/library/agents');
    await page.waitForLoadState('networkidle');

    await page.getByRole('link', { name: /Skill Library/ }).click();
    await page.waitForLoadState('networkidle');

    await expect(page.getByRole('heading', { name: 'Skill Library' })).toBeVisible({ timeout: 5_000 });
  });

  test('can navigate from Skill Library to Tool Library via link', async ({ page }) => {
    await page.goto('/library/skills');
    await page.waitForLoadState('networkidle');

    await page.getByRole('link', { name: /Tool Library/ }).click();
    await page.waitForLoadState('networkidle');

    await expect(page.getByRole('heading', { name: 'Tool Library' })).toBeVisible({ timeout: 5_000 });
  });
});

// ---------------------------------------------------------------------------
// Dashboards page (read-only — dashboards are created via Chat AI)
// ---------------------------------------------------------------------------

test.describe('Dashboards page', () => {
  test('loads and shows heading', async ({ page }) => {
    await page.goto('/dashboards');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Dashboards').first()).toBeVisible({ timeout: 5_000 });
  });

  test('shows empty state or dashboard list', async ({ page }) => {
    await page.goto('/dashboards');
    await page.waitForLoadState('networkidle');

    // Either the empty state message or a dashboard selector should be visible
    const emptyState = page.getByText('No dashboards yet');
    const dashboardSelector = page.getByLabel('Select dashboard');

    const hasEmpty = await emptyState.isVisible().catch(() => false);
    const hasSelector = await dashboardSelector.isVisible().catch(() => false);

    // One of these should be true
    expect(hasEmpty || hasSelector).toBeTruthy();
  });
});

// ---------------------------------------------------------------------------
// Console errors on library pages
// ---------------------------------------------------------------------------

test.describe('Library pages console errors', () => {
  test('no console errors on library page loads', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') errors.push(msg.text());
    });

    const routes = ['/library/tools', '/library/agents', '/library/skills'];
    for (const route of routes) {
      await page.goto(route);
      await page.waitForLoadState('networkidle');
    }

    // Filter out known acceptable errors
    const realErrors = errors.filter(e =>
      !e.includes('favicon') &&
      !e.includes('WebSocket') &&
      !e.includes('net::ERR')
    );
    expect(realErrors).toEqual([]);
  });
});
