import { test, expect } from '@playwright/test';

test.describe('Authentication', () => {
  test('should show login page', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('h2')).toContainText('Sign in to your account');
  });

  test('should allow login with valid credentials', async ({ page }) => {
    // Note: This requires a running backend or mock
    await page.goto('/login');
    await page.fill('input[formControlName="email"]', 'admin@openguard.test');
    await page.fill('input[formControlName="password"]', 'admin123');
    await page.click('button[type="submit"]');
    
    // Should redirect to dashboard or MFA if enabled
    // await expect(page).toHaveURL('/');
  });

  test('should redirect to MFA if required', async ({ page }) => {
    // Mocking or setup would go here
  });
});
