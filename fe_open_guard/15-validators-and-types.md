# §15 — Zod Validators & TypeScript Types

All domain types and form validators live in `lib/validators/` and `types/`. They are the frontend's single source of truth for data shapes — derived from the BE's `shared/models` package (BE spec §4).

---

## 15.1 Domain Model Types

```ts
// types/models.ts
// These types mirror the Go structs in shared/models/*.go (BE spec §4).
// When the BE shared module has a breaking change (major version bump),
// these types MUST be updated in the same PR.

export type UserStatus =
  | 'active'
  | 'suspended'
  | 'deprovisioned'
  | 'provisioning_failed'

export type ProvisioningStatus =
  | 'complete'
  | 'initializing'
  | 'provisioning_failed'

export interface User {
  id:                  string
  org_id:              string
  email:               string
  display_name:        string
  status:              UserStatus
  mfa_enabled:         boolean
  mfa_method:          'totp' | 'webauthn' | null
  scim_external_id:    string | null
  provisioning_status: ProvisioningStatus
  tier_isolation:      'shared' | 'schema' | 'shard'
  version:             number
  last_login_at:       string | null    // ISO 8601
  last_login_ip:       string | null
  failed_login_count:  number
  locked_until:        string | null
  created_at:          string
  updated_at:          string
  deleted_at:          string | null
}

export interface Connector {
  id:               string
  org_id:           string
  name:             string
  webhook_url:      string | null
  scopes:           ConnectorScope[]
  status:           'active' | 'suspended' | 'pending'
  event_volume_30d: number    // synthesized; from ClickHouse
  last_event_at:    string | null
  created_at:       string
  updated_at:       string
  suspended_at:     string | null
}

export type ConnectorScope =
  | 'events:write'
  | 'policy:evaluate'
  | 'audit:read'
  | 'scim:write'
  | 'dlp:scan'

export interface Policy {
  id:          string
  org_id:      string
  name:        string
  version:     number
  logic:       PolicyLogic
  created_at:  string
  updated_at:  string
}

export interface PolicyLogic {
  rules: PolicyRule[]
}

export interface PolicyRule {
  id:       string    // client-side UUID for drag-and-drop key
  subject:  string    // role | user_id | group | '*'
  action:   string    // read | write | delete | execute | '*'
  resource: string    // free-text
  effect:   'allow' | 'deny'
}

export interface AuditEvent {
  id:           string
  type:         string
  org_id:       string
  actor_id:     string
  actor_type:   'user' | 'service' | 'system'
  occurred_at:  string
  source:       string
  event_source: string    // "internal" | "connector:<id>"
  trace_id:     string
  span_id:      string
  schema_ver:   string
  chain_seq:    number
  chain_hash:   string
  prev_hash:    string
  payload:      Record<string, unknown>
}

export type AlertSeverity = 'low' | 'medium' | 'high' | 'critical'
export type AlertStatus   = 'open' | 'acknowledged' | 'resolved'

export interface ThreatAlert {
  id:              string
  org_id:          string
  actor_id:        string
  detector:        string
  severity:        AlertSeverity
  status:          AlertStatus
  risk_score:      number
  description:     string
  contributing_event_ids: string[]
  saga_steps:      AlertSagaStep[]
  occurred_at:     string
  acknowledged_at: string | null
  resolved_at:     string | null
  resolved_by:     string | null
  resolution_note: string | null
  mttr_seconds:    number | null
  created_at:      string
}

export interface AlertSagaStep {
  step:       number
  label:      string
  status:     'pending' | 'in_progress' | 'completed' | 'failed'
  completed_at: string | null
  error:      string | null
  detail:     string | null    // e.g. "HTTP 200, 142ms" for webhook step
}

export type ReportType   = 'gdpr' | 'soc2' | 'hipaa'
export type ReportStatus = 'pending' | 'processing' | 'completed' | 'failed'

export interface ComplianceReport {
  id:          string
  org_id:      string
  type:        ReportType
  status:      ReportStatus
  period_from: string
  period_to:   string
  sections:    string[]
  s3_key:      string | null
  s3_sig_key:  string | null
  created_at:  string
  completed_at: string | null
  error:       string | null
}

export type FindingType = 'pii' | 'credential' | 'financial'
export type FindingKind = 'email' | 'ssn' | 'credit_card' | 'phone_us' | 'api_key' | 'high_entropy'
export type DLPAction   = 'monitor' | 'mask' | 'block'

export interface DLPFinding {
  id:           string
  org_id:       string
  event_id:     string
  rule_id:      string | null
  finding_type: FindingType
  finding_kind: FindingKind
  json_path:    string
  action_taken: DLPAction
  occurred_at:  string
}

export interface DLPPolicy {
  id:         string
  org_id:     string
  name:       string
  rules:      DLPPolicyRule[]
  enabled:    boolean
  mode:       'monitor' | 'block'
  created_at: string
  updated_at: string
}

export interface DLPPolicyRule {
  id:           string
  type:         'regex' | 'keyword' | 'jsonpath'
  pattern:      string
  finding_type: FindingType
  action:       DLPAction
}

export interface WebhookDelivery {
  id:            string
  connector_id:  string
  event_type:    string
  status:        'delivered' | 'failed' | 'retrying' | 'dead'
  http_status:   number | null
  latency_ms:    number | null
  attempts:      number
  max_attempts:  number
  next_retry_at: string | null
  delivered_at:  string | null
  created_at:    string
  request_body:  string | null    // truncated
  response_body: string | null    // truncated
}

export interface IntegrityResult {
  ok:              boolean
  org_id:          string
  checked_at:      string
  total_events:    number
  gaps:            ChainGap[]
}

export interface ChainGap {
  expected_seq: number
  found_seq:    number
  gap_at:       string    // ISO timestamp of last valid event
}

export interface SystemService {
  name:       string
  status:     'healthy' | 'degraded' | 'down'
  uptime_pct: number
  p99_ms:     number | null
  last_check: string
}

export interface CircuitBreakerState {
  name:                 string
  state:                'closed' | 'half_open' | 'open'
  consecutive_failures: number
  opened_at:            string | null
}

export interface OutboxStats {
  service:         string
  pending_records: number
  relay_lag_ms:    number
}
```

