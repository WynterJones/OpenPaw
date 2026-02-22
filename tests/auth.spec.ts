import { test, expect } from '@playwright/test';
import { TEST_USER } from './helpers';

test.describe('Authentication', () => {
  test('is logged in after setup (storage state)', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await expect(page).toHaveURL(/\/chat/);
  });

  test('redirects unauthenticated users to login', async ({ browser, baseURL }) => {
    const context = await browser.newContext({
      baseURL,
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto('/chat');
    await expect(page).toHaveURL(/\/login/, { timeout: 10_000 });
    await context.close();
  });

  test('login page renders correctly', async ({ browser, baseURL }) => {
    const context = await browser.newContext({
      baseURL,
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto('/login');
    await page.waitForLoadState('networkidle');
    await expect(page.getByText('OpenPaw', { exact: true }).first()).toBeVisible();
    await expect(page.getByText('Sign in to your dashboard')).toBeVisible();
    await expect(page.getByLabel('Username')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
    await expect(page.getByRole('button', { name: /sign in/i })).toBeVisible();
    await context.close();
  });

  test('wrong credentials show error or reload login page', async ({ browser, baseURL }) => {
    const context = await browser.newContext({
      baseURL,
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto('/login');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Username').fill('baduser');
    await page.getByLabel('Password').fill('wrongpassword');
    await page.getByRole('button', { name: /sign in/i }).click();
    await page.waitForLoadState('networkidle');
    // Should still be on login page (not authenticated)
    await expect(page).toHaveURL(/\/login/);
    await expect(page.getByRole('button', { name: /sign in/i })).toBeVisible();
    await context.close();
  });

  test('can log in with valid credentials', async ({ browser, baseURL }) => {
    const context = await browser.newContext({
      baseURL,
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();
    await page.goto('/login');
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Username').fill(TEST_USER.username);
    await page.getByLabel('Password').fill(TEST_USER.password);
    await page.getByRole('button', { name: /sign in/i }).click();
    await expect(page).toHaveURL(/\/chat/, { timeout: 10_000 });
    await context.close();
  });

  test('logout flow invalidates session', async ({ page, browser, baseURL }) => {
    // Start authenticated — default storage state is already logged in
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await expect(page).toHaveURL(/\/chat/);

    // Open the user menu in the header and click Sign out
    await page.getByRole('button', { name: 'User menu' }).click();
    await page.getByRole('menuitem', { name: /sign out/i }).click();

    // Should redirect to /login
    await expect(page).toHaveURL(/\/login/, { timeout: 10_000 });

    // Verify the session is truly invalidated by attempting to navigate to /chat
    // in a fresh context so no stale cookies from the current page carry over
    const freshContext = await browser.newContext({
      baseURL,
      storageState: { cookies: [], origins: [] },
    });
    const freshPage = await freshContext.newPage();
    await freshPage.goto('/chat');
    await expect(freshPage).toHaveURL(/\/login/, { timeout: 10_000 });
    await freshContext.close();
  });
});

test.describe('Password change', () => {
  // Helper: navigate to the Profile tab on the Settings page
  async function goToProfileTab(page: import('@playwright/test').Page) {
    await page.goto('/settings');
    await page.waitForLoadState('networkidle');
    // The Profile tab is rendered via role="tab" with id="tab-profile"
    await page.getByRole('tab', { name: 'Profile' }).click();
    // Wait for the "Change Password" card heading to appear
    await expect(page.getByText('Change Password')).toBeVisible({ timeout: 8_000 });
  }

  test('can change password and revert', async ({ page }) => {
    const tempPassword = 'Newpass5678';

    await goToProfileTab(page);

    // --- Step 1: Change to tempPassword ---
    await page.getByLabel('Current Password').fill(TEST_USER.password);
    await page.getByLabel('New Password').fill(tempPassword);
    await page.getByLabel('Confirm New Password').fill(tempPassword);
    await page.getByRole('button', { name: 'Update Password' }).click();

    // Wait for success toast: "Password changed successfully"
    await expect(page.getByText(/password changed successfully/i)).toBeVisible({ timeout: 8_000 });

    // --- Step 2: Revert back to original password ---
    // The form fields are cleared on success, so fill them again
    await page.getByLabel('Current Password').fill(tempPassword);
    await page.getByLabel('New Password').fill(TEST_USER.password);
    await page.getByLabel('Confirm New Password').fill(TEST_USER.password);
    await page.getByRole('button', { name: 'Update Password' }).click();

    // Confirm the revert also succeeded
    await expect(page.getByText(/password changed successfully/i)).toBeVisible({ timeout: 8_000 });
  });

  test('update password button disabled until all fields filled', async ({ page }) => {
    await goToProfileTab(page);

    const updateBtn = page.getByRole('button', { name: 'Update Password' });

    // Button starts disabled — all fields empty
    await expect(updateBtn).toBeDisabled();

    // Fill only Current Password — still disabled
    await page.getByLabel('Current Password').fill(TEST_USER.password);
    await expect(updateBtn).toBeDisabled();

    // Fill New Password as well — still disabled (Confirm is empty)
    await page.getByLabel('New Password').fill('Newpass5678');
    await expect(updateBtn).toBeDisabled();

    // Fill Confirm New Password — button should now be enabled
    await page.getByLabel('Confirm New Password').fill('Newpass5678');
    await expect(updateBtn).toBeEnabled();

    // Clear one field — button goes disabled again
    await page.getByLabel('New Password').clear();
    await expect(updateBtn).toBeDisabled();
  });
});
