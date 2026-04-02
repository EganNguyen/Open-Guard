import { test, expect } from '@playwright/test';

test.describe('Register Flow (Real BE)', () => {
  test('should register a new org and reach dashboard', async ({ page }) => {
    const unique = `${Date.now()}-${Math.floor(Math.random() * 1e9)}`;
    const testOrg = `E2E Org ${unique}`;
    const testEmail = `tester_${unique}@openguard.io`;
    const testPassword = 'Password123!';

    await page.goto('/register');

    await page.fill('input#reg-org', testOrg);
    await page.fill('input#reg-email', testEmail);
    await page.fill('input#reg-password', testPassword);
    await page.fill('input#reg-confirm', testPassword);

    await page.waitForSelector('button[type="submit"]:not([disabled])');
    const [registerResp] = await Promise.all([
      page.waitForResponse((resp) => {
        return resp.url().includes('/api/v1/auth/register') && resp.request().method() === 'POST';
      }),
      page.click('button[type="submit"]'),
    ]);
    if (registerResp.status() !== 201) {
      const body = await registerResp.text();
      throw new Error(`register failed: status=${registerResp.status()} body=${body}`);
    }

    await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 });
  });
});