---

## 15.2 API Response Types

```ts
// types/api.ts
import type {
  User, Connector, Policy, AuditEvent, ThreatAlert,
  ComplianceReport, DLPFinding, DLPPolicy, WebhookDelivery
} from './models'

// Standard pagination wrappers (matches BE spec §4.7)
export interface OffsetMeta {
  page:        number
  per_page:    number
  total:       number
  total_pages: number
  next_cursor: null
}

export interface CursorMeta {
  per_page:    number
  next_cursor: string | null
  total:       null
}

export interface OffsetPage<T> { data: T[]; meta: OffsetMeta }
export interface CursorPage<T> { data: T[]; meta: CursorMeta }

// API error shape (matches BE models.APIErrorBody)
export interface APIErrorBody {
  code:       string
  message:    string
  request_id: string
  trace_id:   string
  retryable:  boolean
  fields?:    { field: string; message: string }[]
}

// Specific response types
export interface ConnectorCreateResponse {
  connector:          Connector
  api_key_plaintext:  string    // one-time; never stored
  api_key_prefix:     string    // first 8 chars; safe to log
}

export interface PolicyEvaluateRequest {
  user_id:    string
  action:     string
  resource:   string
  user_groups?: string[]
  context?:   Record<string, unknown>
}

export interface PolicyEvaluateResponse {
  permitted:        boolean
  matched_policies: string[]
  reason:           string
  cache_hit:        'none' | 'redis' | 'sdk'
  evaluated_at:     string
  latency_ms:       number
}

export interface EvalLogEntry {
  id:           string
  org_id:       string
  user_id:      string
  action:       string
  resource:     string
  result:       boolean
  policy_ids:   string[]
  latency_ms:   number
  cache_hit:    'none' | 'redis' | 'sdk'
  evaluated_at: string
}

export interface ExportJob {
  job_id:      string
  status:      'pending' | 'processing' | 'completed' | 'failed'
  format:      'csv' | 'json'
  size_bytes:  number | null
  created_at:  string
  completed_at: string | null
  download_url: string | null    // pre-signed S3 URL, expires 1h
  error:        string | null
}

export interface ThreatStats {
  open_by_severity:         Record<string, number>
  acknowledged_by_severity: Record<string, number>
  resolved_7d:              number
  avg_mttr_seconds:         Record<string, number>
}

export interface PostureResult {
  score:    number    // 0–100
  controls: ControlStatus[]
  trend:    number    // delta vs last period
}

export interface ControlStatus {
  framework: 'gdpr' | 'soc2' | 'hipaa'
  control:   string
  status:    'compliant' | 'needs_attention' | 'non_compliant'
  evidence:  string | null
  guidance:  string | null
}

export interface SystemStatus {
  services:        SystemService[]
  circuit_breakers: CircuitBreakerState[]
  outbox:          OutboxStats[]
  dependencies: {
    postgres:    'healthy' | 'degraded' | 'down'
    mongodb:     'healthy' | 'degraded' | 'down'
    redis:       'healthy' | 'degraded' | 'down'
    kafka:       'healthy' | 'degraded' | 'down'
    clickhouse:  'healthy' | 'degraded' | 'down'
  }
}
```

---

## 15.3 Zod Form Validators

```ts
// lib/validators/connector.ts
import { z } from 'zod'

const CONNECTOR_SCOPES = [
  'events:write',
  'policy:evaluate',
  'audit:read',
  'scim:write',
  'dlp:scan',
] as const

export const connectorCreateSchema = z.object({
  name: z
    .string()
    .min(2, 'Name must be at least 2 characters')
    .max(64, 'Name must be at most 64 characters'),
  webhook_url: z
    .string()
    .url('Must be a valid URL')
    .refine(url => url.startsWith('https://'), {
      message: 'Webhook URL must use HTTPS',
    })
    .optional()
    .or(z.literal('')),
  description: z.string().max(256).optional(),
  scopes: z
    .array(z.enum(CONNECTOR_SCOPES))
    .min(1, 'At least one scope is required'),
})

export type ConnectorCreateInput = z.infer<typeof connectorCreateSchema>
```

