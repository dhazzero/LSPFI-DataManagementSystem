// Run against a disposable DMS database only. Creates synthetic assessment rows.
// PLAYWRIGHT_MODULE may point to an existing Playwright installation.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

(async () => {
  const url = process.env.LSPFI_BROWSER_TEST_URL;
  const password = process.env.LSPFI_BROWSER_TEST_PASSWORD;
  if (!url || !password) throw new Error('Set LSPFI_BROWSER_TEST_URL and LSPFI_BROWSER_TEST_PASSWORD for a disposable database.');
  const browser = await chromium.launch({ headless: true, ...(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {}) });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const field = name => page.locator(`#record-form [name="${name}"]`);
  const save = () => page.locator('#record-form button[type="submit"]').click();
  const awaitNumber = async n => {
    await page.waitForFunction(n => document.querySelector('#record-form [name="certificate"]')?.value.includes(String(n).padStart(7, '0')), n);
  };
  const open = async (scheme, nik) => {
    await page.locator('[data-action="new"]').click();
    await field('certificate_year').waitFor();
    assert.equal(await field('certificate_year').count(), 1);
    await field('name').fill('ASESI UJI BROWSER');
    await field('nik').fill(nik);
    await field('birth_date').fill('1990-02-01');
    await field('email').fill('browser@example.test');
    await field('phone').fill('081234567890');
    await field('scheme').selectOption(scheme);
  };
  try {
    await page.goto(url);
    await page.locator('#login-form [name="username"]').fill('admin');
    await page.locator('#login-form [name="password"]').fill(password);
    await page.locator('#login-form button[type="submit"]').click();
    await page.locator('[data-nav="registrations"]').click();
    await page.getByRole('heading', { name: 'Nomor registrasi & sertifikat', exact: true }).waitFor();
    await open('24', '0000000000000101');
    await field('registration_year').fill('2026');
    await field('certificate_year').fill('2027');
    await save();
    await awaitNumber(1);
    assert.equal(await field('certificate').inputValue(), '64911 4210 3 0000001 2027');
    assert.equal(await field('registration').inputValue(), 'PPL 2605 00001');
    await field('name').fill('ASESI UJI BROWSER DIPERBARUI');
    await save();
    await page.waitForFunction(() => document.querySelector('#detail-content h2')?.textContent === 'ASESI UJI BROWSER DIPERBARUI');
    assert.equal(await field('certificate').inputValue(), '64911 4210 3 0000001 2027');
    await page.locator('[data-action="detail-tab"][data-tab="docs"]').click();
    await page.locator('[data-action="detail-tab"][data-tab="data"]').click();
    assert.equal(await field('certificate_year').inputValue(), '2027');
    await page.locator('[data-action="close"]').click();

    await open('25', '0000000000000102');
    await page.locator('#certificate-auto').uncheck();
    await field('certificate').fill('MANUAL-DRAFT');
    await page.locator('#certificate-auto').check();
    assert.equal(await field('certificate').inputValue(), '');
    await page.locator('#certificate-auto').uncheck();
    assert.equal(await field('certificate').inputValue(), 'MANUAL-DRAFT');
    await page.locator('#certificate-auto').check();
    await save();
    await awaitNumber(2);
    assert.equal(await field('registration').inputValue(), 'PPL 2605 00001');
    await page.locator('[data-action="close"]').click();

    await open('99', '0000000000000103');
    await save();
    await page.locator('#record-error').filter({ hasText: 'metadata skema 99 belum tersedia' }).waitFor();
    await field('scheme').selectOption('24');
    await save();
    await awaitNumber(3);
    assert.equal(await field('registration').inputValue(), 'PPL 2605 00002');
    await page.locator('[data-action="close"]').click();

    await open('24', '0000000000000104');
    await page.locator('#certificate-auto').uncheck();
    await save();
    await page.waitForFunction(() => document.querySelector('#record-form [name="registration"]')?.value === 'PPL 2605 00003');
    assert.equal(await field('certificate').inputValue(), '');
    await page.locator('[data-action="close"]').click();
    const missing = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Arsip belum memiliki nomor sertifikat', exact: true }) });
    await missing.locator('[data-action="detail"]').click();
    await page.locator('#certificate-auto').check();
    await save();
    await awaitNumber(4);
    assert.equal(await field('registration').inputValue(), 'PPL 2605 00003');
    await page.locator('[data-action="close"]').click();
    await page.reload();
    await page.getByRole('heading', { name: 'Nomor registrasi & sertifikat', exact: true }).waitFor();
    await page.getByRole('heading', { name: 'Semua arsip sudah memiliki nomor sertifikat', exact: true }).waitFor();
    for (const file of ['manifest.json', 'master_data.json', 'master_data.sql', 'registrasiAsesi.ts']) {
      const response = await page.request.get(`${url}/data/${file}`);
      assert.equal(response.status(), 404, file);
    }
    assert.equal(await page.locator('.certificate-preview').evaluate(el => getComputedStyle(el).fontSize), '16px');
    assert.deepEqual(errors, []);
    if (process.env.LSPFI_BROWSER_TEST_SCREENSHOT) {
      const output = process.env.LSPFI_BROWSER_TEST_SCREENSHOT;
      fs.mkdirSync(path.dirname(output), { recursive: true });
      await page.screenshot({ path: output, fullPage: true });
    }
    await page.setViewportSize({ width: 390, height: 844 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true, 'mobile page overflow');
    console.log('PASS: login, generated numbers, manual toggle, edits, rollback, missing certificate completion, reload, private assets, and no browser errors.');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
