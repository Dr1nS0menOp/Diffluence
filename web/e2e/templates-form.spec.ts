// SPDX-License-Identifier: Apache-2.0
// Spec 074, wave GT2 in the browser: the form of a template, its check step, and the
// sections of the template editor, by keyboard where a person would use one.
import type { Page } from '@playwright/test';
import { ADMIN, checkAccessibility, editorSettled, expect, login, newSpace, test, unique } from './helpers';

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

type Spec = Record<string, unknown> & { name: string };

/** A template of a space whose text asks each question alone in its paragraph. */
async function makeTemplate(
  page: Page,
  spaceId: string,
  name: string,
  specs: Spec[],
  steps: { title: string; fields: string[] }[] = [],
) {
  const document = {
    type: 'doc',
    content: specs.map((s) => ({
      type: 'paragraph',
      content: [{ type: 'templateField', attrs: { name: s.name, kind: 'line' } }],
    })),
  };
  const made = await page.request.post(`/api/v1/spaces/${spaceId}/templates`, { data: { name, document } });
  expect(made.status(), await made.text()).toBe(201);
  const id = ((await made.json()) as { id: string }).id;
  const fields = await page.request.put(`/api/v1/templates/${id}/fields`, { data: { fields: specs, steps } });
  expect(fields.status(), await fields.text()).toBe(200);
  return id;
}

/** Opens the form of a template from Create, through the gallery. */
async function openForm(page: Page, spaceKey: string, name: string) {
  await page.goto(`/spaces/${spaceKey}/overview`);
  await page.getByRole('button', { name: 'More to create' }).click();
  await page.getByRole('menuitem', { name: 'From a template' }).click();
  const gallery = page.getByRole('dialog', { name: 'Start from a template' });
  const find = gallery.getByRole('combobox', { name: 'Find a template' });
  await find.fill(name);
  await expect(gallery.getByRole('option', { name: new RegExp(name) })).toBeVisible();
  // Enter uses the template the preview loaded, so the preview comes first.
  await expect(gallery.locator('.prose')).toBeVisible();
  await find.press('Enter');
  const form = page.getByRole('dialog', { name });
  await expect(form).toBeVisible();
  return form;
}

