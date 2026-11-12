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
  await page.keyboard.press('Meta+A');
  await page.keyboard.type(text);
}

test('welcome screen and recent vaults render', async ({ page }) => {
  await bootstrap(page);
  await expect(page.getByRole('heading', { name: 'Welcome to Markvault' })).toBeVisible();
  await expect(page.getByText('Choose folder… (Ctrl+O)')).toBeVisible();
  await expect(page.locator('.recent button')).toHaveText(['/mock/vault']);
});

test('open vault, render tree, and select a note', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await expect(page.locator('.meta')).toContainText('vault · 3 notes');
  await expect(page.locator('.tree-item.file')).toHaveCount(3);
  await selectFile(page, 'Idea.md');
  await expect(page.locator('.cm-content')).toContainText('searchable thought about kittens');
});

test('autosave persists edits and reports success', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await selectFile(page, 'Idea.md');
  await replaceDoc(page, '# Idea\n\nEdited by playwright.\n');
  await expect(page.locator('.doc-meta')).toContainText('Saved', { timeout: 5000 });
  const content = await page.evaluate(() => (window as any).__mock.content('Notes/Idea.md'));
  expect(content).toBe('# Idea\n\nEdited by playwright.\n');
});

test('search finds notes and opens the selected result', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await page.locator('input.search').fill('kittens');
  await expect(page.locator('.result')).toHaveCount(1);
  await page.locator('.result').click();
  await expect(page.locator('.doc-title')).toHaveText('Notes/Idea.md');
});

test('preview renders GFM and neutralizes unsafe content', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await selectFile(page, 'Welcome.md');
  await page.getByRole('button', { name: 'Preview' }).click();
  const preview = page.locator('.preview');
  await expect(preview.getByRole('link', { name: 'world' })).toHaveAttribute('href', 'https://example.com');
  await expect(preview.locator('.unsafe-link')).toHaveText('bad');
  await expect(preview.locator('a[href^="javascript"]')).toHaveCount(0);
  await expect(preview.locator('script')).toHaveCount(0);
  expect(await page.evaluate(() => (window as any).__pwned)).toBeUndefined();
  await expect(preview.locator('.deferred-image')).toBeVisible();
});

test('new note prompt creates and opens a note', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  page.once('dialog', (d) => d.accept('Fresh'));
  await page.getByRole('button', { name: 'New Note' }).click();
  await expect(page.locator('.doc-title')).toHaveText('Fresh.md');
  const content = await page.evaluate(() => (window as any).__mock.content('Fresh.md'));
  expect(content).toBe('# Fresh\n');
});

test('rename accepts a name without .md extension', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await selectFile(page, 'Idea.md');
  page.once('dialog', (d) => d.accept('Renamed'));
  await page.getByRole('button', { name: 'Rename' }).click();
  await expect(page.locator('.doc-title')).toHaveText('Renamed.md');
  await expect(page.locator('.error')).toHaveCount(0);
});

test('delete moves the open note out of the tree', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await selectFile(page, 'Idea.md');
  page.once('dialog', (d) => d.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page.locator('.tree-item.file', { hasText: 'Idea.md' })).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Select a note' })).toBeVisible();
});

test('external edit during unsaved edits shows conflict and keep-my-version wins', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await selectFile(page, 'Idea.md');
  await replaceDoc(page, '# Idea\n\nUser version.\n');
  await page.evaluate(() => (window as any).__mock.setFile('Notes/Idea.md', '# Idea\n\nExternal version.\n'));
  await expect(page.locator('.conflict')).toBeVisible();
  await page.getByRole('button', { name: 'Keep my version' }).click();
  await expect(page.locator('.conflict')).toHaveCount(0);
  const content = await page.evaluate(() => (window as any).__mock.content('Notes/Idea.md'));
  expect(content).toBe('# Idea\n\nUser version.\n');
});

test('external edit of a clean note reloads without marking it dirty', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await selectFile(page, 'Idea.md');
  await page.evaluate(() => (window as any).__mock.setFile('Notes/Idea.md', '# Idea\n\nExternal version.\n'));
  await expect(page.locator('.cm-content')).toContainText('External version');
  await page.waitForTimeout(600);
  await expect(page.locator('.doc-meta')).toContainText('Ready');
  const writes = await page.evaluate(() => (window as any).__mock.state.writes);
  expect(writes).toHaveLength(0);
});
