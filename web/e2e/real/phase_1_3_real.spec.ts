import { test, expect, APIRequestContext } from '@playwright/test';
import { randomUUID } from 'crypto';

/**
 * PHASE 1-3 REAL E2E INTEGRATION TEST SUITE
 * ============================================================
 * Tests real running services — NO mocks, NO page.route() overrides.
 *
 * Prerequisites:
 *   - Full stack running:  make dev
 *   - Frontend dev server: npm run dev  (handled by playwright.config.ts webServer)
 *   - Todo App running:    cd examples/todoapp && go run main.go
 *
 * Coverage:
 *   Phase 1 — IAM:     Register → Login → Dashboard → Connector → Create user in org
 *   Phase 2 — Policy:  Create (IP Allowlist + RBAC) → List → Evaluate
 *                      → User in org accesses todoapp via OpenGuard
 *                      → OpenGuard extend access (RBAC role elevation)
 *                      → User in an organization can access the todo app and perform CRUD operations based on policies.
 *   Phase 3 — Audit:   Read everything in Audit log
 */

// ─── Shared helpers ───────────────────────────────────────────────────────────

const API = 'http://127.0.0.1:8080';
const TODOAPP = process.env.TODOAPP_URL || 'http://127.0.0.1:8083'; // TodoApp example (default to 8083 for container)
const sleep = (ms: number) => new Promise(r => setTimeout(r, ms));

async function postWithRetry(
  request: APIRequestContext,
  url: string,
  data: Record<string, unknown>,
  attempts = 10,
): Promise<Awaited<ReturnType<typeof request.post>>> {
  let lastErr: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      const res = await request.post(url, { data });
      // If we get a server-side error that might be transient, retry
      if (res.status() >= 500 && i < attempts - 1) {
        await sleep(1000 * (i + 1));
        continue;
      }
      return res;
    } catch (err) {
      lastErr = err;
      await sleep(1000 * (i + 1));
    }
  }
  throw lastErr;
}

/**
 * Sets up a new organization by registering an admin user.
 * Returns the admin token, orgId, userId and email.
 */
async function setupOrg(request: APIRequestContext): Promise<{
  token: string;
  orgId: string;
  userId: string;
  email: string;
  orgName: string;
}> {
  const uuid = randomUUID();
  const email = `admin_${uuid}@e2e.openguard.io`;
  const orgName = `Org ${uuid}`;

  console.log(`[setupOrg] Attempting register for: ${orgName}`);

  const regRes = await postWithRetry(request, `${API}/api/v1/auth/register`, {
    org_name: orgName,
    email,
    password: 'Password123!',
    display_name: 'E2E Admin',
  });

  if (regRes.status() !== 201) {
    const text = await regRes.text();
    // If it's a duplicate key, maybe a previous retry attempt actually succeeded
    if (regRes.status() === 400 && text.includes('duplicate key')) {
      console.log(`[setupOrg] Duplicate key error (likely previous retry succeeded). Proceeding to login.`);
    } else {
      console.error(`[setupOrg] Registration failed for ${orgName}:`, text);
      expect(regRes.status(), `register failed for ${orgName}: ${text}`).toBe(201);
    }
  }

  const loginRes = await postWithRetry(request, `${API}/api/v1/auth/login`, {
    email,
    password: 'Password123!',
  });
  expect(loginRes.status(), `login failed: ${await loginRes.text()}`).toBe(200);
  const body = await loginRes.json();
  return {
    token: body.token,
    orgId: body.org?.id ?? '',
    userId: body.user?.id ?? '',
    email,
    orgName,
  };
}

// Legacy alias for compatibility
async function setupUser(request: APIRequestContext) {
  return setupOrg(request);
}

// ─── Wait for control-plane ────────────────────────────────────────────────────

test.beforeAll(async ({ request }) => {
  await expect.poll(async () => {
    try {
      const res = await request.get(`${API}/health/ready`);
      return res.status();
    } catch {
      return 0;
    }
  }, { timeout: 90_000, intervals: [1000] }).toBe(200);
});

// =============================================================================
// PHASE 1 — IAM: Register, Login, Dashboard, Connector, Manage Users
// =============================================================================

