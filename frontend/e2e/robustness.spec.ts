import { test, expect, type Page } from '@playwright/test';
import { DEFAULT_FILES, DEFAULT_SETTINGS, installMock } from './mock';

async function bootstrap(
  page: Page,
  files: Record<string, string> = DEFAULT_FILES,
  settings: typeof DEFAULT_SETTINGS = DEFAULT_SETTINGS,
) {
  await installMock(page, files, settings);
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
  await page.locator('.cm-content').click();
  await page.keyboard.press('Meta+A');
  await page.keyboard.type(text);
}

test('a stale search response cannot overwrite newer results', async ({ page }) => {
  await bootstrap(page);
  await openVault(page);
  await page.evaluate(() => {
    (window as any).__mock.state.searchDelay = 600;
  });
  await page.locator('input.search').fill('kittens');
  // Let the first request start, then make the next one fast.
  await page.waitForTimeout(300);
  await page.evaluate(() => {
    (window as any).__mock.state.searchDelay = 0;
  });
  await page.locator('input.search').fill('Nested');
  await expect(page.locator('.result-path')).toHaveText('Notes/sub/Deep.md');
  // The slower first request resolves last; it must be ignored.
  await page.waitForTimeout(800);
  await expect(page.locator('.result-path')).toHaveText('Notes/sub/Deep.md');
});

test('literal square brackets in note text are not rendered as search highlights', async ({ page }) => {
  await bootstrap(page, {
    ...DEFAULT_FILES,
    'Notes/Brackets.md': '# Brackets\n\n[draft] some zebras here\n',
  });
  await openVault(page);
  await page.locator('input.search').fill('zebras');
  const snippets = page.locator('.result-snippet');
  await expect(snippets).toHaveCount(1);
  await expect(snippets.locator('mark')).toHaveText('zebras');
  await expect(snippets).toContainText('[draft]');
});

test('switching vaults saves pending edits first', async ({ page }) => {
  await bootstrap(page, DEFAULT_FILES, { ...DEFAULT_SETTINGS, autosaveMs: 5000 });
  await openVault(page);
  await selectFile(page, 'Idea.md');
  await replaceDoc(page, '# Idea\n\nUnsaved vault switch edit.\n');
  await page.getByRole('button', { name: 'Open Vault' }).click();
  await expect(page.locator('.statusbar')).toContainText('Opened vault');
  const writes = await page.evaluate(() => (window as any).__mock.state.writes);
  expect(writes.map((w: any) => w.content)).toContain('# Idea\n\nUnsaved vault switch edit.\n');
});

test('deleting a note during an in-flight save does not recreate it', async ({ page }) => {
  await bootstrap(page, DEFAULT_FILES, { ...DEFAULT_SETTINGS, autosaveMs: 5000 });
  await openVault(page);
  await selectFile(page, 'Idea.md');
  await replaceDoc(page, '# Idea\n\nDelete me.\n');
  await page.evaluate(() => {
    (window as any).__mock.state.writeDelay = 700;
  });
  // Start a slow write, then delete before it lands.
  await page.keyboard.press('Meta+S');
  page.once('dialog', (d) => d.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page.locator('.tree-item.file', { hasText: 'Idea.md' })).toHaveCount(0);
  await page.waitForTimeout(900);
  const exists = await page.evaluate(() => (window as any).__mock.content('Notes/Idea.md') !== undefined);
  expect(exists).toBe(false);
});

test('plain relative images are deferred instead of requested', async ({ page }) => {
  await bootstrap(page, {
    'Images.md': '# Images\n\n![plain](img.png)\n\n![remote](https://example.com/x.png)\n',
  });
  await openVault(page);
  await selectFile(page, 'Images.md');
  await page.getByRole('button', { name: 'Preview' }).click();
  const preview = page.locator('.preview');
  await expect(preview.locator('.deferred-image')).toContainText('[image: plain]');
  await expect(preview.locator('img')).toHaveCount(1);
  await expect(preview.locator('img')).toHaveAttribute('src', 'https://example.com/x.png');
});
