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
