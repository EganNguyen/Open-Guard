-- Migration to fix cache_hit column type from BOOLEAN to TEXT
-- Per spec §11.1 and Senior Architect Review P1 recommendation

ALTER TABLE policy_eval_log 
    ALTER COLUMN cache_hit TYPE TEXT 
    USING (CASE WHEN cache_hit THEN 'redis' ELSE 'none' END);

ALTER TABLE policy_eval_log 
    ALTER COLUMN cache_hit SET DEFAULT 'none';
