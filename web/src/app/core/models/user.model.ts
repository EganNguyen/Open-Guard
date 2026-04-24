export interface User {
  id: string;
  org_id: string;
  email: string;
  display_name: string;
  role: 'admin' | 'user' | 'owner';
  status: 'active' | 'suspended' | 'deprovisioned';
  mfa_enabled: boolean;
  scim_external_id?: string;
  created_at: string;
  updated_at: string;
}

export interface AuthResponse {
  user: User;
  access_token: string;
  mfa_required?: boolean;
  mfa_challenge?: string;
}

export interface LoginCredentials {
  email: string;
  password?: string;
}
