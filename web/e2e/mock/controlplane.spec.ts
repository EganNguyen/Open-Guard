import { test, expect } from '@playwright/test';

test.describe('Control Plane Integration', () => {
  const MOCK_TOKEN = 'mock-jwt-token';
  
  test.beforeEach(async ({ page }) => {
    // Mock the Auth API
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({
          access_token: MOCK_TOKEN,
          refresh_token: 'mock-refresh-token',
          expires_in: 3600,
          token_type: 'Bearer'
        }),
      });
    });

    // Mock the Users API
    await page.route('**/api/v1/users', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({
          data: Array(42).fill({ id: '1', email: 'test@example.com' }),
          meta: { total_items: 42, total_pages: 1, page: 1, per_page: 50 }
        }),
      });
    });

    // Mock the Policies API
    await page.route('**/api/v1/policies', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({
          data: Array(7).fill({ id: '1', name: 'Policy 1' }),
          meta: { total: 7 }
        }),
      });
    });

    // Mock failing services (503)
    await page.route('**/api/v1/threats', async (route) => {
      await route.fulfill({ status: 503, headers: { 'Access-Control-Allow-Origin': '*' }, body: 'Service Unavailable' });
    });
    await page.route('**/api/v1/audit', async (route) => {
      await route.fulfill({ status: 503, headers: { 'Access-Control-Allow-Origin': '*' }, body: 'Service Unavailable' });
    });
    await page.route('**/api/v1/alerts', async (route) => {
      await route.fulfill({ status: 503, headers: { 'Access-Control-Allow-Origin': '*' }, body: 'Service Unavailable' });
    });
  });

  test('should login and display real counts on dashboard', async ({ page }) => {
    // 1. Login
    await page.goto('/login');
    await page.fill('input#login-email', 'admin@openguard.io');
    await page.fill('input#login-password', 'password123');
    await page.click('button[type="submit"]');

    // 2. Verify redirect
    await expect(page).toHaveURL(/\/dashboard/);

    // 3. Verify token storage
    const token = await page.evaluate(() => localStorage.getItem('access_token'));
    expect(token).toBe(MOCK_TOKEN);

    // 4. Verify Dashboard Counts
    await expect(page.getByTestId('user-count')).toHaveText('42');
    await expect(page.getByTestId('policy-count')).toHaveText('7');
  });

  test('should navigate to Connectors and show Phase 6 badge', async ({ page }) => {
    await page.goto('/dashboard/connectors');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('Connected Applications');
    await expect(title.getByText('Phase 6')).toBeVisible();
  });

  test('should navigate to Threat Detection and show Phase 4 badge', async ({ page }) => {
    await page.goto('/dashboard/threats');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('Live Threat Stream');
    await expect(title.getByText('Phase 4')).toBeVisible();
  });

  test('should navigate to Audit Log and show Phase 3 badge', async ({ page }) => {
    await page.goto('/dashboard/audit');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('System Audit Events');
    await expect(title.getByText('Phase 3')).toBeVisible();
  });

  test('should navigate to Compliance and show Phase 5 badge', async ({ page }) => {
    await page.goto('/dashboard/compliance');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('Compliance Reports');
    await expect(title.getByText('Phase 5')).toBeVisible();
  });

  test('should load dashboard even when some services fail', async ({ page }) => {
    // Manual setup: put token in localStorage then goto dashboard
    await page.goto('/login');
    await page.evaluate((token) => localStorage.setItem('access_token', token), MOCK_TOKEN);
    await page.goto('/dashboard');

    // Verify main components still render
    await expect(page.getByText('Security recommendations')).toBeVisible();
    await expect(page.getByTestId('user-count')).toHaveText('42');
  });
});
