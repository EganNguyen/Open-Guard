DO $$
BEGIN
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'policy_eval_log' 
        AND column_name = 'cache_hit' 
        AND data_type = 'boolean'
    ) THEN
        ALTER TABLE policy_eval_log 
            ALTER COLUMN cache_hit TYPE TEXT 
            USING (CASE WHEN cache_hit THEN 'redis' ELSE 'none' END);
        
        ALTER TABLE policy_eval_log 
            ALTER COLUMN cache_hit SET DEFAULT 'none';
    END IF;
END $$;
