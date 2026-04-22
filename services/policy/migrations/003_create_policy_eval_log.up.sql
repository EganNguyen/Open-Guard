-- Policy eval log per spec §11.1
-- Records each policy evaluation decision for audit and debugging

CREATE TABLE policy_eval_log (
    id                 UUID        PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id             UUID        NOT NULL,
    subject_id         TEXT        NOT NULL,   -- user_id or connector_id
    action             TEXT        NOT NULL,
    resource           TEXT        NOT NULL,
    effect             TEXT        NOT NULL,   -- 'allow' or 'deny'
    matched_policy_ids UUID[]      NOT NULL DEFAULT '{}',
    cache_hit          BOOLEAN     NOT NULL DEFAULT FALSE,
    latency_ms         INT         NOT NULL DEFAULT 0,
    evaluated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policy_eval_log_org_id ON policy_eval_log(org_id);
CREATE INDEX idx_policy_eval_log_evaluated_at ON policy_eval_log(evaluated_at DESC);

-- Enable RLS
ALTER TABLE policy_eval_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_eval_log FORCE ROW LEVEL SECURITY;

CREATE POLICY policy_eval_log_org_isolation ON policy_eval_log
    USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
    WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT ON policy_eval_log TO openguard_app;
