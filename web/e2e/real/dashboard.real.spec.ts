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

async function createUserAndToken(request: any) {
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

  const loginRes = await postWithRetry(
    request,
    'http://127.0.0.1:8080/api/v1/auth/login',
    { email: testEmail, password: testPassword }
  );
  expect(loginRes.status()).toBe(200);
  const loginBody = await loginRes.json();

  return loginBody.token as string;
}

test.describe('Dashboard Page (Real BE)', () => {
  test('should load dashboard for an authenticated user', async ({ page, request }) => {
    const token = await createUserAndToken(request);

    await page.goto('/login');
    await page.evaluate((t) => localStorage.setItem('access_token', t), token);

    await page.goto('/dashboard');
    await expect(page.getByText('Overview').first()).toBeVisible();
  });
});