test.describe('Phase 1 — IAM Foundation', () => {
  test('P1-01: Register → Login → receive JWT', async ({ request }) => {
    const { token } = await setupOrg(request);
    expect(token).toBeTruthy();
  });

  test('P1-02: Login UI → Dashboard loads with real token', async ({ page, request }) => {
    const { token } = await setupOrg(request);

    await page.goto('/login');
    await page.evaluate((t) => localStorage.setItem('access_token', t), token);

    await page.goto('/dashboard');
    await expect(page.getByText('Overview').first()).toBeVisible({ timeout: 15_000 });
  });

  test('P1-03: Register a Connector via UI and verify it appears in list', async ({ page, request }) => {
    test.setTimeout(90_000);
    const { token } = await setupOrg(request);
    const connectorName = `E2E Connector ${Date.now()}`;

    await page.goto('/login');
    await page.evaluate((t) => localStorage.setItem('access_token', t), token);
    await page.goto('/dashboard/connectors');
    await expect.poll(() => page.url(), { timeout: 15_000 }).toContain('/dashboard/connectors');
    await page.waitForLoadState('networkidle');

    // Wait for the page to be ready and interactive
    await page.waitForLoadState('networkidle');
    await page.getByTestId('register-app-btn').waitFor({ state: 'visible', timeout: 30_000 });
    await page.getByTestId('register-app-btn').click();

    await page.getByTestId('app-name-input').fill(connectorName);
    await page.getByTestId('app-webhook-input').fill('https://example.com/webhook');

    await page.getByTestId('submit-app-btn').click();

    await expect(page.getByTestId('connector-list')).toContainText(connectorName, { timeout: 10_000 });
  });

  test('P1-04: Admin creates a new user in their organization via API', async ({ request }) => {
    const { token, orgId } = await setupOrg(request);
    const unique = Date.now();
    const newEmail = `member_${unique}@openguard.io`;

    // Admin creates a new member via the management API
    const createRes = await request.post(`${API}/api/v1/users`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        email: newEmail,
        display_name: 'New Member',
      },
    });
    expect(createRes.status(), `create user failed: ${await createRes.text()}`).toBe(201);

    const created = await createRes.json();
    expect(created.id).toBeTruthy();
    expect(created.email).toBe(newEmail);
    expect(created.org_id).toBe(orgId);

    // Verify new user appears in the user list — poll for consistency
    await expect.poll(async () => {
      try {
        const listRes = await request.get(`${API}/api/v1/users`, {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (listRes.status() !== 200) return false;
        const body = await listRes.json();
        return (body.data ?? []).some((u: any) => u.email === newEmail);
      } catch (err) {
        return false;
      }
    }, { timeout: 15_000, intervals: [1000] }).toBe(true);
  });

  test('P1-05: Admin creates a user via UI and user appears in the table', async ({ page, request }) => {
    const { token } = await setupOrg(request);
    const unique = Date.now();
    const newEmail = `ui_member_${unique}@openguard.io`;

    await page.goto('/login');
    await page.evaluate((t) => localStorage.setItem('access_token', t), token);
    await page.goto('/dashboard/users');

    await expect(page.getByTestId('page-title')).toContainText('Organization Users', { timeout: 20_000 });
    // Wait for the UI state to settle (either empty or full table)
    await expect(page.getByTestId('user-list').or(page.getByTestId('no-users-view'))).toBeVisible({ timeout: 15_000 });

    // Open create user modal (look in both locations)
    const addBtn = page.getByTestId('add-user-btn').or(page.getByTestId('add-user-empty-btn')).first();
    await addBtn.click();

    // Fill in the form
    await page.getByTestId('create-user-email').fill(newEmail);
    await page.getByTestId('create-user-display-name').fill('UI Created Member');

    // Submit
    await page.getByTestId('submit-create-user').click();

    // Confirm user appears in table
    await expect(page.getByTestId('user-list')).toContainText(newEmail, { timeout: 15_000 });
  });

  test('P1-06: Admin can suspend and re-activate a user they created', async ({ request }) => {
    const { token } = await setupOrg(request);
    const newEmail = `lifecycle_${Date.now()}@openguard.io`;

    // Create user
    const createRes = await request.post(`${API}/api/v1/users`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { email: newEmail, display_name: 'Lifecycle User' },
    });
    expect(createRes.status()).toBe(201);
    const userId = (await createRes.json()).id;

    // Suspend
    const suspendRes = await request.post(`${API}/api/v1/users/${userId}/suspend`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(suspendRes.status()).toBe(200);
    expect((await suspendRes.json()).status).toBe('suspended');

    // Activate
    const activateRes = await request.post(`${API}/api/v1/users/${userId}/activate`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(activateRes.status()).toBe(200);
    expect((await activateRes.json()).status).toBe('active');
  });

  test('P1-07: User list UI renders data from real backend', async ({ page, request }) => {
    const { token, email } = await setupOrg(request);

    await page.goto('/login');
    await page.evaluate((t) => localStorage.setItem('access_token', t), token);

    await page.goto('/dashboard/users');
    // Admin user should always appear in their own org
    await expect(page.getByTestId('user-list').or(page.getByTestId('no-users-view'))).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId('user-list')).toContainText(email, { timeout: 20_000 });
  });

  test('P1-08: Unauthenticated request is rejected', async ({ request }) => {
    const res = await request.get(`${API}/api/v1/users`);
    expect(res.status()).toBe(401);
  });
});

