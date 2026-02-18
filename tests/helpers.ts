import { type Page, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

export const TEST_USER = {
  username: 'testadmin',
  password: 'Testpass1234',
  appName: 'OpenPaw Test',
};

export function resetDatabase() {
  const dbPath = path.resolve('data/openpaw.db');
  for (const ext of ['', '-wal', '-shm']) {
    try { fs.unlinkSync(dbPath + ext); } catch { /* ignore */ }
  }
}

export async function runSetupWizard(page: Page) {
  await page.goto('/setup');

  // Wait for the setup page to render (step 0: Welcome)
  await expect(page.getByText('Welcome to OpenPaw')).toBeVisible({ timeout: 15_000 });

  // Step 0: Welcome — click Continue
  await page.getByRole('button', { name: /continue/i }).click();

  // Step 1: Create Admin Account
  await expect(page.getByLabel('Username')).toBeVisible({ timeout: 5_000 });
  await page.getByLabel('Username').fill(TEST_USER.username);
  await page.getByLabel('Password', { exact: true }).fill(TEST_USER.password);
  await page.getByLabel('Confirm Password').fill(TEST_USER.password);
  await page.getByRole('button', { name: /continue/i }).click();

  // Step 2: API Key — just continue with defaults
  await expect(page.getByText(/step 3/i)).toBeVisible({ timeout: 5_000 });
  await page.getByRole('button', { name: /continue/i }).click();

  // Step 3: Configure Server — update app name, complete setup
  await expect(page.getByLabel('App Name')).toBeVisible({ timeout: 5_000 });
  await page.getByLabel('App Name').clear();
  await page.getByLabel('App Name').fill(TEST_USER.appName);
  await page.getByRole('button', { name: /complete setup/i }).click();

  // Should redirect to /chat
  await expect(page).toHaveURL(/\/chat/, { timeout: 15_000 });
}

export async function loginUser(page: Page) {
  await page.goto('/login');
  await expect(page.getByLabel('Username')).toBeVisible({ timeout: 10_000 });
  await page.getByLabel('Username').fill(TEST_USER.username);
  await page.getByLabel('Password').fill(TEST_USER.password);
  await page.getByRole('button', { name: /sign in/i }).click();
  await expect(page).toHaveURL(/\/chat/, { timeout: 10_000 });
}

export async function waitForApiReady(baseURL: string, maxWait = 15_000) {
  const start = Date.now();
  while (Date.now() - start < maxWait) {
    try {
      const res = await fetch(`${baseURL}/api/v1/setup/status`);
      if (res.status < 500) return;
    } catch { /* server not up yet */ }
    await new Promise(r => setTimeout(r, 500));
  }
  throw new Error(`Server at ${baseURL} did not become ready within ${maxWait}ms`);
}

export function getAuthToken(): string {
  const stateFile = path.resolve('tests/.auth/user.json');
  try {
    const state = JSON.parse(fs.readFileSync(stateFile, 'utf8'));

    // Try localStorage first (some auth flows store token there)
    const origin = state.origins?.[0];
    const tokenEntry = origin?.localStorage?.find((e: { name: string }) => e.name === 'openpaw_token');
    if (tokenEntry?.value) return tokenEntry.value;

    // Fall back to cookie-based auth token
    const cookie = state.cookies?.find((c: { name: string }) => c.name === 'openpaw_token');
    if (cookie?.value) return cookie.value;

    return '';
  } catch {
    return '';
  }
}
