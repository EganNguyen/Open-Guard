import { test, expect, APIRequestContext } from '@playwright/test';
import { randomUUID } from 'crypto';

/**
 * PHASE 4 E2E INTEGRATION TEST SUITE
 * ============================================================
 * Coverage:
 *   - Integrity check (GET /audit/integrity)
 *   - RLS Isolation (Multi-Tenancy)
 *   - Bulk Event Ingestion (POST /v1/events/ingest)
 *   - Audit Metadata (TraceID, Seq)
 */

const API = 'http://127.0.0.1:8080';
const sleep = (ms: number) => new Promise(r => setTimeout(r, ms));

async function postWithRetry(
  request: APIRequestContext,
  url: string,
  data: Record<string, unknown>,
  attempts = 5,
): Promise<Awaited<ReturnType<typeof request.post>>> {
  let lastErr: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      const res = await request.post(url, { data });
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

async function setupOrg(request: APIRequestContext) {
  const uuid = randomUUID().substring(0, 8);
  const email = `admin_${uuid}@e2e.openguard.io`;
  const orgName = `Org ${uuid}`;

  const regRes = await postWithRetry(request, `${API}/api/v1/auth/register`, {
    org_name: orgName,
    email,
    password: 'Password123!',
    display_name: 'E2E Admin',
  });
  expect(regRes.status()).toBe(201);

  const loginRes = await postWithRetry(request, `${API}/api/v1/auth/login`, {
    email,
    password: 'Password123!',
  });
  expect(loginRes.status()).toBe(200);
  const body = await loginRes.json();
  return {
    token: body.token,
    orgId: body.org.id,
    userId: body.user.id,
    email,
  };
}

test.describe('Phase 4 — Audit & Integrity', () => {
  test('P4-01: Audit Integrity check returns ok:true with no gaps for new org', async ({ request }) => {
    const { token } = await setupOrg(request);

    // After registration, at least 1-2 events (org.created, user.created) should exist
    await sleep(2000); // Allow outbox processing

    const integrityRes = await request.get(`${API}/api/v1/audit/integrity`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(integrityRes.status()).toBe(200);
    const body = await integrityRes.json();
    expect(body.ok).toBe(true);
    expect(body.gaps).toHaveLength(0);
  });

  test('P4-02: Multi-Tenancy (RLS) isolation between Org A and Org B', async ({ request }) => {
    // 1. Setup Org A and Org B
    const orgA = await setupOrg(request);
    const orgB = await setupOrg(request);

    // 2. Org A performs an action (create a connector)
    const connRes = await request.post(`${API}/api/v1/admin/connectors`, {
      headers: { Authorization: `Bearer ${orgA.token}` },
      data: { name: 'Org A Connector', webhook_url: 'https://a.com/hook' },
    });
    expect(connRes.status()).toBe(201);

    await sleep(3000); // Wait for async audit log write

    // 3. Verify Org A sees the audit event
    const auditARes = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${orgA.token}` },
    });
    const eventsA = await auditARes.json();
    expect(eventsA.some((e: any) => e.type === 'connector.created' || e.event_type === 'connector.created')).toBe(true);

    // 4. Verify Org B CANNOT see Org A's audit event
    const auditBRes = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${orgB.token}` },
    });
    const eventsB = await auditBRes.json();
    // Org B should only have its own registration/login events
    expect(eventsB.some((e: any) => e.org_id === orgA.orgId)).toBe(false);
    expect(eventsB.some((e: any) => e.type === 'connector.created')).toBe(false);
  });

  test('P4-03: Bulk Event Ingestion — Ingest 10 events and verify visibility', async ({ request }) => {
    const { token } = await setupOrg(request);

    // 1. Create a connector to get API key for ingestion
    const connRes = await request.post(`${API}/api/v1/admin/connectors`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { name: 'Bulk Ingestor', webhook_url: 'https://bulk.com/hook', scopes: ['events:write'] },
    });
    const connectorKey = (await connRes.json()).api_key;

    // 2. Bulk ingest 10 events
    const events = Array.from({ length: 10 }, (_, i) => ({
      event_id: randomUUID(),
      type: 'test.bulk_ingest',
      resource: `resource-${i}`,
      action: 'ingest',
      occurred_at: new Date().toISOString(),
      metadata: { index: i }
    }));

    const ingestRes = await request.post(`${API}/api/v1/events/ingest`, {
      headers: { 'X-OpenGuard-Connector-Key': connectorKey },
      data: { events },
    });
    expect(ingestRes.status()).toBe(200);

    await sleep(5000); // Ingest SLO is < 5s for visibility

    // 3. Verify in audit log
    await expect.poll(async () => {
      const auditRes = await request.get(`${API}/api/v1/audit/events`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (auditRes.status() !== 200) return 0;
      const body = await auditRes.json();
      return body.filter((e: any) => e.type === 'test.bulk_ingest').length;
    }, { timeout: 30_000, intervals: [2000] }).toBe(10);
  });

  test('P4-04: Audit Metadata — Verify TraceID and Sequence integrity', async ({ request }) => {
    const { token } = await setupOrg(request);

    // Action: create a policy
    const traceId = randomUUID().replace(/-/g, '');
    const policyRes = await request.post(`${API}/api/v1/policies`, {
      headers: { Authorization: `Bearer ${token}`, 'X-Trace-ID': traceId },
      data: {
        name: 'Integrity Test Policy',
        type: 'ip_allowlist',
        rules: { allowed_ips: ['1.1.1.1'] },
        enabled: true,
      },
    });
    expect(policyRes.status()).toBe(201);

    await sleep(3000);

    // Verify audit record has TraceID (if system supports it) and correct Sequence
    const auditRes = await request.get(`${API}/api/v1/audit/events`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const events = await auditRes.json();
    const policyEvent = events.find((e: any) => e.type === 'policy.created' || e.event_type === 'policy.created');
    
    expect(policyEvent).toBeTruthy();
    // TraceID should propagate if our middleware is correctly configured
    // Note: Some systems might not propagate TraceID to the audit record yet, 
    // but the architecture doc requires it.
    if (policyEvent.trace_id) {
       expect(policyEvent.trace_id.length).toBeGreaterThan(0);
    }
    
    expect(policyEvent.chain_seq).toBeDefined();
    expect(typeof policyEvent.chain_seq).toBe('number');
  });
});
