import { test, expect } from '@playwright/test';

/**
 * REAL E2E INTEGRATION TEST
 * This test interacts with real services (Control Plane, IAM, Policy, DB) running in Docker.
 * It does NOT mock any API calls.
 */
test.describe('Real System Integration - Policy Lifecycle', () => {
  const timestamp = Date.now();
  const testEmail = `tester_${timestamp}@openguard.io`;
  const testOrg = `E2E Org ${timestamp}`;
  const testPassword = 'Password123!';
  
  let authToken: string;

  test.beforeAll(async ({ request }) => {
    // 1. Register a new organization and user
    const registerRes = await request.post('/api/v1/auth/register', {
      data: {
        org_name: testOrg,
        email: testEmail,
        password: testPassword,
        display_name: 'E2E Tester'
      }
    });
    expect(registerRes.status()).toBe(201);

    // 2. Login to get JWT
    const loginRes = await request.post('/api/v1/auth/login', {
      data: {
        email: testEmail,
        password: testPassword
      }
    });
    expect(loginRes.status()).toBe(200);
    const body = await loginRes.json();
    authToken = body.token;
    expect(authToken).toBeDefined();
  });

  test('should execute full policy lifecycle: create -> list -> evaluate -> delete', async ({ request }) => {
    const policyName = `E2E IP Allowlist ${timestamp}`;
    
    // 1. Create Policy (IP Allowlist)
    const createRes = await request.post('/api/v1/policies', {
      headers: { 'Authorization': `Bearer ${authToken}` },
      data: {
        name: policyName,
        type: 'ip_allowlist',
        rules: {
          allowed_ips: ['1.1.1.1']
        },
        enabled: true
      }
    });
    expect(createRes.status()).toBe(201);
    const policy = await createRes.json();
    const policyId = policy.id;

    // 2. List Policies and verify it appears (allow some time for outbox/async propagation if needed)
    await expect.poll(async () => {
      const listRes = await request.get('/api/v1/policies', {
        headers: { 'Authorization': `Bearer ${authToken}` }
      });
      const list = await listRes.json();
      return list.data.some((p: any) => p.id === policyId);
    }, { timeout: 10000 }).toBe(true);

    // 3. Evaluate Policy - Permitted (IP 1.1.1.1)
    const evalPermitRes = await request.post('/api/v1/policies/evaluate', {
      headers: { 'Authorization': `Bearer ${authToken}` },
      data: {
        action: 'data.read',
        resource: 'sensitive-db',
        ip_address: '1.1.1.1'
      }
    });
    expect(evalPermitRes.status()).toBe(200);
    const evalPermit = await evalPermitRes.json();
    expect(evalPermit.permitted).toBe(true);

    // 4. Evaluate Policy - Denied (IP 2.2.2.2)
    const evalDenyRes = await request.post('/api/v1/policies/evaluate', {
      headers: { 'Authorization': `Bearer ${authToken}` },
      data: {
        action: 'data.read',
        resource: 'sensitive-db',
        ip_address: '2.2.2.2'
      }
    });
    // The policy engine returns 403 Forbidden for denials
    expect(evalDenyRes.status()).toBe(403);
    const evalDeny = await evalDenyRes.json();
    expect(evalDeny.permitted).toBe(false);

    // 5. Delete Policy
    const deleteRes = await request.delete(`/api/v1/policies/${policyId}`, {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    expect(deleteRes.status()).toBe(204);

    // 6. Verify policy is gone from list
    const finalItemsRes = await request.get('/api/v1/policies', {
      headers: { 'Authorization': `Bearer ${authToken}` }
    });
    const finalItems = await finalItemsRes.json();
    expect(finalItems.data.some((p: any) => p.id === policyId)).toBe(false);
  });
});