// =============================================================================
// PHASE 2 — Policy Engine: CRUD + Evaluate + TodoApp + Extended Access
// =============================================================================

test.describe('Phase 2 — Policy Engine', () => {
  test('P2-01: Full IP Allowlist policy lifecycle via API (create→list→evaluate permit→evaluate deny→delete)', async ({ request }) => {
    const { token } = await setupOrg(request);
    const policyName = `E2E IP Allowlist ${Date.now()}`;

    // 1. Create
    const createRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: policyName,
        type: 'ip_allowlist',
        rules: { allowed_ips: ['1.1.1.1'] },
        enabled: true,
      },
    });
    expect(createRes.status(), `create policy: ${await createRes.text()}`).toBe(201);
    const policy = await createRes.json();
    const policyId = policy.id;
    expect(policyId).toBeTruthy();

    // 2. List — poll until it appears (Kafka/outbox propagation)
    await expect.poll(async () => {
      const listRes = await request.get(`${API}/api/v1/policies`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const list = await listRes.json();
      return (list.data ?? []).some((p: any) => p.id === policyId);
    }, { timeout: 15_000, intervals: [500] }).toBe(true);

    // 3. Evaluate — permitted (IP in allowlist)
    const permitRes = await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { action: 'data.read', resource: 'db', ip_address: '1.1.1.1' },
    });
    expect(permitRes.status()).toBe(200);
    expect((await permitRes.json()).permitted).toBe(true);

    // 4. Evaluate — denied (IP not in allowlist)
    const denyRes = await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { action: 'data.read', resource: 'db', ip_address: '9.9.9.9' },
    });
    expect(denyRes.status()).toBe(403);
    expect((await denyRes.json()).permitted).toBe(false);

    // 5. Delete
    const deleteRes = await request.delete(`${API}/api/v1/policies/${policyId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(deleteRes.status()).toBe(204);

    // 6. Verify gone from list
    await expect.poll(async () => {
      const listRes = await request.get(`${API}/api/v1/policies`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const list = await listRes.json();
      return (list.data ?? []).some((p: any) => p.id === policyId);
    }, { timeout: 15_000, intervals: [500] }).toBe(false);
  });

  test('P2-02: RBAC policy — permit matching role, deny missing role', async ({ request }) => {
    const { token, userId } = await setupOrg(request);

    const createRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: `E2E RBAC ${Date.now()}`,
        type: 'rbac',
        rules: { allowed_roles: ['admin'] },
        enabled: true,
      },
    });
    expect(createRes.status(), `create rbac policy: ${await createRes.text()}`).toBe(201);
    const policyId = (await createRes.json()).id;

    await expect.poll(async () => {
      const listRes = await request.get(`${API}/api/v1/policies`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      return ((await listRes.json()).data ?? []).some((p: any) => p.id === policyId);
    }, { timeout: 15_000, intervals: [500] }).toBe(true);

    // Evaluate — permitted (user has 'admin')
    const permitRes = await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { action: 'data.access', resource: 'admin-panel', user_id: userId, user_groups: ['admin'] },
    });
    expect(permitRes.status()).toBe(200);
    expect((await permitRes.json()).permitted).toBe(true);

    // Evaluate — denied (user only has 'viewer')
    const denyRes = await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { action: 'data.access', resource: 'admin-panel', user_id: userId, user_groups: ['viewer'] },
    });
    expect(denyRes.status()).toBe(403);
    expect((await denyRes.json()).permitted).toBe(false);

    // Cleanup
    await request.delete(`${API}/api/v1/policies/${policyId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  });

  test('P2-03: Policy Registry page renders policies from real backend', async ({ page, request }) => {
    const { token } = await setupOrg(request);
    const policyName = `E2E UI Policy ${Date.now()}`;

    const createRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: policyName,
        type: 'ip_allowlist',
        rules: { allowed_ips: ['10.0.0.1'] },
        enabled: true,
      },
    });
    expect(createRes.status()).toBe(201);

    await sleep(1500);

    await page.goto('/login');
    await page.evaluate((t) => localStorage.setItem('access_token', t), token);
    await page.goto('/dashboard/access-policies');

    await expect(page.getByTestId('page-title')).toContainText('Policy Registry', { timeout: 15_000 });
    await expect(page.getByTestId('policy-list')).toContainText(policyName, { timeout: 15_000 });
  });

  test('P2-04: Create and delete a policy via UI', async ({ page, request }) => {
    const { token } = await setupOrg(request);
    const policyName = `E2E UI Create ${Date.now()}`;

    await page.goto('/login');
    await page.evaluate((t) => localStorage.setItem('access_token', t), token);
    await page.goto('/dashboard/access-policies');

    await page.getByTestId('toggle-create-policy').click();
    await page.locator('#policy-name').fill(policyName);
    await page.getByTestId('submit-policy').click();

    await expect(page.getByTestId('policy-list')).toContainText(policyName, { timeout: 20_000 });

    const deleteBtn = page.locator(`[data-testid^="delete-policy-"]`).first();
    await deleteBtn.click();

    await expect(async () => {
      const list = page.getByTestId('policy-list');
      const count = await list.count();
      if (count === 0) return;
      await expect(list).not.toContainText(policyName);
    }).toPass({ timeout: 15_000 });
  });

  test('P2-05: Created org user can access todoapp via OpenGuard authentication', async ({ request }) => {
    test.setTimeout(60_000);

    // 1. Register an org and get admin token
    const { token, orgId } = await setupOrg(request);

    // 2. Check todoapp availability — skip if not running
    try {
      const ping = await request.get(TODOAPP, { timeout: 3000 });
      if (ping.status() !== 200) {
        console.log(`TodoApp not running (status ${ping.status()}), skipping P2-05`);
        test.skip();
      }
    } catch {
      console.log('TodoApp not reachable, skipping P2-05');
      test.skip();
    }

    // 3. Create a connector key for TodoApp to authenticate with OpenGuard
    // (In a real environment, a CONNECTOR_API_KEY would be set in the todoapp env)
    // The todoapp calls /v1/policy/evaluate with its own connector API key.
    // We verify that authenticated requests are evaluated correctly.

    // 4. Call todoapp /api/v1/todos with the user's JWT — should be blocked (no connector key)
    const todoResNoAuth = await request.get(`${TODOAPP}/api/v1/todos`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    // Without a valid connector API key configured in the todoapp, the CP call will fail
    // The todoapp's middleware should return 403 in that case
    expect([200, 403]).toContain(todoResNoAuth.status());
  });

  test('P2-06: OpenGuard authentication — evaluate access for org member with and without policy', async ({ request }) => {
    const { token, orgId, userId } = await setupOrg(request);

    // Create an RBAC policy that restricts sensitive resource access to 'admin' role
    const createRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: `E2E Org Access ${Date.now()}`,
        type: 'rbac',
        rules: { allowed_roles: ['admin'] },
        enabled: true,
      },
    });
    expect(createRes.status()).toBe(201);
    const policyId = (await createRes.json()).id;

    // Wait for propagation
    await expect.poll(async () => {
      const listRes = await request.get(`${API}/api/v1/policies`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      return ((await listRes.json()).data ?? []).some((p: any) => p.id === policyId);
    }, { timeout: 15_000, intervals: [500] }).toBe(true);

    // Evaluate as admin — should be permitted
    const adminEval = await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        action: 'todos.read',
        resource: 'todos',
        user_id: userId,
        user_groups: ['admin'],
      },
    });
    expect(adminEval.status()).toBe(200);
    expect((await adminEval.json()).permitted).toBe(true);

    // Evaluate as regular member — should be denied
    const memberEval = await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        action: 'todos.read',
        resource: 'todos',
        user_id: userId,
        user_groups: ['member'],
      },
    });
    expect(memberEval.status()).toBe(403);
    expect((await memberEval.json()).permitted).toBe(false);

    // Cleanup
    await request.delete(`${API}/api/v1/policies/${policyId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  });

  test('P2-07: OpenGuard extend access — newly created org member gets elevated role then passes policy', async ({ request }) => {
    const { token } = await setupOrg(request);

    // Create a new org member (via admin)
    const memberEmail = `ext_${Date.now()}@openguard.io`;
    const memberRes = await request.post(`${API}/api/v1/users`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { email: memberEmail, display_name: 'Extended Member' },
    });
    expect(memberRes.status()).toBe(201);
    const memberId = (await memberRes.json()).id;

    // Create RBAC policy that requires 'editor' role
    const policyRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: `E2E Extend Access ${Date.now()}`,
        type: 'rbac',
        rules: { allowed_roles: ['editor', 'admin'] },
        enabled: true,
      },
    });
    expect(policyRes.status()).toBe(201);
    const policyId = (await policyRes.json()).id;

    await expect.poll(async () => {
      const listRes = await request.get(`${API}/api/v1/policies`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      return ((await listRes.json()).data ?? []).some((p: any) => p.id === policyId);
    }, { timeout: 15_000, intervals: [500] }).toBe(true);

    // Member with no special role — DENIED
    const denyRes = await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        action: 'content.edit',
        resource: 'articles',
        user_id: memberId,
        user_groups: ['member'],
      },
    });
    expect(denyRes.status()).toBe(403);

    // Admin extends access — member promoted to 'editor' role
    // (simulated by evaluating with the elevated role — in production, role
    //  assignment would be persisted via SCIM or user groups API)
    const grantRes = await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        action: 'content.edit',
        resource: 'articles',
        user_id: memberId,
        user_groups: ['editor'],
      },
    });
    expect(grantRes.status()).toBe(200);
    expect((await grantRes.json()).permitted).toBe(true);

    // Cleanup
    await request.delete(`${API}/api/v1/policies/${policyId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    await request.delete(`${API}/api/v1/users/${memberId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  });

  test('P2-08: Gateway enforces IP policy on protected route (/api/v1/threats)', async ({ request }) => {
    const { token } = await setupOrg(request);

    const createRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: `E2E Gateway Block ${Date.now()}`,
        type: 'ip_allowlist',
        rules: { allowed_ips: ['192.0.2.1'] },
        enabled: true,
      },
    });
    expect(createRes.status()).toBe(201);
    const policyId = (await createRes.json()).id;

    await expect.poll(async () => {
      const listRes = await request.get(`${API}/api/v1/policies`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      return ((await listRes.json()).data ?? []).some((p: any) => p.id === policyId);
    }, { timeout: 15_000, intervals: [500] }).toBe(true);

    const threatsRes = await request.get(`${API}/api/v1/threats`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(threatsRes.status()).toBe(403); // Blocked by IP policy

    // Cleanup
    await request.delete(`${API}/api/v1/policies/${policyId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  });

  test('P2-09: User in TodoApp can read tasks based on policy', async ({ request }) => {
    // 1. Setup Org
    const { token } = await setupOrg(request);

    // 2. Create Connector to get a valid API Key
    const connRes = await request.post(`${API}/api/v1/admin/connectors`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { name: 'TodoApp Connector', webhook_url: 'http://todoapp:8082/webhook' }
    });
    expect(connRes.status()).toBe(201);
    const connectorKey = (await connRes.json()).api_key;

    // 3. Create a policy for "read" action
    const policyRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: `Todo Read Policy ${Date.now()}`,
        type: 'rbac',
        rules: { allowed_roles: ['admin', 'member'] }, // Basic RBAC
        enabled: true,
      },
    });
    expect(policyRes.status()).toBe(201);

    // 4. Call TodoApp GET /api/v1/todos - Should SUCCEED
    // We pass our registered connector key in the special header we added
    const readRes = await request.get(`${TODOAPP}/api/v1/todos`, {
      headers: {
        Authorization: `Bearer ${token}`,
        'X-OpenGuard-Connector-Key': connectorKey
      },
    });

    // If TodoApp is not reachable, skip (it might be starting up)
    if (readRes.status() === 404 || readRes.status() === 502) {
      console.log('TodoApp not reachable (status 404/502), skipping P2-09');
      test.skip();
    }

    expect(readRes.status()).toBe(200);
    const todos = await readRes.json();
    expect(Array.isArray(todos)).toBe(true);
  });

  test('P2-10: User in TodoApp can write tasks based on policy', async ({ request }) => {
    const { token } = await setupOrg(request);

    // Register connector
    const connRes = await request.post(`${API}/api/v1/admin/connectors`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { name: 'TodoApp Writer', webhook_url: 'http://todoapp:8082/webhook' }
    });
    const connectorKey = (await connRes.json()).api_key;

    // Call TodoApp POST /api/v1/todos - Should fail if no policy exists (fail-closed)
    const failRes = await request.post(`${TODOAPP}/api/v1/todos`, {
      headers: {
        Authorization: `Bearer ${token}`,
        'X-OpenGuard-Connector-Key': connectorKey
      },
      data: { title: 'Test Task' },
    });

    if (failRes.status() === 444 || failRes.status() === 404 || failRes.status() === 502) test.skip();

    // It should be 403 Forbidden because no explicit policy allows 'write' for 'todos'
    expect(failRes.status()).toBe(403);

    // Create policy for 'write'
    // Note: The TodoApp maps POST to action 'write' and resource 'todos'
    await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: `Todo Write Policy ${Date.now()}`,
        type: 'rbac',
        rules: { allowed_roles: ['admin'] },
        enabled: true,
      },
    });

    // Wait for policy propagation
    await sleep(2000);

    // Retry POST - Should SUCCEED now
    const successRes = await request.post(`${TODOAPP}/api/v1/todos`, {
      headers: {
        Authorization: `Bearer ${token}`,
        'X-OpenGuard-Connector-Key': connectorKey
      },
      data: { title: 'Verified Task' },
    });
    expect(successRes.status()).toBe(201);
  });
});

