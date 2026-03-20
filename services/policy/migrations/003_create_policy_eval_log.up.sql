-- 003_create_policy_eval_log.up.sql
-- Policy evaluation cache log for audit purposes (Phase 2 — Section 10.1).

CREATE TABLE IF NOT EXISTS policy_eval_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    user_id      UUID NOT NULL,
    action       TEXT NOT NULL,
    resource     TEXT NOT NULL,
    result       BOOLEAN NOT NULL,
    policy_ids   UUID[] NOT NULL DEFAULT '{}',
    latency_ms   INT NOT NULL,
    cached       BOOLEAN NOT NULL DEFAULT FALSE,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policy_eval_org_user ON policy_eval_log(org_id, user_id, evaluated_at DESC);

ALTER TABLE policy_eval_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_eval_log FORCE ROW LEVEL SECURITY;
CREATE POLICY policy_eval_org_isolation ON policy_eval_log
    USING (org_id = current_setting('app.org_id', true)::UUID);
