const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

/* ── Types ── */

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  token_type: string;
}

export interface RegisterRequest {
  org_name: string;
  email: string;
  password: string;
}

export interface RegisterResponse {
  org_id: string;
  user_id: string;
  access_token: string;
  refresh_token: string;
}


export interface User {
  id: string;
  email: string;
  name: string;
  org_id: string;
  status: string;
  created_at: string;
}

export interface Policy {
  id: string;
  name: string;
  effect: string;
  actions: string[];
  resources: string[];
  subjects: string[];
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
