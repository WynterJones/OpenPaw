import { test, expect } from '@playwright/test';

// ---------------------------------------------------------------------------
// Secrets CRUD
// ---------------------------------------------------------------------------

test.describe('Secrets CRUD', () => {
  test('can create a secret', async ({ page }) => {
    await page.goto('/secrets');
    await page.waitForLoadState('networkidle');

    // Page heading visible
    await expect(page.getByText('Secrets').first()).toBeVisible();

    // Open the Add Secret modal
    await page.getByRole('button', { name: /add secret/i }).click();

    // Modal should appear with the correct title
    const modal = page.getByRole('dialog', { name: /add secret/i });
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Fill in Name and Value
    await modal.getByLabel('Name').fill('E2E_TEST_SECRET');
    await modal.getByLabel('Value').fill('test-value-123');

    // Submit — the button inside the modal is also "Add Secret"
    await modal.getByRole('button', { name: /add secret/i }).click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 10_000 });

    // The new secret should now appear in the list
    await expect(page.getByText('E2E_TEST_SECRET')).toBeVisible({ timeout: 10_000 });
  });

  test('can delete a secret', async ({ page }) => {
    await page.goto('/secrets');
    await page.waitForLoadState('networkidle');

    // Find the row containing our test secret and click its delete button
    const row = page.locator('tr, [data-row]').filter({ hasText: 'E2E_TEST_SECRET' });
    // The delete button in the DataTable row has title="Delete secret"
    await row.locator('button[title="Delete secret"]').click();

    // The delete confirmation modal should appear
    const deleteModal = page.getByRole('dialog', { name: /delete secret/i });
    await expect(deleteModal).toBeVisible({ timeout: 5_000 });

    // Click the danger Delete button inside the modal
    await deleteModal.getByRole('button', { name: /^delete$/i }).click();

    // Modal should close
    await expect(deleteModal).not.toBeVisible({ timeout: 10_000 });

    // Secret should no longer be visible
    await expect(page.getByText('E2E_TEST_SECRET')).not.toBeVisible({ timeout: 10_000 });
  });
});

// ---------------------------------------------------------------------------
// Skills CRUD
// ---------------------------------------------------------------------------

test.describe('Skills CRUD', () => {
  test('can create a skill', async ({ page }) => {
    await page.goto('/skills');
    await page.waitForLoadState('networkidle');

    // Page heading visible
    await expect(page.getByText('Skills').first()).toBeVisible();

    // Open the Create Skill modal via the "Add Skill" button
    await page.getByRole('button', { name: /add skill/i }).click();

    // Modal title is "Create Skill"
    const modal = page.getByRole('dialog', { name: /create skill/i });
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Fill in the skill name
    await modal.getByLabel('Name').fill('e2e-test-skill');

    // The content textarea is pre-filled with a YAML template — leave as-is or
    // replace with known content so the edit test can verify it changed.
    const contentArea = modal.locator('textarea');
    await expect(contentArea).toBeVisible();
    await contentArea.clear();
    await contentArea.fill('---\nname: e2e-test-skill\ndescription: E2E test skill\n---\n\n# E2E Test Skill\n\nCreated by Playwright.\n');

    // Click the Create button (has Plus icon)
    await modal.getByRole('button', { name: /^create$/i }).click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 10_000 });

    // Skill should appear in the list
    await expect(page.getByText('e2e-test-skill')).toBeVisible({ timeout: 10_000 });
  });

  test('can edit a skill', async ({ page }) => {
    await page.goto('/skills');
    await page.waitForLoadState('networkidle');

    // Click on the skill row to enter inline edit mode (full-page textarea, NOT a modal)
    await page.getByText('e2e-test-skill').first().click();

    // The inline edit view should appear: a Save button and a textarea are shown
    const saveButton = page.getByRole('button', { name: /save/i });
    await expect(saveButton).toBeVisible({ timeout: 5_000 });

    // There is only one full-page textarea in edit mode (min-h-[500px])
    const editTextarea = page.locator('textarea.min-h-\\[500px\\]');
    await expect(editTextarea).toBeVisible({ timeout: 5_000 });

    // Update the content
    await editTextarea.clear();
    await editTextarea.fill('---\nname: e2e-test-skill\ndescription: Updated by Playwright\n---\n\n# E2E Test Skill\n\nUpdated content.\n');

    // Save the edit
    await saveButton.click();

    // We should return to the list view (Save button gone, skill name visible again)
    await expect(saveButton).not.toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('e2e-test-skill')).toBeVisible({ timeout: 10_000 });

    // Navigate away and come back to verify persistence
    await page.goto('/chat');
    await page.goto('/skills');
    await page.waitForLoadState('networkidle');

    // Click on the skill again to re-enter edit mode
    await page.getByText('e2e-test-skill').first().click();
    const verifyTextarea = page.locator('textarea.min-h-\\[500px\\]');
    await expect(verifyTextarea).toBeVisible({ timeout: 5_000 });

    // The updated description should be present in the content
    await expect(verifyTextarea).toContainText('Updated by Playwright');
  });

  test('can delete a skill', async ({ page }) => {
    await page.goto('/skills');
    await page.waitForLoadState('networkidle');

    // Set up native confirm() handler BEFORE the click that triggers it
    page.on('dialog', dialog => dialog.accept());

    // Click the delete button for our test skill (title="Delete skill")
    // Use a Card row that contains the skill name and find its delete button
    const skillCard = page.locator('div').filter({ hasText: /^e2e-test-skill/ }).first();
    // The delete button with title="Delete skill" is the Trash2 icon button
    const deleteBtn = page.locator('button[title="Delete skill"]').first();
    await expect(deleteBtn).toBeVisible({ timeout: 5_000 });
    await deleteBtn.click();

    // After the native confirm is accepted, the skill should disappear from the list
    await expect(skillCard).not.toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('e2e-test-skill')).not.toBeVisible({ timeout: 10_000 });
  });
});

