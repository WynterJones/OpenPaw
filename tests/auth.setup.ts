import { test as setup, expect } from '@playwright/test';
import { runSetupWizard, loginUser, TEST_USER, waitForApiReady } from './helpers';

setup('authenticate for tests', async ({ page, baseURL }) => {
  await waitForApiReady(baseURL!);

  // Check if setup is needed or if we can just log in
  const statusRes = await page.request.get('/api/v1/setup/status');
  const status = await statusRes.json();

  if (status.needs_setup) {
    // Fresh database — run full setup wizard
    await runSetupWizard(page);
  } else {
    // Already set up — just log in
    await loginUser(page);
  }

  // Verify we're logged in and on /chat
  await expect(page).toHaveURL(/\/chat/);

  // Save auth state for other tests
  await page.context().storageState({ path: 'tests/.auth/user.json' });
});
