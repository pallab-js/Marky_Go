import { test, expect, type Page } from '@playwright/test';
import { DEFAULT_FILES, installMock } from './mock';

async function bootstrap(page: Page, files = DEFAULT_FILES) {
  await installMock(page, files);
  await page.goto('/');
}

async function openVault(page: Page) {
  await page.getByRole('button', { name: 'Open Vault' }).click();
  await expect(page.locator('.statusbar')).toContainText('Opened vault');
}

async function selectFile(page: Page, name: string) {
  await page.locator('.tree-item.file', { hasText: name }).first().click();
  await expect(page.locator('.doc-title')).toContainText(name);
}

async function replaceDoc(page: Page, text: string) {
  const content = page.locator('.cm-content');
  await content.click();
  await page.keyboard.press('ControlOrMeta+A');
  await page.keyboard.type(text);
}

test('rename while dirty keeps the user edits', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await selectFile(page, 'Idea.md');
  await replaceDoc(page, '# Idea\n\nPrecious unsaved edit.\n');
  // Rename immediately, before the autosave timer fires.
  page.once('dialog', (d) => d.accept('Notes/Renamed.md'));
  await page.getByRole('button', { name: 'Rename' }).click();
  await expect(page.locator('.doc-title')).toHaveText('Notes/Renamed.md');
  const content = await page.evaluate(() => (window as any).__mock.content('Notes/Renamed.md'));
  expect(content).toBe('# Idea\n\nPrecious unsaved edit.\n');
});

test('switching notes during a conflict does not discard the edits', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await selectFile(page, 'Idea.md');
  await replaceDoc(page, '# Idea\n\nPrecious unsaved edit.\n');
  await page.evaluate(() => (window as any).__mock.setFile('Notes/Idea.md', '# Idea\n\nExternal version.\n'));
  await expect(page.locator('.conflict')).toBeVisible();
  await page.locator('.tree-item.file', { hasText: 'Deep.md' }).first().click();
  // Still on the conflicted note until the user resolves it.
  await expect(page.locator('.doc-title')).toHaveText('Notes/Idea.md');
  await expect(page.locator('.conflict')).toBeVisible();
  await expect(page.locator('.cm-content')).toContainText('Precious unsaved edit');
});

test('new note accepts an uppercase .MD extension without doubling it', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  page.once('dialog', (d) => d.accept('Shout.MD'));
  await page.getByRole('button', { name: 'New Note' }).click();
  await expect(page.locator('.doc-title')).toHaveText('Shout.MD');
  const exists = await page.evaluate(() => (window as any).__mock.content('Shout.MD') !== undefined);
  expect(exists).toBe(true);
});
