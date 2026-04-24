DROP TRIGGER IF EXISTS outbox_insert_notify ON outbox_records;
DROP FUNCTION IF EXISTS notify_outbox();
DROP TABLE IF EXISTS outbox_records;
