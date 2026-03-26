import { test, expect } from '@playwright/test';

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

const postWithRetry = async (
  request: any,
  url: string,
  data: Record<string, unknown>,
  attempts = 5
) => {
  let lastErr: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      return await request.post(url, { data });
    } catch (err) {
      lastErr = err;
      await sleep(500 * (i + 1));
    }
  }
  throw lastErr;
};

async function createUser(request: any) {
  const unique = `${Date.now()}-${Math.floor(Math.random() * 1e9)}`;
  const testEmail = `tester_${unique}@openguard.io`;
  const testOrg = `E2E Org ${unique}`;
  const testPassword = 'Password123!';

  const registerRes = await postWithRetry(
    request,
    'http://127.0.0.1:8080/api/v1/auth/register',
    {
      org_name: testOrg,
      email: testEmail,
      password: testPassword,
      display_name: 'E2E Tester',
    }
  );
  expect(registerRes.status()).toBe(201);

  return { testEmail, testPassword };
}

test.describe('Login Flow (Real BE)', () => {
  test('should login via UI and reach dashboard', async ({ page, request }) => {
    const { testEmail, testPassword } = await createUser(request);

    await page.goto('/login');

    await page.fill('input#login-email', testEmail);
    await page.fill('input#login-password', testPassword);
    const [loginResp] = await Promise.all([
      page.waitForResponse((resp) => {
        return resp.url().includes('/api/v1/auth/login') && resp.request().method() === 'POST';
      }),
      page.click('button[type="submit"]'),
    ]);
    if (loginResp.status() !== 200) {
      const body = await loginResp.text();
      throw new Error(`login failed: status=${loginResp.status()} body=${body}`);
    }

    await expect(page).toHaveURL(/\/dashboard/, { timeout: 10_000 });
  });
});
