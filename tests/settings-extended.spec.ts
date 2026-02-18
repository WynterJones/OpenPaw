import { test, expect, type Page } from '@playwright/test';
import { TEST_USER } from './helpers';

// ---------------------------------------------------------------------------
// Settings — Design tab
// ---------------------------------------------------------------------------

test.describe('Settings — Design tab', () => {
  async function goToDesignTab(page: Page) {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'Design' }).click();
    await expect(page.getByText('Accent Color')).toBeVisible({ timeout: 8_000 });
  }

  test('loads accent color picker with preset swatches', async ({ page }) => {
    await goToDesignTab(page);

    // Verify the color picker section heading
    await expect(page.getByText('Accent Color')).toBeVisible();

    // Verify at least some preset swatch buttons are present
    const presetNames = ['Pink', 'Indigo', 'Emerald', 'Amber', 'Blue', 'Red', 'Violet', 'Teal'];
    for (const name of presetNames) {
      await expect(page.getByRole('button', { name, exact: true })).toBeVisible();
    }

    // Verify the custom color input exists
    await expect(page.locator('input[type="color"]')).toBeVisible();
  });

  test('background image section exists with presets', async ({ page }) => {
    await goToDesignTab(page);

    // The "Background Image" card heading should be visible
    await expect(page.getByText('Background Image')).toBeVisible();

    // The "None" button for no background
    await expect(page.getByRole('button', { name: 'None' })).toBeVisible();

    // Preset background options should be present (aria-label based buttons)
    const bgPresets = ['Cat Robot', 'Tech Garden', 'Happy Shoggoth', 'Scary Shoggoth'];
    for (const preset of bgPresets) {
      await expect(page.getByRole('button', { name: preset })).toBeVisible();
    }
  });

  test('font picker section exists', async ({ page }) => {
    await goToDesignTab(page);

    // The "Font" heading should be visible
    await expect(page.getByText('Font').first()).toBeVisible();

    // Font options should be present
    const fontNames = ['Vend Sans', 'Playfair', 'Roboto', 'Fira Code', 'Inter'];
    for (const font of fontNames) {
      await expect(page.getByRole('button', { name: font, exact: true })).toBeVisible();
    }
  });

  test('Save Design and Reset buttons exist', async ({ page }) => {
    await goToDesignTab(page);

    await expect(page.getByRole('button', { name: 'Save Design' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Reset' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Design System' })).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Settings — AI Models tab
// ---------------------------------------------------------------------------

test.describe('Settings — AI Models tab', () => {
  async function goToModelsTab(page: Page) {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    await page.getByRole('tab', { name: 'AI Models' }).click();
    // Wait for the API key section to load
    await expect(page.getByText('OpenRouter API Key')).toBeVisible({ timeout: 8_000 });
  }

  test('loads API key status section', async ({ page }) => {
    await goToModelsTab(page);

    // The API key section heading
    await expect(page.getByText('OpenRouter API Key')).toBeVisible();

    // Should show configured or not-configured status
    const configured = page.getByText('Configured');
    const notConfigured = page.getByText('Not configured');
    const isConfigured = await configured.isVisible().catch(() => false);
    const isNotConfigured = await notConfigured.isVisible().catch(() => false);
    expect(isConfigured || isNotConfigured).toBeTruthy();
  });

  test('gateway and builder model pickers exist', async ({ page }) => {
    await goToModelsTab(page);

    // Gateway Model section
    await expect(page.getByText('Gateway Model')).toBeVisible();
    await expect(page.getByText('Used to analyze user messages')).toBeVisible();

    // Builder Model section
    await expect(page.getByText('Builder Model')).toBeVisible();
    await expect(page.getByText('Used when building tools')).toBeVisible();
  });

  test('agent max turns and timeout settings exist', async ({ page }) => {
    await goToModelsTab(page);

    // Max Turns section
    await expect(page.getByText('Agent Max Turns')).toBeVisible();

    // Agent Timeout section
    await expect(page.getByText('Agent Timeout')).toBeVisible();

    // Save button
    await expect(page.getByRole('button', { name: 'Save Model Settings' })).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Settings — About tab
// ---------------------------------------------------------------------------

test.describe('Settings — About tab', () => {
  test('shows version and branding info', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    // Click the About tab
    await page.getByRole('tab', { name: 'About' }).click();

    // Should show the OpenPaw branding
    await expect(page.getByText('OpenPaw').first()).toBeVisible({ timeout: 8_000 });

    // Should show version number
    await expect(page.getByText('v0.0.1')).toBeVisible();

    // Should show the privacy statement
    await expect(page.getByText('Your data stays on your machine')).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Settings — Security tab
// ---------------------------------------------------------------------------

test.describe('Settings — Security tab', () => {
  test('loads session timeout and IP allowlist controls', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    await page.getByRole('tab', { name: 'Security' }).click();
    await expect(page.getByText('Security')).toBeVisible({ timeout: 8_000 });

    // Session Timeout input
    await expect(page.getByLabel('Session Timeout (hours)')).toBeVisible();

    // IP Allowlist toggle
    await expect(page.getByText('IP Allowlist')).toBeVisible();
    await expect(page.getByRole('switch', { name: /IP allowlist/i })).toBeVisible();

    // Save button
    await expect(page.getByRole('button', { name: 'Save Changes' })).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Settings — System tab
// ---------------------------------------------------------------------------

test.describe('Settings — System tab', () => {
  test('shows system information grid', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    await page.getByRole('tab', { name: 'System' }).click();
    await expect(page.getByText('System Information')).toBeVisible({ timeout: 8_000 });

    // Should show key system info labels
    await expect(page.getByText('Version')).toBeVisible();
    await expect(page.getByText('Go Version')).toBeVisible();
    await expect(page.getByText('Platform')).toBeVisible();
    await expect(page.getByText('Uptime')).toBeVisible();
    await expect(page.getByText('Database Size')).toBeVisible();
  });

  test('data management buttons exist', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    await page.getByRole('tab', { name: 'System' }).click();
    await expect(page.getByText('Data Management')).toBeVisible({ timeout: 8_000 });

    await expect(page.getByRole('button', { name: 'Export Data' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Import Data' })).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Settings — Notifications tab
// ---------------------------------------------------------------------------

test.describe('Settings — Notifications tab', () => {
  test('loads notification sound toggle', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    await page.getByRole('tab', { name: 'Notifications' }).click();
    await expect(page.getByText('Sound')).toBeVisible({ timeout: 8_000 });

    // Notification Sound toggle
    await expect(page.getByText('Notification Sound')).toBeVisible();
    await expect(page.getByRole('switch', { name: /notification sound/i })).toBeVisible();

    // Preview sound link
    await expect(page.getByText('Preview sound')).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Settings — Network tab
// ---------------------------------------------------------------------------

test.describe('Settings — Network tab', () => {
  test('loads local network and tailscale sections', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    await page.getByRole('tab', { name: 'Network' }).click();
    await expect(page.getByText('Local Network')).toBeVisible({ timeout: 8_000 });

    // Tailscale section
    await expect(page.getByText('Tailscale')).toBeVisible();
    await expect(page.getByRole('switch', { name: /tailscale remote access/i })).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Settings — Danger tab
// ---------------------------------------------------------------------------

test.describe('Settings — Danger tab', () => {
  test('shows warning and danger actions', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    await page.getByRole('tab', { name: 'Danger' }).click();

    // Warning banner
    await expect(page.getByText('Actions on this page are permanent')).toBeVisible({ timeout: 8_000 });

    // Delete All Data section
    await expect(page.getByText('Delete All Data')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Delete Data' })).toBeVisible();

    // Delete Account section
    await expect(page.getByText('Delete Account')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Delete Account' })).toBeVisible();
  });

  test('delete data modal requires typing DELETE', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    await page.getByRole('tab', { name: 'Danger' }).click();
    await expect(page.getByText('Delete All Data')).toBeVisible({ timeout: 8_000 });

    // Open the delete data modal
    await page.getByRole('button', { name: 'Delete Data' }).click();

    const modal = page.getByRole('dialog', { name: /delete all data/i });
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // The "Delete All Data" button inside the modal should be disabled
    const confirmBtn = modal.getByRole('button', { name: /delete all data/i });
    await expect(confirmBtn).toBeDisabled();

    // Type something wrong — button stays disabled
    await modal.getByPlaceholder('DELETE').fill('WRONG');
    await expect(confirmBtn).toBeDisabled();

    // Type DELETE — button becomes enabled
    await modal.getByPlaceholder('DELETE').clear();
    await modal.getByPlaceholder('DELETE').fill('DELETE');
    await expect(confirmBtn).toBeEnabled();

    // Close modal without deleting
    await modal.getByRole('button', { name: 'Cancel' }).click();
    await expect(modal).not.toBeVisible({ timeout: 5_000 });
  });
});

// ---------------------------------------------------------------------------
// Settings — Profile tab
// ---------------------------------------------------------------------------

test.describe('Settings — Profile tab', () => {
  test('shows account info and username field', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    // Profile is the default tab
    await expect(page.getByText('Account')).toBeVisible({ timeout: 8_000 });

    // Username input
    await expect(page.getByLabel('Username')).toBeVisible();

    // Update Username button
    await expect(page.getByRole('button', { name: 'Update Username' })).toBeVisible();
  });

  test('Update Username button disabled when username unchanged', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    // Profile is the default tab
    await expect(page.getByText('Account')).toBeVisible({ timeout: 8_000 });

    // The button should be disabled since username is not changed
    const updateBtn = page.getByRole('button', { name: 'Update Username' });
    await expect(updateBtn).toBeDisabled();

    // Change the username — button should become enabled
    const usernameInput = page.getByLabel('Username');
    const original = await usernameInput.inputValue();
    await usernameInput.clear();
    await usernameInput.fill(original + '_test');
    await expect(updateBtn).toBeEnabled();

    // Restore to original — button should be disabled again
    await usernameInput.clear();
    await usernameInput.fill(original);
    await expect(updateBtn).toBeDisabled();
  });

  test('password change section shows correct fields', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    // Profile is the default tab — scroll to Change Password
    await expect(page.getByText('Change Password')).toBeVisible({ timeout: 8_000 });

    // All three password fields should be present
    await expect(page.getByLabel('Current Password')).toBeVisible();
    await expect(page.getByLabel('New Password')).toBeVisible();
    await expect(page.getByLabel('Confirm New Password')).toBeVisible();

    // Update Password button should be disabled when fields are empty
    await expect(page.getByRole('button', { name: 'Update Password' })).toBeDisabled();
  });

  test('password mismatch shows error toast', async ({ page }) => {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    await expect(page.getByText('Change Password')).toBeVisible({ timeout: 8_000 });

    // Fill in mismatched passwords
    await page.getByLabel('Current Password').fill(TEST_USER.password);
    await page.getByLabel('New Password').fill('Mismatch1234');
    await page.getByLabel('Confirm New Password').fill('Different5678');

    // Click Update Password
    await page.getByRole('button', { name: 'Update Password' }).click();

    // Should show error toast about mismatch
    await expect(page.getByText(/passwords do not match/i)).toBeVisible({ timeout: 8_000 });
  });
});

// ---------------------------------------------------------------------------
// Settings — all tabs navigate correctly
// ---------------------------------------------------------------------------

test.describe('Settings — tab navigation', () => {
  test('all tabs can be activated without errors', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') errors.push(msg.text());
    });

    await page.goto('/settings');
    await page.waitForLoadState('networkidle');

    const tabNames = [
      'Profile', 'General', 'Notifications', 'Network',
      'AI Models', 'Design', 'Security', 'System', 'About', 'Danger',
    ];

    for (const tabName of tabNames) {
      await page.getByRole('tab', { name: tabName }).click();
      await page.waitForTimeout(500);
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

// ---------------------------------------------------------------------------
// Agent edit page — system prompt and tools tab
// ---------------------------------------------------------------------------

test.describe('Agent edit — gateway agent', () => {
  test('can view gateway agent edit page', async ({ page }) => {
    await page.goto('/agents/gateway');
    await page.waitForLoadState('networkidle');

    // Should show the agent name in the top bar
    await expect(page.getByText('Pounce').first()).toBeVisible({ timeout: 8_000 });

    // Should show the "Edit Agent" subtitle
    await expect(page.getByText('Edit Agent')).toBeVisible();

    // Details section should have the Name input
    await expect(page.getByLabel('Name')).toBeVisible();
    await expect(page.getByLabel('Description')).toBeVisible();
  });

  test('gateway agent has system prompt or identity tabs', async ({ page }) => {
    await page.goto('/agents/gateway');
    await page.waitForLoadState('networkidle');

    // The gateway agent may have identity initialized (file tabs) or legacy (system prompt textarea).
    // Check for either case.
    const hasSystemPrompt = await page.getByText('System Prompt').isVisible().catch(() => false);
    const hasSoulTab = await page.getByRole('tab', { name: 'Soul' }).isVisible().catch(() => false);

    // One of these should be true
    expect(hasSystemPrompt || hasSoulTab).toBeTruthy();
  });

  test('gateway agent has tools or skills tab if initialized', async ({ page }) => {
    await page.goto('/agents/gateway');
    await page.waitForLoadState('networkidle');

    // If the agent is initialized, it should have a Tools tab
    const hasToolsTab = await page.getByRole('tab', { name: 'Tools' }).isVisible().catch(() => false);
    const hasSkillsTab = await page.getByRole('tab', { name: 'Skills' }).isVisible().catch(() => false);

    // If initialized, both should exist. If not initialized, the Initialize button should be there.
    const hasInitialize = await page.getByRole('button', { name: 'Initialize' }).isVisible().catch(() => false);

    // Either the tabs are shown (initialized) or the Initialize button is shown (legacy)
    expect(hasToolsTab || hasInitialize).toBeTruthy();

    // If tools tab exists, click it and verify it shows content
    if (hasToolsTab) {
      await page.getByRole('tab', { name: 'Tools' }).click();
      await page.waitForTimeout(500);

      // Should show "Agent Tools" heading or "No tools assigned" empty state
      const hasToolsHeading = await page.getByText('Agent Tools').isVisible().catch(() => false);
      const hasNoTools = await page.getByText('No tools assigned').isVisible().catch(() => false);
      expect(hasToolsHeading || hasNoTools).toBeTruthy();
    }
  });
});
