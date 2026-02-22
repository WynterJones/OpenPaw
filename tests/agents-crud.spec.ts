import { test, expect, type Page } from '@playwright/test';

// These tests run in serial order so that each test can depend on state
// created by the previous one (create → navigate → edit → toggle → delete).
test.describe.serial('Agent CRUD', () => {
  const AGENT_NAME = 'E2E Test Agent';
  const AGENT_DESC = 'Created by Playwright tests';
  const AGENT_SLUG = 'e2e-test-agent';

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  /** Navigate to /agents and wait for the page to fully load. */
  async function gotoAgents(page: Page) {
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');
  }

  /**
   * Find the agent card for AGENT_NAME in whichever view is active and click it
   * to navigate to the edit page.
   */
  async function openAgentEditPage(page: Page) {
    await gotoAgents(page);
    // The card / row shows the agent name as text — click the first match that
    // is NOT the Pounce/Gateway card.
    const agentCard = page.locator(`text="${AGENT_NAME}"`).first();
    await expect(agentCard).toBeVisible({ timeout: 10_000 });
    await agentCard.click();
    await page.waitForLoadState('networkidle');
    await expect(page).toHaveURL(new RegExp(`/agents/${AGENT_SLUG}`), { timeout: 10_000 });
  }

  // ---------------------------------------------------------------------------
  // Test 1 — create
  // ---------------------------------------------------------------------------

  test('can create an agent', async ({ page }) => {
    await gotoAgents(page);

    // Open the Create Agent modal
    await page.getByRole('button', { name: /add agent/i }).click();

    // Modal should appear with title "Create Agent"
    const modal = page.getByRole('dialog', { name: /create agent/i });
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Fill in name and description
    await modal.getByLabel('Name').fill(AGENT_NAME);
    await modal.getByLabel('Description').fill(AGENT_DESC);

    // Click the "Create Agent" button
    await modal.getByRole('button', { name: /create agent/i }).click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 10_000 });

    // The new agent should appear in the list
    await expect(page.getByText(AGENT_NAME)).toBeVisible({ timeout: 10_000 });
  });

  // ---------------------------------------------------------------------------
  // Test 2 — button disabled when name empty
  // ---------------------------------------------------------------------------

  test('create button is disabled with empty name', async ({ page }) => {
    await gotoAgents(page);

    await page.getByRole('button', { name: /add agent/i }).click();

    const modal = page.getByRole('dialog', { name: /create agent/i });
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // "Create Agent" submit button — should be disabled with no name
    const createBtn = modal.getByRole('button', { name: /create agent/i });
    await expect(createBtn).toBeDisabled();

    // Fill in a name → button becomes enabled
    const nameInput = modal.getByLabel('Name');
    await nameInput.fill('Temporary Name');
    await expect(createBtn).toBeEnabled();

    // Clear the name → button is disabled again
    await nameInput.clear();
    await expect(createBtn).toBeDisabled();

    // Close the modal without creating
    await modal.getByRole('button', { name: /cancel/i }).click();
    await expect(modal).not.toBeVisible({ timeout: 5_000 });
  });

  // ---------------------------------------------------------------------------
  // Test 3 — navigate to edit page
  // ---------------------------------------------------------------------------

  test('can navigate to agent edit page', async ({ page }) => {
    await openAgentEditPage(page);

    // The edit page top-bar shows the agent name
    await expect(page.getByText(AGENT_NAME).first()).toBeVisible();

    // The "Name" input in the Details card should be pre-filled
    const nameInput = page.getByLabel('Name');
    await expect(nameInput).toHaveValue(AGENT_NAME);
  });

  // ---------------------------------------------------------------------------
  // Test 4 — edit agent details
  // ---------------------------------------------------------------------------

  test('can edit agent details', async ({ page }) => {
    await openAgentEditPage(page);

    const UPDATED_DESC = 'Updated by Playwright';

    // Save button should be disabled — no changes yet
    const saveBtn = page.getByRole('button', { name: /^save$/i });
    await expect(saveBtn).toBeDisabled();

    // Change the description
    const descInput = page.getByLabel('Description');
    await descInput.clear();
    await descInput.fill(UPDATED_DESC);

    // Save button should now be enabled
    await expect(saveBtn).toBeEnabled();

    // Click Save
    await saveBtn.click();

    // After save, the button goes back to disabled (no unsaved changes)
    await expect(saveBtn).toBeDisabled({ timeout: 10_000 });

    // Reload and verify the change persisted
    await page.reload();
    await page.waitForLoadState('networkidle');
    await expect(page.getByLabel('Description')).toHaveValue(UPDATED_DESC, { timeout: 5_000 });

    // Restore original description
    const descInputAfter = page.getByLabel('Description');
    await descInputAfter.clear();
    await descInputAfter.fill(AGENT_DESC);
    await page.getByRole('button', { name: /^save$/i }).click();
  });

  // ---------------------------------------------------------------------------
  // Test 5 — toggle enabled state from list page
  // ---------------------------------------------------------------------------

  test('can toggle agent enabled state', async ({ page }) => {
    await gotoAgents(page);

    // The Toggle component renders as role="switch" with aria-label="Enable agent"
    // Each agent card has one. We find the one nearest the E2E Test Agent text.
    // Strategy: find the card containing the agent name, then locate the switch inside it.
    const agentCard = page.locator('[class*="Card"], .overflow-hidden').filter({ hasText: AGENT_NAME }).first();
    await expect(agentCard).toBeVisible({ timeout: 10_000 });

    const toggle = agentCard.getByRole('switch', { name: /enable agent/i });
    await expect(toggle).toBeVisible();

    // Record initial checked state
    const initialChecked = await toggle.getAttribute('aria-checked');

    // Click the toggle (it has stopPropagation so it won't navigate)
    await toggle.click();

    // The aria-checked should have flipped
    const expectedChecked = initialChecked === 'true' ? 'false' : 'true';
    await expect(toggle).toHaveAttribute('aria-checked', expectedChecked, { timeout: 5_000 });

    // Toggle back so the agent is in whatever state it started
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-checked', initialChecked!, { timeout: 5_000 });
  });

  // ---------------------------------------------------------------------------
  // Test 6 — delete from edit page
  // ---------------------------------------------------------------------------

  test('can delete agent from edit page', async ({ page }) => {
    await openAgentEditPage(page);

    // The "Delete Agent" button is a plain <button> at the bottom of the left column.
    // It contains the text "Delete Agent" and a Trash2 icon.
    const deleteBtn = page.getByRole('button', { name: /delete agent/i });
    await expect(deleteBtn).toBeVisible({ timeout: 5_000 });
    await deleteBtn.click();

    // A modal with title "Delete Agent" should appear
    const deleteModal = page.getByRole('dialog', { name: /delete agent/i });
    await expect(deleteModal).toBeVisible({ timeout: 5_000 });

    // Confirm by clicking the danger "Delete" button inside the modal
    const confirmBtn = deleteModal.getByRole('button', { name: /^delete$/i });
    await expect(confirmBtn).toBeVisible();
    await confirmBtn.click();

    // Should redirect back to /agents
    await expect(page).toHaveURL(/\/agents$/, { timeout: 10_000 });

    // The deleted agent should no longer appear in the list
    await page.waitForLoadState('networkidle');
    await expect(page.getByText(AGENT_NAME)).not.toBeVisible({ timeout: 5_000 });
  });
});