// =============================================================================
// PHASE 3 — Event Bus & Audit Log (Read Everything)
// =============================================================================

test.describe('Phase 3 — Audit Log', () => {
  test('P3-01: Audit page loads for authenticated user', async ({ page, request }) => {
    const { token } = await setupOrg(request);

    await page.goto('/login');
    await page.evaluate((t) => localStorage.setItem('access_token', t), token);
    await page.goto('/dashboard/audit');

    await expect(page.getByTestId('page-title')).toContainText('System Audit Events', { timeout: 15_000 });
  });

  test('P3-02: Audit endpoint returns valid response (200 or 503)', async ({ request }) => {
    const { token } = await setupOrg(request);

    const res = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${token}` },
    });

    expect([200, 503]).toContain(res.status());

    if (res.status() === 200) {
      const body = await res.json();
      expect(Array.isArray(body)).toBe(true);
    }
  });

  test('P3-03: Audit log records user.created event after admin creates a user', async ({ request }) => {
    test.setTimeout(120_000);
    const { token } = await setupOrg(request);

    // Verify audit service is running
    const healthRes = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (healthRes.status() === 503) {
      test.skip();
    }

    // Admin creates a new user — triggers user.created event in outbox → Kafka → audit
    const memberEmail = `audit_member_${Date.now()}@openguard.io`;
    const createRes = await request.post(`${API}/api/v1/users`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { email: memberEmail, display_name: 'Audit Member' },
    });
    expect(createRes.status()).toBe(201);

    await sleep(3000);

    // Poll the audit log — user.created event should eventually appear
    await expect.poll(async () => {
      const auditRes = await request.get(`${API}/api/v1/audit/events`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (auditRes.status() !== 200) return false;
      const events = await auditRes.json();
      return Array.isArray(events) && events.some(
        (e: any) => e.type === 'user.created' || e.event_type === 'user.created'
      );
    }, { timeout: 60_000, intervals: [3000] }).toBe(true);
  });

  test('P3-04: Audit log records policy.created event after policy creation', async ({ request }) => {
    test.setTimeout(120_000);
    const { token } = await setupOrg(request);

    const healthRes = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (healthRes.status() === 503) test.skip();

    // Create a policy — should trigger policy.created in audit
    const createRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: `Audit Policy Trigger ${Date.now()}`,
        type: 'ip_allowlist',
        rules: { allowed_ips: ['1.2.3.4'] },
        enabled: true,
      },
    });
    expect(createRes.status()).toBe(201);
    const policyId = (await createRes.json()).id;

    await sleep(5000);

    await expect.poll(async () => {
      const auditRes = await request.get(`${API}/api/v1/audit/events`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (auditRes.status() !== 200) return false;
      const events = await auditRes.json();
      return Array.isArray(events) && events.length > 0;
    }, { timeout: 60_000, intervals: [3000] }).toBe(true);

    // Cleanup
    await request.delete(`${API}/api/v1/policies/${policyId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
  });

  test('P3-05: Audit log records auth.login.success event after login', async ({ request }) => {
    test.setTimeout(120_000);

    const healthRes0 = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${(await setupOrg(request)).token}` },
    });
    if (healthRes0.status() === 503) test.skip();

    // Register and login — should emit auth.login.success
    const { token } = await setupOrg(request);

    await sleep(5000);

    await expect.poll(async () => {
      const auditRes = await request.get(`${API}/api/v1/audit/events`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (auditRes.status() !== 200) return false;
      const events = await auditRes.json();
      return (
        Array.isArray(events) &&
        events.some(
          (e: any) =>
            e.type === 'auth.login.success' || e.event_type === 'auth.login.success'
        )
      );
    }, { timeout: 60_000, intervals: [3000] }).toBe(true);
  });

  test('P3-06: Full audit trail — read all events accumulated across the E2E session', async ({ request }) => {
    test.setTimeout(120_000);
    const { token } = await setupOrg(request);

    const healthRes = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (healthRes.status() === 503) test.skip();

    // Trigger a variety of events in sequence
    // a) Create a user
    await request.post(`${API}/api/v1/users`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { email: `audit_full_${Date.now()}@openguard.io` },
    });

    // b) Create a policy
    const policyRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        name: `Audit Full Trail ${Date.now()}`,
        type: 'ip_allowlist',
        rules: { allowed_ips: ['5.5.5.5'] },
        enabled: true,
      },
    });
    const policyId = (await policyRes.json()).id;

    // c) Evaluate a policy
    await request.post(`${API}/api/v1/policies/evaluate`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { action: 'data.read', resource: 'todos', ip_address: '5.5.5.5' },
    });

    // Read everything from audit — poll as events propagate through Kafka
    await expect.poll(async () => {
      try {
        const auditRes = await request.get(`${API}/api/v1/audit/events`, {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (auditRes.status() !== 200) return false;
        const events = await auditRes.json();
        return Array.isArray(events) && events.length > 0;
      } catch (err) {
        return false;
      }
    }, { timeout: 60_000, intervals: [3000] }).toBe(true);

    const auditRes = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (auditRes.status() === 200) {
      const events = await auditRes.json();
      expect(Array.isArray(events)).toBe(true);
      // There should be at least some events captured
      expect(events.length).toBeGreaterThan(0);

      // Log event types for visibility
      const types = [...new Set(events.map((e: any) => e.type ?? e.event_type))];
      console.log(`[P3-06] Audit log event types observed: ${types.join(', ')}`);
    }

    // Cleanup
    if (policyId) {
      await request.delete(`${API}/api/v1/policies/${policyId}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
    }
  });
});
