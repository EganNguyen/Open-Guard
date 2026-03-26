const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

/* ── Types ── */

export interface LoginRequest {
  email: string;
  password: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  plan: string;
  created_at: string;
  updated_at: string;
}

export interface User {
  id: string;
  org_id: string;
  email: string;
  display_name: string;
  status: string;
  mfa_enabled: boolean;
  tier_isolation: string;
  created_at: string;
  updated_at: string;
}

export interface LoginResponse {
  token: string;
  refresh_token: string;
  expires_in: number;
  user: User;
  org: Organization;
}

export interface RegisterRequest {
  org_name: string;
  email: string;
  password: string;
}

export interface RegisterResponse {
  user: User;
  org: Organization;
  token: string;
}

export interface Policy {
  id: string;
  name: string;
  effect: string;
  actions: string[];
  resources: string[];
  subjects: string[];
}

export interface Connector {
  id: string;
  org_id: string;
  name: string;
  webhook_url: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface ListResponse<T> {
  data: T[];
  meta: {
    total_items?: number;
    total_pages?: number;
    page?: number;
    per_page?: number;
    total?: number;
  };
}

export interface ApiError {
  error: {
    code: string;
    message: string;
    request_id: string;
    trace_id: string;
    retryable: boolean;
  };
}

/* ── HTTP helpers ── */

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const url = `${API_BASE}${path}`;
  const res = await fetch(url, {
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
        retryable: false,
      },
    }));
    throw err;
  }

  // Handle No Content
  if (res.status === 204) {
    return {} as T;
  }

  return res.json() as Promise<T>;
}

/* ── Auth API ── */

export async function login(data: LoginRequest): Promise<LoginResponse> {
  return request<LoginResponse>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function register(data: RegisterRequest): Promise<RegisterResponse> {
  return request<RegisterResponse>("/api/v1/auth/register", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function refreshToken(token: string): Promise<LoginResponse> {
  return request<LoginResponse>("/api/v1/auth/refresh", {
    method: "POST",
    body: JSON.stringify({ refresh_token: token }),
  });
}

export async function logout(token: string): Promise<void> {
  await request<void>("/api/v1/auth/logout", {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  });
}

/* ── Management API ── */

export async function getUsers(token: string): Promise<ListResponse<User>> {
  return request<ListResponse<User>>("/api/v1/users", {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function getPolicies(token: string): Promise<ListResponse<Policy>> {
  return request<ListResponse<Policy>>("/api/v1/policies", {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function getThreats(token: string): Promise<ListResponse<any>> {
  return request<ListResponse<any>>("/api/v1/threats", {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function getAuditEvents(token: string): Promise<ListResponse<any>> {
  return request<ListResponse<any>>("/api/v1/audit", {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function getAlerts(token: string): Promise<ListResponse<any>> {
  return request<ListResponse<any>>("/api/v1/alerts", {
    headers: { Authorization: `Bearer ${token}` },
  });
}
/* ── Connector API ── */

export async function getConnectors(token: string): Promise<ListResponse<Connector>> {
  return request<ListResponse<Connector>>("/api/v1/admin/connectors", {
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function createConnector(token: string, data: { name: string; webhook_url: string }): Promise<Connector> {
  return request<Connector>("/api/v1/admin/connectors", {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify(data),
  });
}

export async function updateConnector(
  token: string,
  id: string,
  data: Partial<{ name: string; webhook_url: string; scopes: string[] }>
): Promise<Connector> {
  return request<Connector>(`/api/v1/admin/connectors/${id}`, {
    method: "PATCH",
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify(data),
  });
}

export async function suspendConnector(token: string, id: string): Promise<void> {
  await request<void>(`/api/v1/admin/connectors/${id}/suspend`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function activateConnector(token: string, id: string): Promise<void> {
  await request<void>(`/api/v1/admin/connectors/${id}/activate`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function deleteConnector(token: string, id: string): Promise<void> {
  await request<void>(`/api/v1/admin/connectors/${id}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
}

/* ── Policy CRUD API ── */

export async function createPolicy(
  token: string,
  data: { name: string; description?: string; type: string; rules: object; enabled?: boolean }
): Promise<Policy> {
  return request<Policy>("/api/v1/policies", {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify(data),
  });
}

export async function updatePolicy(token: string, id: string, data: Partial<Policy>): Promise<Policy> {
  return request<Policy>(`/api/v1/policies/${id}`, {
    method: "PUT",
    headers: { Authorization: `Bearer ${token}` },
    body: JSON.stringify(data),
  });
}

export async function deletePolicy(token: string, id: string): Promise<void> {
  await request<void>(`/api/v1/policies/${id}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function getPolicyEvalLogs(token: string): Promise<ListResponse<any>> {
  return request<ListResponse<any>>("/api/v1/policy/eval-logs", {
    headers: { Authorization: `Bearer ${token}` },
  });
}

/* ── Compliance API ── */

export async function getComplianceReports(token: string): Promise<ListResponse<any>> {
  return request<ListResponse<any>>("/api/v1/compliance", {
    headers: { Authorization: `Bearer ${token}` },
  });
}

