/* ── Types ── */

export interface Organization {
  id: string;
  name: string;
  slug: string;
  plan: string;
  created_at: string;
}

export interface User {
  id: string;
  org_id: string;
  email: string;
  display_name: string;
  status: string;
  mfa_enabled: boolean;
  created_at: string;
}

export interface Policy {
  id: string;
  org_id: string;
  name: string;
  type: string;
  rules: any;
  enabled: boolean;
  created_at: string;
}

export interface Connector {
  id: string;
  org_id: string;
  name: string;
  webhook_url: string;
  status: string;
  created_at: string;
}

export interface AuditEvent {
  id: string;
  org_id: string;
  actor_id: string;
  actor_type: string;
  actor_email?: string;
  type: string;
  status: "success" | "failure";
  description?: string;
  occurred_at: string;
  metadata?: Record<string, any>;
  request_id?: string;
  trace_id?: string;
}

export interface ListResponse<T> {
  data: T[];
  meta: {
    total?: number;
    next_cursor?: string;
    has_more?: boolean;
  };
}

export interface ApiError {
  error: {
    code: string;
    message: string;
    request_id: string;
    trace_id: string;
  };
}

/* ── HTTP helpers (BFF Proxy based) ── */

async function apiRequest<T>(path: string, options: RequestInit = {}): Promise<T> {
  // All requests go through the BFF proxy: /api/proxy/v1/...
  const res = await fetch(`/api/proxy/${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
  });

  if (!res.ok) {
    const err: ApiError = await res.json().catch(() => ({
      error: {
        code: "UNKNOWN",
        message: res.statusText,
        request_id: "",
        trace_id: "",
      },
    }));
    throw err;
  }

  if (res.status === 204) return {} as T;

  return res.json();
}

/* ── Audit API ── */

export async function getAuditEvents(params?: {
  cursor?: string;
  limit?: number;
  event_type?: string;
  actor_id?: string;
  start_at?: string;
  end_at?: string;
}): Promise<ListResponse<AuditEvent>> {
  const searchParams = new URLSearchParams();
  if (params?.cursor) searchParams.set("cursor", params.cursor);
  if (params?.limit) searchParams.set("limit", params.limit.toString());
  if (params?.event_type) searchParams.set("event_type", params.event_type);
  if (params?.actor_id) searchParams.set("actor_id", params.actor_id);
  if (params?.start_at) searchParams.set("start_at", params.start_at);
  if (params?.end_at) searchParams.set("end_at", params.end_at);

  const query = searchParams.toString();
  return apiRequest<ListResponse<AuditEvent>>(`audit/events${query ? `?${query}` : ""}`);
}

export async function getAuditIntegrity(): Promise<{ ok: boolean; gaps?: any[] }> {
  return apiRequest("audit/integrity");
}

export async function triggerAuditExport(format: "csv" | "json"): Promise<{ job_id: string }> {
  return apiRequest("audit/export", {
    method: "POST",
    body: JSON.stringify({ format }),
  });
}

/* ── Connectors API ── */

export async function getConnectors(): Promise<ListResponse<Connector>> {
  return apiRequest("admin/connectors");
}

export async function createConnector(data: { name: string; webhook_url: string }): Promise<Connector> {
  return apiRequest("admin/connectors", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

/* ── Policies API ── */

export async function getPolicies(): Promise<ListResponse<Policy>> {
  return apiRequest("policies");
}

/* ── Users API ── */

export async function getUsers(): Promise<ListResponse<User>> {
  return apiRequest("users");
}

export async function createUser(data: { email: string; display_name?: string }): Promise<User> {
  return apiRequest("users", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function suspendUser(id: string): Promise<User> {
  return apiRequest(`users/${id}/suspend`, { method: "POST" });
}

export async function activateUser(id: string): Promise<User> {
  return apiRequest(`users/${id}/activate`, { method: "POST" });
}

export async function deleteUser(id: string): Promise<void> {
  return apiRequest(`users/${id}`, { method: "DELETE" });
}

/* ── Policy Actions ── */

export async function createPolicy(data: { name: string; type: string; rules: any; enabled: boolean }): Promise<Policy> {
  return apiRequest("policies", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function deletePolicy(id: string): Promise<void> {
  return apiRequest(`policies/${id}`, { method: "DELETE" });
}