test('the form by keyboard: errors beside the field and in a summary, never while typing (TPL-321, TPL-322)', async ({
  page,
}) => {
  const space = await newSpace(page, 'Form errors');
  const name = unique('Request');
  await makeTemplate(page, space.id, name, [
    { name: 'Name', kind: 'line', required: true, min_length: 3 },
    { name: 'Hours', kind: 'number', min: 1, max: 40 },
  ]);
  const form = await openForm(page, space.key, name);
  const who = form.getByLabel('Name (required)');
  const hours = form.getByLabel('Hours');
  await checkAccessibility(page);

  // Typing in an empty field shows no error; leaving it does, in words beside it.
  await who.focus();
  await page.keyboard.type('A');
  await expect(form.locator('.field-error')).toHaveCount(0);
  await page.keyboard.press('Tab');
  await expect(form.getByText('Enter at least 3 characters.')).toBeVisible();
  await expect(who).toHaveAttribute('aria-invalid', 'true');
  // The error goes as soon as the answer is right.
  await who.focus();
  await page.keyboard.type('da');
  await expect(form.getByText('Enter at least 3 characters.')).toHaveCount(0);

  // Next with an error moves the focus to a summary that links to the field.
  await hours.fill('50');
  await hours.press('Enter');
  const summary = form.getByRole('alert').filter({ hasText: 'Some answers need a change' });
  await expect(summary).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(summary.getByRole('link', { name: 'Hours: Enter a number of at most 40.' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(hours).toBeFocused();
  await checkAccessibility(page);
  await hours.fill('7,5');
  await expect(
    form
      .locator('.field-error')
      .filter({ hasText: 'Enter a number with at most 0 digits after the decimal sign.' }),
  ).toBeVisible();
  await hours.fill('8');
  await hours.press('Enter');

  // The check step lists the answers with a way back to each, and the page it will make.
  await expect(form.getByRole('heading', { name: 'Check before making' })).toBeVisible();
  await expect(form.getByRole('definition').filter({ hasText: 'Ada' })).toBeVisible();
  await expect(form.getByText('The pages start as drafts of yours')).toBeVisible();
  await checkAccessibility(page);
  await form.getByRole('button', { name: 'Change Hours' }).click();
  await expect(hours).toBeFocused();
  await expect(hours).toHaveValue('8');
  await hours.press('Enter');
  await form.getByRole('button', { name: 'Create 1 draft' }).click();
  await expect(page.getByLabel('Title', { exact: true })).toBeVisible();
  await expect(page.locator('.prose.editing')).toContainText('Ada');
});

test('Back and a refusal of the server keep every answer, and closing asks first (TPL-324, TPL-332)', async ({
  page,
}) => {
  const space = await newSpace(page, 'Form answers');
  const name = unique('Kick-off');
  await makeTemplate(
    page,
    space.id,
    name,
    [
      { name: 'Topic', kind: 'line', required: true },
      { name: 'Tools', kind: 'choices', options: ['Laptop', 'Phone', 'Badge'] },
      { name: 'Remote', kind: 'yesno' },
    ],
    [
      { title: 'About', fields: ['Topic'] },
      { title: 'Equipment', fields: ['Tools', 'Remote'] },
    ],
  );
  const form = await openForm(page, space.key, name);
  await expect(form.getByRole('status').filter({ hasText: 'Step 1 of 3' })).toBeVisible();
  await form.getByLabel('Topic (required)').fill('Roadmap');
  await form.getByRole('button', { name: 'Next' }).click();
  await expect(form.getByRole('status').filter({ hasText: 'Step 2 of 3' })).toBeVisible();
  await form.getByRole('checkbox', { name: 'Phone' }).check();
  await form.getByRole('checkbox', { name: 'Remote' }).check();
  await form.getByRole('button', { name: 'Back' }).click();
  await expect(form.getByLabel('Topic (required)')).toHaveValue('Roadmap');
  await form.getByRole('button', { name: 'Next' }).click();
  await expect(form.getByRole('checkbox', { name: 'Phone' })).toBeChecked();

  // The server refuses an answer the browser took: the words stand beside the field, and
  // every other answer stays.
  await page.route('**/sets/check', (route) =>
    route.fulfill({
      status: 400,
      contentType: 'application/json',
      body: JSON.stringify({
        error: {
          code: 'invalid_answers',
          message: 'Some answers need a change.',
          fields: [
            {
              field: 'Topic',
              problem: 'too_long',
              params: { max: '3' },
              message: 'Enter at most 3 characters.',
            },
          ],
        },
      }),
    }),
  );
  await form.getByRole('button', { name: 'Next' }).click();
  await expect(form.getByLabel('Topic (required)')).toHaveValue('Roadmap');
  await expect(form.getByText('Enter at most 3 characters.').first()).toBeVisible();
  await page.unroute('**/sets/check');
  await form.getByLabel('Topic (required)').fill('Q3');
  await form.getByRole('button', { name: 'Next' }).click();
  await expect(form.getByRole('checkbox', { name: 'Phone' })).toBeChecked();

  // Escape asks before the answers are lost; Cancel keeps them.
  await page.keyboard.press('Escape');
  const asking = page.getByRole('dialog', { name: 'Close the form' });
  await expect(asking).toBeVisible();
  await asking.getByRole('button', { name: 'Cancel' }).click();
  await expect(form.getByRole('checkbox', { name: 'Phone' })).toBeChecked();
  await page.keyboard.press('Escape');
  await page
    .getByRole('dialog', { name: 'Close the form' })
    .getByRole('button', { name: 'Close and lose the answers' })
    .click();
  await expect(form).toHaveCount(0);
});

test('the sections of the template editor by keyboard: a set three levels deep, a question and a step (TPL-1010 to TPL-1012)', async ({
  page,
}) => {
  const space = await newSpace(page, 'Template editor');
  const name = unique('Onboarding');
  const id = await makeTemplate(page, space.id, name, [{ name: 'Topic', kind: 'line' }]);

  // Pages: added and indented with the keyboard only, no dragging.
  await page.goto(`/templates/${id}/settings?section=pages`);
  const pages = page.getByRole('region', { name: 'Pages this template makes' });
  for (const entry of ['Equipment', 'Laptop', 'Charger']) {
    await pages.getByLabel('Name of the page').fill(entry);
    await pages.getByLabel('Name of the page').press('Enter');
    await expect(pages.getByRole('cell', { name: entry, exact: true })).toBeVisible();
  }
  await pages
    .getByRole('row', { name: /^Laptop/ })
    .getByRole('button', { name: 'Indent one level' })
    .focus();
  await page.keyboard.press('Enter');
  await expect(pages.getByRole('status')).toHaveText('This template makes up to 4 pages, 3 levels deep.');
  await pages
    .getByRole('row', { name: /^Charger/ })
    .getByRole('button', { name: 'Indent one level' })
    .press('Enter');
  await expect(pages.getByRole('cell', { name: 'Charger', exact: true })).toHaveAttribute('data-depth', '2');
  await pages
    .getByRole('row', { name: /^Charger/ })
    .getByRole('button', { name: 'Indent one level' })
    .press('Enter');
  await expect(pages.getByRole('status')).toHaveText('This template makes up to 4 pages, 4 levels deep.');
  await pages
    .getByRole('row', { name: /^Charger/ })
    .getByRole('button', { name: 'Outdent one level' })
    .press('Enter');
  await expect(pages.getByRole('status')).toHaveText('This template makes up to 4 pages, 3 levels deep.');
  await checkAccessibility(page);

  // Form: a question's settings in a dialog, and a step.
  await page.getByRole('link', { name: 'Form' }).click();
  await expect(page.getByRole('heading', { name: 'Form', level: 1 })).toBeVisible();
  await page.getByRole('button', { name: 'Settings of Topic' }).press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Settings of Topic' });
  await dialog.getByLabel('Kind of answer').selectOption('number');
  await dialog.getByLabel('Lowest', { exact: true }).fill('5');
  await dialog.getByLabel('Highest', { exact: true }).fill('1');
  await dialog.getByRole('button', { name: 'Keep these settings' }).press('Enter');
  await expect(dialog.getByText('The lowest number cannot be above the highest.')).toBeVisible();
  await checkAccessibility(page);
  await dialog.getByLabel('Highest', { exact: true }).fill('10');
  await dialog.getByRole('button', { name: 'Keep these settings' }).press('Enter');
  await expect(dialog).toHaveCount(0);
  await page.getByRole('button', { name: 'Add a step' }).press('Enter');
  await page.getByLabel('Step of Topic').selectOption({ label: 'Step 1' });
  await page.getByRole('button', { name: 'Save', exact: true }).press('Enter');
  await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole('row', { name: /Topic/ }).getByRole('cell', { name: 'A number' }),
  ).toBeVisible();

  // Versions: each save is a version, with what it changed.
  await page.getByRole('link', { name: 'Versions' }).click();
  await page.getByRole('button', { name: /Changes in version 1/ }).press('Enter');
  await expect(page.getByRole('region', { name: 'What version 1 changed' })).toBeVisible();
  await checkAccessibility(page);
});

test('/ offers the values of a guided template and links to the pages of its set (TPL-1013)', async ({
  page,
}) => {
  const space = await newSpace(page, 'Template values');
  await page.goto(`/spaces/${space.key}/templates`);
  const name = unique('Project');
  await page.getByLabel('New template name').fill(name);
  await page.getByRole('button', { name: 'New template' }).click();
  const pages = page.getByRole('region', { name: 'Pages this template makes' });
  await pages.getByLabel('Name of the page').fill('Risks');
  await pages.getByRole('button', { name: 'Add a page' }).click();
  await expect(pages.getByRole('cell', { name: 'Risks', exact: true })).toBeVisible();
  await page.locator('.prose.editing').first().click();
  await editorSettled(page);
  await page.keyboard.type('/value');
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Value of the template' });
  await dialog.getByLabel('Which value').selectOption({ label: 'Link to Risks' });
  await dialog.getByRole('button', { name: 'Add' }).click();
  await expect(page.locator('.prose.editing [data-value^="entry:"]')).toBeVisible();
});

test('an empty page offers templates beside the editor (ORG-045)', async ({ page }) => {
  const space = await newSpace(page, 'Template panel');
  const created = await page.request.post('/api/v1/pages', {
    data: { space_id: space.id, parent_id: null, title: 'Blank' },
  });
  const { id } = (await created.json()) as { id: string };
  await page.goto(`/pages/${id}/edit`);
  const panel = page.getByRole('complementary', { name: 'Start it from a template' });
  await expect(panel).toBeVisible();
  await panel.getByLabel('Find a template').fill('Meeting');
  await panel.getByRole('button', { name: 'Meeting notes' }).click();
  const gallery = page.getByRole('dialog', { name: 'Start from a template' });
  await expect(gallery.getByRole('option', { name: /Meeting notes/ })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await checkAccessibility(page);
  await gallery.getByRole('button', { name: 'Close' }).click();
  // Typing in the page puts the panel away.
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.keyboard.type('Notes');
  await expect(panel).toHaveCount(0);
});

// Spec 074, wave GT3: conditions.

test('a question a condition shows appears after its cause, is announced, and the focus stays (TPL-1023)', async ({
  page,
}) => {
  const space = await newSpace(page, 'Form conditions');
  const name = unique('Site visit');
  await makeTemplate(page, space.id, name, [
    { name: 'Works on site', kind: 'yesno' },
    {
      name: 'Badge number',
      kind: 'line',
      required: true,
      condition: { match: 'all', tests: [{ subject: 'field', field: 'Works on site', op: 'yes' }] },
    },
  ]);
  const form = await openForm(page, space.key, name);
  const cause = form.getByRole('checkbox', { name: 'Works on site' });
  const badge = form.getByLabel('Badge number (required)');
  await expect(badge).toHaveCount(0);
  await cause.focus();
  await page.keyboard.press('Space');
  await expect(badge).toBeVisible();
  await expect(form.locator('[aria-live="polite"]')).toHaveText('1 more question');
  await expect(cause).toBeFocused();
  await checkAccessibility(page);
  // The new question comes after the one that caused it, in the order of the Tab key.
  await page.keyboard.press('Tab');
  await expect(badge).toBeFocused();
  await page.keyboard.type('B-12');
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('Space');
  await expect(badge).toHaveCount(0);
  await expect(form.locator('[aria-live="polite"]')).toHaveText('1 question fewer');
  // Hidden, the required question does not block, and its answer is not sent.
  const sent = page.waitForRequest((r) => r.url().includes('/sets/check'));
  await page.keyboard.press('Enter');
  expect((await sent).postData() ?? '').not.toContain('B-12');
  await expect(form.getByRole('heading', { name: 'Check before making' })).toBeVisible();
});

test('a condition is edited by keyboard, and its sentence says what it means (TPL-1014)', async ({
  page,
}) => {
  const space = await newSpace(page, 'Condition editor');
  const name = unique('Incident');
  const id = await makeTemplate(page, space.id, name, [
    { name: 'Severity', kind: 'choice', options: ['Low', 'High'] },
    { name: 'Statement', kind: 'text' },
  ]);
  await page.goto(`/templates/${id}/settings?section=form`);
  const statementRow = page
    .getByRole('row')
    .filter({ has: page.getByRole('cell', { name: 'Statement', exact: true }) });
  await statementRow.getByRole('button', { name: 'Add a condition' }).click();
  const dialog = page.getByRole('dialog', { name: 'Condition: Statement' });
  // The first test starts on the only earlier question; the value is chosen by keyboard.
  await expect(dialog.getByLabel('Test 1: what is tested')).toHaveValue('Severity');
  await dialog.getByLabel('Test 1: how').selectOption('is');
  await dialog.getByLabel('Test 1: how').focus();
  await page.keyboard.press('Tab');
  await expect(dialog.getByLabel('Test 1: value')).toBeFocused();
  await dialog.getByLabel('Test 1: value').selectOption('High');
  await expect(dialog.getByText('Only when Severity is High')).toBeVisible();
  // A second test of a question that is not earlier is not offered; any of turns the sentence.
  await dialog.getByRole('button', { name: 'Add a test' }).press('Enter');
  await dialog.getByLabel('Test 2: how').selectOption('empty');
  await dialog.getByLabel('Applies when').selectOption('any');
  await expect(dialog.getByText('Only when Severity is High or Severity is empty')).toBeVisible();
  await expect(
    dialog.getByLabel('Test 2: what is tested').locator('option', { hasText: 'Statement' }),
  ).toHaveCount(0);
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Remove test 2' }).press('Enter');
  await dialog.getByRole('button', { name: 'Keep this condition' }).press('Enter');
  await expect(page.getByRole('cell', { name: /Only when Severity is High/ })).toBeVisible();
  await page.getByRole('button', { name: 'Save' }).press('Enter');
  await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('cell', { name: /Only when Severity is High/ })).toBeVisible();
});
