import { test, expect } from '@playwright/test';

test.describe('Register Flow', () => {
  test.beforeEach(async ({ page }) => {
    await page.route('**/api/v1/auth/register', async (route) => {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          org_id: 'mock-org-id',
          user_id: 'mock-user-id',
          access_token: 'mock-token',
          refresh_token: 'mock-refresh'
        }),
      });
    });
  });

  test('should render the register page', async ({ page }) => {
    await page.goto('/register');
    
    await expect(page.locator('h1')).toHaveText('Create your organization');
    
    // Check fields
    await expect(page.locator('input#reg-org')).toBeVisible();
    await expect(page.locator('input#reg-email')).toBeVisible();
    await expect(page.locator('input#reg-password')).toBeVisible();
    await expect(page.locator('input#reg-confirm')).toBeVisible();
  });

  test('should show error when passwords do not match', async ({ page }) => {
    await page.goto('/register');
    
    await page.fill('input#reg-org', 'Acme');
    await page.fill('input#reg-email', 'admin@acme.com');
    await page.fill('input#reg-password', 'securepass123');
    await page.fill('input#reg-confirm', 'differentpass123');
    
    await page.click('button[type="submit"]');
    
    // Check for error text
    await expect(page.getByText('Passwords do not match')).toBeVisible();
  });

  test('should redirect to dashboard on success', async ({ page }) => {
    await page.goto('/register');
    
    await page.fill('input#reg-org', 'Acme');
    await page.fill('input#reg-email', 'admin@acme.com');
    await page.fill('input#reg-password', 'securepass123');
    await page.fill('input#reg-confirm', 'securepass123');
    
    await page.click('button[type="submit"]');
    
    await expect(page).toHaveURL(/\/dashboard/);
  });
});