// ---------------------------------------------------------------------------
// Schedules CRUD
// ---------------------------------------------------------------------------

test.describe('Schedules CRUD', () => {
  test('can create a schedule', async ({ page }) => {
    await page.goto('/scheduler');
    await page.waitForLoadState('networkidle');

    // Page heading visible
    await expect(page.getByText('Scheduler').first()).toBeVisible();

    // Open the Create Schedule modal
    await page.getByRole('button', { name: /create schedule/i }).click();

    // The modal is size="lg" with title "Create Schedule"
    const modal = page.getByRole('dialog', { name: /create schedule/i });
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Fill in the schedule name
    await modal.getByLabel('Name').fill('E2E Test Schedule');

    // Select type: "AI Prompt" (the Bot icon button)
    await modal.getByRole('button', { name: /ai prompt/i }).click();

    // Select a schedule preset — "Every hour" (value: '0 * * * *')
    const presetSelect = modal.getByLabel('Schedule Preset');
    await expect(presetSelect).toBeVisible({ timeout: 3_000 });
    await presetSelect.selectOption({ label: 'Every hour' });

    // Select an agent from the Agent dropdown (pick the first non-placeholder option)
    const agentSelect = modal.getByLabel('Agent');
    await expect(agentSelect).toBeVisible({ timeout: 3_000 });
    // Get all options and pick the first real agent (not the placeholder "Select an agent...")
    const agentOptions = await agentSelect.locator('option').all();
    if (agentOptions.length > 1) {
      const firstAgentValue = await agentOptions[1].getAttribute('value');
      if (firstAgentValue) {
        await agentSelect.selectOption(firstAgentValue);
      }
    }

    // Fill in the prompt textarea
    const promptTextarea = modal.locator('textarea[placeholder*="Enter the prompt"]');
    await expect(promptTextarea).toBeVisible({ timeout: 3_000 });
    await promptTextarea.fill('E2E test prompt: summarize recent activity.');

    // Click "Create Schedule"
    await modal.getByRole('button', { name: /create schedule/i }).click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 10_000 });

    // The new schedule should appear in the list
    await expect(page.getByText('E2E Test Schedule')).toBeVisible({ timeout: 10_000 });
  });

  test('can toggle a schedule', async ({ page }) => {
    await page.goto('/scheduler');
    await page.waitForLoadState('networkidle');

    // Find the row containing our test schedule
    const row = page.locator('tr, div[role="row"]').filter({ hasText: 'E2E Test Schedule' });

    // The Toggle component renders as a button with role="switch" and aria-label="Enable schedule"
    const toggle = row.getByRole('switch', { name: /enable schedule/i });
    await expect(toggle).toBeVisible({ timeout: 5_000 });

    // Record current state
    const initiallyChecked = await toggle.getAttribute('aria-checked');

    // Click the toggle
    await toggle.click();

    // The aria-checked value should change
    await expect(toggle).not.toHaveAttribute('aria-checked', initiallyChecked ?? 'true', { timeout: 5_000 });
  });

  test('can delete a schedule', async ({ page }) => {
    await page.goto('/scheduler');
    await page.waitForLoadState('networkidle');

    // Schedules delete immediately — no confirm dialog
    // Find the delete button (title="Delete") in the row for our test schedule.
    // In list view the DataTable renders rows; the delete button is hidden on small
    // screens (hidden sm:block) so we target by title.
    const row = page.locator('tr, div[role="row"]').filter({ hasText: 'E2E Test Schedule' });
    await expect(row).toBeVisible({ timeout: 5_000 });

    const deleteBtn = row.locator('button[title="Delete"]');
    await expect(deleteBtn).toBeVisible({ timeout: 5_000 });
    await deleteBtn.click();

    // No modal — the row should simply disappear
    await expect(page.getByText('E2E Test Schedule')).not.toBeVisible({ timeout: 10_000 });
  });
});
