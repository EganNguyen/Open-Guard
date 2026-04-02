import { test, expect } from '@playwright/test';

test.describe('Login Flow', () => {
  test.beforeEach(async ({ page }) => {
    await page.route('**/api/v1/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          access_token: 'mock-token',
          refresh_token: 'mock-refresh',
          expires_in: 3600,
          token_type: 'Bearer'
        }),
      });
    });
  });

  test('should render the login page', async ({ page }) => {
    await page.goto('/login');
    
    // Check heading
    await expect(page.locator('h1')).toHaveText('Sign in to your account');
    
    // Check form fields
    await expect(page.locator('label[for="login-email"]')).toBeVisible();
    await expect(page.locator('input#login-email')).toBeVisible();
    
    await expect(page.locator('label[for="login-password"]')).toBeVisible();
    await expect(page.locator('input#login-password')).toBeVisible();
    
    // Check submit button
    await expect(page.locator('button[type="submit"]')).toContainText('Sign in');
  });

  test('should succeed with mock credentials', async ({ page }) => {
    await page.goto('/login');
    
    await page.fill('input#login-email', 'admin@acme.com');
    await page.fill('input#login-password', 'correctpass');
    
    await page.click('button[type="submit"]');
    
    // Verify redirection to dashboard
    await expect(page).toHaveURL(/\/dashboard/);
  });
});
