ALTER TABLE policy_eval_log ALTER COLUMN cache_hit TYPE BOOLEAN
USING (cache_hit != 'none');
