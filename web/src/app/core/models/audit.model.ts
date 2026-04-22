import { z } from 'zod';

export const AuditLogSchema = z.object({
  id: z.string(),
  org_id: z.string(),
  subject_id: z.string(),
  action: z.string(),
  resource: z.string(),
  effect: z.enum(['allow', 'deny']),
  matched_policy_ids: z.array(z.string()),
  cache_hit: z.boolean(),
  latency_ms: z.number(),
  created_at: z.string(),
});

export type AuditLog = z.infer<typeof AuditLogSchema>;
