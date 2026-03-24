import { test, expect } from '@playwright/test';

test.describe('IAM Web UI Flow', () => {
  const MOCK_TOKEN = 'mock-iam-jwt';

  test.beforeEach(async ({ page }) => {
    // Mock login to bypass auth
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        headers: { 'Access-Control-Allow-Origin': '*' },
        body: JSON.stringify({ access_token: MOCK_TOKEN }),
      });
    });

    // Mock IAM/Policy APIs with CORS headers
    const mockHeaders = { 'Access-Control-Allow-Origin': '*' };
    
    await page.route('**/api/v1/users', async (route) => {
      await route.fulfill({ status: 200, headers: mockHeaders, contentType: 'application/json', body: JSON.stringify({ data: [], meta: { total_items: 0 } }) });
    });
    await page.route('**/api/v1/identity-providers', async (route) => {
      await route.fulfill({ status: 200, headers: mockHeaders, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
    });
    await page.route('**/api/v1/auth-policies', async (route) => {
      await route.fulfill({ status: 200, headers: mockHeaders, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
    });
    await page.route('**/api/v1/external-users', async (route) => {
      await route.fulfill({ status: 200, headers: mockHeaders, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
    });
    await page.route('**/api/v1/access-policies', async (route) => {
      await route.fulfill({ status: 200, headers: mockHeaders, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
    });

    // Go to login and set token
    await page.goto('/login');
    await page.evaluate((token) => localStorage.setItem('access_token', token), MOCK_TOKEN);
  });

  test('should navigate to Users and show Phase 1 badge', async ({ page }) => {
    await page.goto('/dashboard/users');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('Organization Users');
    await expect(title.getByText('Phase 1')).toBeVisible();
  });

  test('should navigate to Identity Providers and show Phase 1 badge', async ({ page }) => {
    await page.goto('/dashboard/identity-providers');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('External IdPs');
    await expect(title.getByText('Phase 1')).toBeVisible();
  });

  test('should navigate to Auth Policies and show Phase 6 badge', async ({ page }) => {
    await page.goto('/dashboard/auth-policies');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('Auth Configuration');
    await expect(title.getByText('Phase 6')).toBeVisible();
  });

  test('should navigate to External Users and show Phase 6 badge', async ({ page }) => {
    await page.goto('/dashboard/external-users');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('External Collaboration');
    await expect(title.getByText('Phase 6')).toBeVisible();
  });

  test('should navigate to Access Policies and show Phase 2 badge', async ({ page }) => {
    await page.goto('/dashboard/access-policies');
    const title = page.getByTestId('page-title');
    await expect(title).toContainText('Policy Registry');
    await expect(title.getByText('Phase 2')).toBeVisible();
  });
});
