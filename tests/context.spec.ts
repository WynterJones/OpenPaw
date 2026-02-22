import { test, expect } from '@playwright/test';

// ---------------------------------------------------------------------------
// Context File CRUD
//
// Tests run in serial order because they build on each other:
//   create folder → upload file → select file → rename → delete file → delete folder
// ---------------------------------------------------------------------------

test.describe.serial('Context File CRUD', () => {
  // -------------------------------------------------------------------------
  // 1. Create a folder
  // -------------------------------------------------------------------------
  test('can create a folder', async ({ page }) => {
    await page.goto('/context');
    await page.waitForLoadState('networkidle');

    // Click the "New Folder" button in the sidebar footer
    await page.getByRole('button', { name: /new folder/i }).click();

    // Modal with title "New Folder" should appear
    const modal = page.getByRole('dialog', { name: /new folder/i });
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Fill the folder name input (placeholder "e.g. Personal, Work, Projects")
    await modal.locator('input[placeholder="e.g. Personal, Work, Projects"]').fill('E2E Test Folder');

    // Click "Create Folder"
    await modal.getByRole('button', { name: /create folder/i }).click();

    // Modal closes
    await expect(modal).not.toBeVisible({ timeout: 10_000 });

    // Folder appears in the sidebar tree
    await expect(page.getByText('E2E Test Folder')).toBeVisible({ timeout: 10_000 });
  });

  // -------------------------------------------------------------------------
  // 2. Upload a file
  // -------------------------------------------------------------------------
  test('can upload a file', async ({ page }) => {
    await page.goto('/context');
    await page.waitForLoadState('networkidle');

    // The hidden file input sits in the sidebar footer area
    const fileInput = page.locator('input[type="file"]');
    await expect(fileInput).toBeAttached({ timeout: 5_000 });

    const buffer = Buffer.from('Hello from E2E test');
    await fileInput.setInputFiles({
      name: 'e2e-test.txt',
      mimeType: 'text/plain',
      buffer,
    });

    // Wait for the upload to be reflected in the sidebar
    await expect(page.getByText('e2e-test.txt')).toBeVisible({ timeout: 15_000 });
  });

  // -------------------------------------------------------------------------
  // 3. Select a file and view its content
  // -------------------------------------------------------------------------
  test('can select and view a file', async ({ page }) => {
    await page.goto('/context');
    await page.waitForLoadState('networkidle');

    // Click on the uploaded file in the sidebar to select it
    await page.getByText('e2e-test.txt').click();

    // The right panel should show a textarea with the file content
    const contentArea = page.locator('main textarea');
    await expect(contentArea).toBeVisible({ timeout: 10_000 });
    await expect(contentArea).toHaveValue('Hello from E2E test', { timeout: 10_000 });
  });

  // -------------------------------------------------------------------------
  // 4. Rename a file
  // -------------------------------------------------------------------------
  test('can rename a file', async ({ page }) => {
    await page.goto('/context');
    await page.waitForLoadState('networkidle');

    // Click on the file to select it first
    await page.getByText('e2e-test.txt').click();

    // The file header shows the filename as a clickable button — click it to enter rename mode
    const filenameButton = page.locator('main').getByRole('button', { name: 'e2e-test.txt' });
    await expect(filenameButton).toBeVisible({ timeout: 5_000 });
    await filenameButton.click();

    // An autofocused rename input should now appear in the file header bar
    // The rename input is the only input inside the main panel header when renaming
    const renameInput = page.locator('main').locator('input').first();
    await expect(renameInput).toBeVisible({ timeout: 3_000 });

    // Clear and type the new name
    await renameInput.fill('e2e-renamed.txt');
    await renameInput.press('Enter');

    // The sidebar should now show the updated filename
    await expect(page.getByText('e2e-renamed.txt')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('e2e-test.txt')).not.toBeVisible({ timeout: 5_000 });
  });

  // -------------------------------------------------------------------------
  // 5. Delete a file
  // -------------------------------------------------------------------------
  test('can delete a file', async ({ page }) => {
    await page.goto('/context');
    await page.waitForLoadState('networkidle');

    // Right-click on the renamed file to open the context menu
    await page.getByText('e2e-renamed.txt').click({ button: 'right' });

    // The context menu should appear with a "Delete" option
    const deleteMenuItem = page.getByRole('button', { name: /^delete$/i });
    await expect(deleteMenuItem).toBeVisible({ timeout: 5_000 });
    await deleteMenuItem.click();

    // The "Delete File" modal should appear
    const deleteModal = page.getByRole('dialog', { name: /delete file/i });
    await expect(deleteModal).toBeVisible({ timeout: 5_000 });

    // Click the danger "Delete" button inside the modal
    await deleteModal.getByRole('button', { name: /^delete$/i }).click();

    // Modal closes and file is removed from the sidebar
    await expect(deleteModal).not.toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('e2e-renamed.txt')).not.toBeVisible({ timeout: 10_000 });
  });

  // -------------------------------------------------------------------------
  // 6. Delete a folder
  // -------------------------------------------------------------------------
  test('can delete a folder', async ({ page }) => {
    await page.goto('/context');
    await page.waitForLoadState('networkidle');

    // Verify the folder is visible before deleting
    await expect(page.getByText('E2E Test Folder')).toBeVisible({ timeout: 5_000 });

    // Register native confirm() handler BEFORE clicking the delete button
    page.on('dialog', (dialog) => dialog.accept());

    // The delete button on the folder row has aria-label="Delete folder".
    // It is hidden (opacity-0) until hover, so hover the folder row first.
    const folderRow = page.locator('div').filter({ hasText: /^E2E Test Folder$/ }).first();
    await folderRow.hover();

    const deleteFolderBtn = folderRow.locator('button[aria-label="Delete folder"]');
    await expect(deleteFolderBtn).toBeVisible({ timeout: 5_000 });
    await deleteFolderBtn.click();

    // After accepting the confirm dialog the folder should disappear
    await expect(page.getByText('E2E Test Folder')).not.toBeVisible({ timeout: 10_000 });
  });

  // -------------------------------------------------------------------------
  // 7. About You editor persists content
  // -------------------------------------------------------------------------
  test('About You editor saves and persists content', async ({ page }) => {
    await page.goto('/context');
    await page.waitForLoadState('networkidle');

    // Click "About You" button at the top of the sidebar
    await page.getByRole('button', { name: /about you/i }).click();

    // The right panel enters "About You" mode — a textarea should appear
    const aboutTextarea = page.locator('main textarea[placeholder*="Tell your agents about yourself"]');
    await expect(aboutTextarea).toBeVisible({ timeout: 10_000 });

    // Clear any existing content and type new content
    await aboutTextarea.fill('E2E test about content');

    // The Save button becomes active (primary variant) when content is dirty
    const saveBtn = page.locator('main').getByRole('button', { name: /^save$/i });
    await expect(saveBtn).toBeEnabled({ timeout: 3_000 });
    await saveBtn.click();

    // Wait for save to complete (button returns to non-loading state)
    await expect(saveBtn).toBeDisabled({ timeout: 10_000 });

    // Navigate away and come back
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await page.goto('/context');
    await page.waitForLoadState('networkidle');

    // Click "About You" again
    await page.getByRole('button', { name: /about you/i }).click();

    // The textarea should contain the previously saved content
    const persistedTextarea = page.locator('main textarea[placeholder*="Tell your agents about yourself"]');
    await expect(persistedTextarea).toBeVisible({ timeout: 10_000 });
    await expect(persistedTextarea).toHaveValue('E2E test about content', { timeout: 10_000 });
  });
});
