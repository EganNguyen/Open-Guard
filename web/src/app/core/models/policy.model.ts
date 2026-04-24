import { z } from 'zod';

export const PolicyLogicSchema = z.object({
  type: z.enum(['rbac', 'deny_all', 'allow_all']),
  subjects: z.array(z.string()).optional(),
  actions: z.array(z.string()).optional(),
  resources: z.array(z.string()).optional(),
});

export const PolicySchema = z.object({
  id: z.string(),
  org_id: z.string(),
  name: z.string().min(1, 'Name is required'),
  description: z.string().optional(),
  logic: PolicyLogicSchema,
  version: z.number(),
  created_at: z.string(),
  updated_at: z.string(),
});

export type Policy = z.infer<typeof PolicySchema>;
export type PolicyLogic = z.infer<typeof PolicyLogicSchema>;

export const EvaluateRequestSchema = z.object({
  org_id: z.string(),
  subject_id: z.string().min(1, 'Subject is required'),
  action: z.string().min(1, 'Action is required'),
  resource: z.string().min(1, 'Resource is required'),
});

export type EvaluateRequest = z.infer<typeof EvaluateRequestSchema>;

export interface EvaluateResponse {
  effect: 'allow' | 'deny';
  matched_policy_ids: string[];
  max_version: number;
  cache_hit: 'none' | 'redis' | 'sdk';
  latency_ms: number;
}
