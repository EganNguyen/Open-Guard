import { test, expect } from '@playwright/test';

test.describe('Dashboard Page', () => {
  test('should display main elements', async ({ page }) => {
    // Navigate straight to dashboard
    await page.goto('/dashboard');
    
    // Check Topbar (use first to avoid matching the Sidebar nav link)
    await expect(page.getByText('Overview').first()).toBeVisible();
    
    // Check Hero Card
    await expect(page.getByRole('heading', { name: 'Verify your domain' })).toBeVisible();
    await expect(page.getByText('2 of 8 complete')).toBeVisible();
    
    // Check Stat Cards
    await expect(page.getByText('Active alerts').first()).toBeVisible();
    await expect(page.getByText('Managed users')).toBeVisible();
    await expect(page.getByText('Audit events (24h)')).toBeVisible();
    
    // Check Panels
    await expect(page.getByText('Security recommendations')).toBeVisible();
    await expect(page.getByText('Service SLOs')).toBeVisible();
    await expect(page.getByText('Recent audit events')).toBeVisible();
  });

  test('should interact with recommendations', async ({ page }) => {
    await page.goto('/dashboard');
    
    // Find the domain recommendation text and click it directly
    await page.getByText('Add another admin').click();
    
    // Check if it got marked done
    await expect(page.getByText('✓ Done')).toBeVisible();
  });
});