```ts
// lib/validators/policy.ts
import { z } from 'zod'

export const policyRuleSchema = z.object({
  id:       z.string().uuid(),
  subject:  z.string().min(1, 'Subject is required'),
  action:   z.string().min(1, 'Action is required'),
  resource: z.string().min(1, 'Resource is required'),
  effect:   z.enum(['allow', 'deny']),
})

export const policyCreateSchema = z.object({
  name:        z.string().min(1, 'Policy name is required').max(128),
  description: z.string().max(512).optional(),
  rules:       z.array(policyRuleSchema).min(1, 'At least one rule is required'),
})

export type PolicyCreateInput = z.infer<typeof policyCreateSchema>
```

```ts
// lib/validators/dlp.ts
import { z } from 'zod'

export const dlpPolicyRuleSchema = z.object({
  id:           z.string().uuid(),
  type:         z.enum(['regex', 'keyword', 'jsonpath']),
  pattern:      z.string().min(1, 'Pattern is required'),
  finding_type: z.enum(['pii', 'credential', 'financial']),
  action:       z.enum(['monitor', 'mask', 'block']),
})

export const dlpPolicyCreateSchema = z.object({
  name:    z.string().min(1).max(128),
  mode:    z.enum(['monitor', 'block']),
  enabled: z.boolean(),
  rules:   z.array(dlpPolicyRuleSchema),
})

export type DLPPolicyCreateInput = z.infer<typeof dlpPolicyCreateSchema>
```

```ts
// lib/validators/user.ts
import { z } from 'zod'

export const userInviteSchema = z.object({
  email:        z.string().email('Must be a valid email address'),
  display_name: z.string().min(1).max(128),
})

export const orgSettingsSchema = z.object({
  name:         z.string().min(2).max(64),
  max_users:    z.number().int().min(1).optional(),
  max_sessions: z.number().int().min(1).max(100),
  mfa_required: z.boolean(),
  sso_required: z.boolean(),
})

export const apiTokenCreateSchema = z.object({
  name:      z.string().min(1).max(64),
  scopes:    z.array(z.string()).min(1),
  expires_at: z.string().datetime().optional(),
})
```

```ts
// lib/validators/report.ts
import { z } from 'zod'

export const reportCreateSchema = z.object({
  type:        z.enum(['gdpr', 'soc2', 'hipaa']),
  period_from: z.string().datetime(),
  period_to:   z.string().datetime(),
  sections:    z.array(z.string()).min(1),
}).refine(
  data => new Date(data.period_from) < new Date(data.period_to),
  { message: 'Period start must be before period end', path: ['period_from'] }
)

export type ReportCreateInput = z.infer<typeof reportCreateSchema>
```

```ts
// lib/validators/evaluate.ts
import { z } from 'zod'

export const evaluateRequestSchema = z.object({
  user_id:     z.string().min(1, 'User ID is required'),
  action:      z.string().min(1, 'Action is required'),
  resource:    z.string().min(1, 'Resource is required'),
  user_groups: z.array(z.string()).optional(),
  context:     z.record(z.unknown()).optional(),
})

export type EvaluateRequest = z.infer<typeof evaluateRequestSchema>
```

---

## 15.4 SSE Event Types

```ts
// types/events.ts
// Shapes of EventEnvelope payloads received via SSE streams.
// These match the BE's models.EventEnvelope (BE spec §4.1).

export interface SSEAuditEvent {
  id:           string
  type:         string
  org_id:       string
  actor_id:     string
  actor_type:   'user' | 'service' | 'system'
  occurred_at:  string
  source:       string
  event_source: string
  trace_id:     string
  schema_ver:   string
  chain_seq:    number
  payload:      Record<string, unknown>
}

export interface SSEThreatAlert {
  alert_id:    string
  severity:    'low' | 'medium' | 'high' | 'critical'
  detector:    string
  actor_id:    string
  description: string
  occurred_at: string
}

// Stream event wrapper — every SSE message has this shape
export interface SSEMessage<T> {
  event: string    // e.g. "audit.event" | "threat.alert"
  data:  T
}
```

---

## 15.5 Type Guards

```ts
// lib/utils/type-guards.ts
import type { APIErrorBody } from '@/types/api'

export function isAPIError(value: unknown): value is { error: APIErrorBody } {
  return (
    typeof value === 'object' &&
    value !== null &&
    'error' in value &&
    typeof (value as any).error.code === 'string'
  )
}

export function isCursorPage<T>(page: unknown): page is { data: T[]; meta: { next_cursor: string | null } } {
  return (
    typeof page === 'object' &&
    page !== null &&
    'data' in page &&
    'meta' in page &&
    'next_cursor' in (page as any).meta
  )
}
```
