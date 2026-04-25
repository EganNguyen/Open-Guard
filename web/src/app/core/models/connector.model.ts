export interface Connector {
  id: string;
  org_id?: string;
  name: string;
  description?: string;
  client_secret?: string;
  redirect_uris: string[];
  created_at?: string;
  updated_at?: string;
}

export interface ConnectorRegistrationResult {
  id: string;
  org_id: string;
}
