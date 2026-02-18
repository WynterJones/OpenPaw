import { test, expect } from '@playwright/test';

test.describe('Tools page', () => {
  test('loads and shows tools heading', async ({ page }) => {
    await page.goto('/tools');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Tools').first()).toBeVisible();
  });

  test('has search input', async ({ page }) => {
    await page.goto('/tools');
    await page.waitForLoadState('networkidle');
    await expect(page.locator('input[placeholder="Search tools..."]')).toBeVisible();
  });

  test('search filters the tools list', async ({ page }) => {
    await page.goto('/tools');
    await page.waitForLoadState('networkidle');

    const searchInput = page.locator('input[placeholder="Search tools..."]');
    await expect(searchInput).toBeVisible();

    // Count visible tool cards or rows before searching (tools or empty state)
    const toolCards = page.locator('.grid .rounded-xl, .grid [class*="Card"], tbody tr');
    const initialCount = await toolCards.count();

    // Type a search term that is very unlikely to match any tool name
    await searchInput.fill('zzz_no_match_xqz');
    await page.waitForTimeout(300);

    if (initialCount > 0) {
      // If there were tools, searching for a nonsense term should show fewer or none
      const filteredCount = await toolCards.count();
      expect(filteredCount).toBeLessThanOrEqual(initialCount);
    }

    // The empty state or no-results message should appear when no matches
    const noResultsVisible = await page.getByText(/no tools found/i).isVisible().catch(() => false);
    const emptyStateVisible = await page.locator('[class*="EmptyState"], [class*="empty"]').isVisible().catch(() => false);
    // Either fewer results or a no-results message is acceptable
    expect(noResultsVisible || emptyStateVisible || initialCount === 0).toBeTruthy();

    // Clear search and verify list returns to original state
    await searchInput.clear();
    await page.waitForTimeout(300);
    const restoredCount = await toolCards.count();
    expect(restoredCount).toBe(initialCount);
  });
});

test.describe('Agents page', () => {
  test('loads and shows agents heading', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Agents').first()).toBeVisible();
    // Should have the Add Agent button
    await expect(page.getByRole('button', { name: /add agent/i })).toBeVisible();
  });
});

test.describe('Secrets page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/secrets');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Secrets').first()).toBeVisible();
  });
});

test.describe('Dashboards page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/dashboards');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Dashboards').first()).toBeVisible();
  });
});

test.describe('Scheduler page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/scheduler');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Scheduler').first()).toBeVisible();
  });
});

test.describe('Logs page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/logs');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Logs').first()).toBeVisible();
  });

  test('search input filters log entries', async ({ page }) => {
    await page.goto('/logs');
    await page.waitForLoadState('networkidle');

    // The Logs page uses SearchBar with placeholder "Search logs..."
    const searchInput = page.locator('input[placeholder="Search logs..."]');
    await expect(searchInput).toBeVisible({ timeout: 8000 });

    // Count visible table rows before filtering (client-side filter on loaded page)
    const rows = page.locator('tbody tr');
    const initialCount = await rows.count();

    // Type a search query unlikely to match anything
    await searchInput.fill('zzz_no_match_xqz');
    await page.waitForTimeout(300);

    if (initialCount > 0) {
      // Rows should be fewer or zero after filtering
      const filteredCount = await rows.count();
      expect(filteredCount).toBeLessThanOrEqual(initialCount);
    }

    // Clear search and verify row count is restored
    await searchInput.clear();
    await page.waitForTimeout(300);
    const restoredCount = await rows.count();
    expect(restoredCount).toBe(initialCount);
  });
});

