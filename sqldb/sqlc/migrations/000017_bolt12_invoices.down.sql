DROP INDEX IF EXISTS invoices_offer_id_idx;
DROP INDEX IF EXISTS invoices_is_bolt12_idx;

-- SQLite does not support DROP COLUMN, so for SQLite the down migration
-- would require recreating the table. For Postgres the ALTERs work directly.
-- Since we only support the SQL store going forward, we use standard SQL.
ALTER TABLE invoices DROP COLUMN IF EXISTS invreq_payer_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS invoice_node_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS offer_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS is_bolt12;
