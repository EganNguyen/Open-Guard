import { test, expect } from '@playwright/test';

test.describe('Audit Log Flow', () => {
  const MOCK_TOKEN = 'mock-audit-jwt';

  test.beforeEach(async ({ page }) => {
    // Mock login
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({ access_token: MOCK_TOKEN }),
      });
    });

    // Mock Audit APIs
    await page.route('**/api/v1/audit', async (route) => {
      await route.fulfill({
        status: 200,
        headers: { 'Access-Control-Allow-Origin': '*' },
        contentType: 'application/json',
        body: JSON.stringify({
          data: [
            { timestamp: '2026-03-24 10:45:12', actor: 'admin@openguard.io', action: 'policy.create', target: 'pol_rbac_01', status: 'success' },
            { timestamp: '2026-03-24 10:48:05', actor: 'system', action: 'threat.detected', target: 'brute_force_attack', status: 'alert' }
          ],
          meta: { total: 2 }
        })
      });
    });

    // Go to login and set token
    await page.goto('/login');
    await page.evaluate((token) => localStorage.setItem('access_token', token), MOCK_TOKEN);
  });

  test('should display Audit Log with Phase 3 badge', async ({ page }) => {
    await page.goto('/dashboard/audit');
    
    // Verify title and badge
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('System Audit Events');
    await expect(title.getByText('Phase 3')).toBeVisible();

    // Verify table visibility
    await expect(page.getByTestId('audit-list')).toBeVisible();
    await expect(page.getByText('admin@openguard.io')).toBeVisible();
    await expect(page.getByText('brute_force_attack')).toBeVisible();
  });
});