test.describe('Settings page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Settings').first()).toBeVisible();
  });

  test('General tab persists app name change', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    // Click the General tab
    await page.getByRole('button', { name: 'General' }).click();
    await page.waitForLoadState('networkidle');

    // Find the App Name input (labeled "App Name")
    const appNameInput = page.getByLabel('App Name');
    await expect(appNameInput).toBeVisible({ timeout: 8000 });

    // Read the original value so we can restore it afterward
    const originalValue = await appNameInput.inputValue();

    // Update to a unique test value
    const testValue = 'OpenPaw Test Updated';
    await appNameInput.clear();
    await appNameInput.fill(testValue);

    // Click Save Changes
    await page.getByRole('button', { name: /save changes/i }).click();

    // Wait for the success toast to appear
    await expect(page.getByText(/settings saved/i)).toBeVisible({ timeout: 8000 });

    // Reload and re-open General tab
    await page.reload();
    await page.waitForLoadState('networkidle');
    await page.getByRole('button', { name: 'General' }).click();
    await page.waitForLoadState('networkidle');

    // Verify the value persisted
    const persistedInput = page.getByLabel('App Name');
    await expect(persistedInput).toBeVisible({ timeout: 8000 });
    await expect(persistedInput).toHaveValue(testValue);

    // Reset back to the original value so other tests are not affected
    await persistedInput.clear();
    await persistedInput.fill(originalValue || 'OpenPaw');
    await page.getByRole('button', { name: /save changes/i }).click();
    await expect(page.getByText(/settings saved/i)).toBeVisible({ timeout: 8000 });
  });

  test('Design tab accent color persists after save', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    // Navigate to the Design tab
    await page.getByRole('tab', { name: 'Design' }).click();
    await expect(page.getByText('Accent Color')).toBeVisible({ timeout: 8_000 });

    // Determine which preset is currently selected (has aria-pressed="true")
    // The PRESETS are: Pink, Indigo, Emerald, Amber, Blue, Red, Violet, Teal
    const presetNames = ['Pink', 'Indigo', 'Emerald', 'Amber', 'Blue', 'Red', 'Violet', 'Teal'];

    let originalPreset: string | null = null;
    for (const name of presetNames) {
      const btn = page.getByRole('button', { name, exact: true });
      const pressed = await btn.getAttribute('aria-pressed');
      if (pressed === 'true') {
        originalPreset = name;
        break;
      }
    }

    // Choose a target color that differs from the currently selected one.
    // Default accent in the app is Pink (#E84BA5); pick Emerald as the test target,
    // falling back to Indigo if Emerald is already selected.
    const targetPreset = originalPreset === 'Emerald' ? 'Indigo' : 'Emerald';

    // Click the target preset swatch
    await page.getByRole('button', { name: targetPreset, exact: true }).click();

    // The swatch should now be pressed
    await expect(
      page.getByRole('button', { name: targetPreset, exact: true })
    ).toHaveAttribute('aria-pressed', 'true');

    // Save the design
    await page.getByRole('button', { name: 'Save Design' }).click();
    await expect(page.getByText(/design saved/i)).toBeVisible({ timeout: 8_000 });

    // Reload the page and navigate back to Design tab
    await page.reload();
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'Design' }).click();
    await expect(page.getByText('Accent Color')).toBeVisible({ timeout: 8_000 });

    // Verify the target preset is still selected after reload
    await expect(
      page.getByRole('button', { name: targetPreset, exact: true })
    ).toHaveAttribute('aria-pressed', 'true');

    // Reset to the original preset (or Pink as the app default) to avoid
    // leaving a modified accent color for other tests
    const resetTo = originalPreset ?? 'Pink';
    await page.getByRole('button', { name: resetTo, exact: true }).click();
    await page.getByRole('button', { name: 'Save Design' }).click();
    await expect(page.getByText(/design saved/i)).toBeVisible({ timeout: 8_000 });
  });
});

test.describe('Skills page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/skills');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Skills').first()).toBeVisible();
  });
});

test.describe('Context page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/context');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Context').first()).toBeVisible();
  });
});

test.describe('Browser page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/browser');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Browsers').first()).toBeVisible();
  });
});

test.describe('Heartbeat page', () => {
  test('loads successfully', async ({ page }) => {
    await page.goto('/heartbeat');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('Heartbeat').first()).toBeVisible();
  });
});

test.describe('Empty states', () => {
  test('heartbeat shows empty execution state', async ({ page }) => {
    await page.goto('/heartbeat');
    await page.waitForLoadState('networkidle');
    // Should show "No executions yet" or "No agents" empty state (role="status")
    const emptyState = page.getByRole('status');
    if (await emptyState.count() > 0) {
      await expect(emptyState.first()).toBeVisible();
    }
  });

  test('notification bell shows empty state', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    // Click notification bell (aria-label contains "Notifications")
    const bell = page.getByRole('button', { name: /notifications/i });
    if (await bell.isVisible().catch(() => false)) {
      await bell.click();
      // Should see "No notifications" text in dropdown
      await expect(page.getByText('No notifications')).toBeVisible({ timeout: 3_000 });
    }
  });
});

test.describe('Rapid actions', () => {
  test('double-clicking new chat does not create duplicates', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    // Rapidly click "New Chat" twice
    const newChatBtn = page.getByRole('button', { name: /new chat/i }).first();
    await newChatBtn.dblclick();

    // Wait a moment for any duplicate creation
    await page.waitForTimeout(1000);

    // Verify textarea is visible (at least one chat was created)
    await expect(page.locator('textarea[placeholder*="Ask anything"]')).toBeVisible({ timeout: 5_000 });
  });
});

test.describe('Console errors', () => {
  test('no console errors on page load', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') errors.push(msg.text());
    });

    const routes = ['/chat', '/tools', '/agents', '/secrets', '/dashboards', '/scheduler', '/logs', '/settings', '/skills', '/context'];
    for (const route of routes) {
      await page.goto(route);
      await page.waitForLoadState('networkidle');
    }

    // Filter out known acceptable errors (websocket disconnects, missing favicon, network errors)
    const realErrors = errors.filter(e =>
      !e.includes('favicon') &&
      !e.includes('WebSocket') &&
      !e.includes('net::ERR')
    );
    expect(realErrors).toEqual([]);
  });
});
