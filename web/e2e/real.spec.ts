import { test, expect } from '@playwright/test';

/**
 * REAL E2E INTEGRATION TEST
 * This test interacts with real services (Control Plane, IAM, DB) running in Docker.
 * It does NOT mock any API calls.
 */
test.describe('Real System Integration', () => {
  const timestamp = Date.now();
  const testEmail = `admin_${timestamp}@openguard.io`;
  const testOrg = `Test Org ${timestamp}`;
  const testPassword = 'Password123!';

  test('should register, login, and show data from real DB', async ({ page }) => {
    // 1. Visit Registration
    await page.goto('/register');
    
    // 2. Perform Real Registration
    await page.fill('#reg-org', testOrg);
    await page.fill('#reg-email', testEmail);
    await page.fill('#reg-password', testPassword);
    await page.fill('#reg-confirm', testPassword);
    await page.click('button[type="submit"]');

    // 3. Verify real redirect to Dashboard
    // This confirms that:
    // - Control Plane proxied to IAM
    // - IAM created Org & User in PostgreSQL
    // - IAM generated a real JWT
    // - Frontend stored the real token
    await expect(page).toHaveURL(/\/dashboard/);
    await expect(page.getByText(testOrg)).toBeVisible();

    // 4. Verify Data Hydration
    // "Managed users" should at least include the user we just created.
    // We use a small delay or retry to wait for DB consistency if needed, 
    // but Playwright expect() handles retries.
    const userCount = page.getByTestId('user-count');
    await expect(async () => {
      const text = await userCount.innerText();
      expect(parseInt(text)).toBeGreaterThan(0);
    }).toPass();

    // 5. Navigate to Users list and verify our user exists in the Real DB
    // (Note: Currently the users page is a placeholder, but we can verify 
    // the sidebar/navigation still works with real auth state).
    await page.goto('/dashboard/users');
    await expect(page.getByTestId('page-title')).toContainText('Organization Users');
    
    // 6. Verify token is valid and persists
    const token = await page.evaluate(() => localStorage.getItem('access_token'));
    expect(token).toBeTruthy();
    expect(token?.split('.').length).toBe(3); // Basic JWT format check
  });
});
