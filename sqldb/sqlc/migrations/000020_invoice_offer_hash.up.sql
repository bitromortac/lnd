-- Denormalize the 32-byte SHA256 offer hash onto the invoices table so
-- RPC marshaling can read it directly without JOIN-ing the offers table.
ALTER TABLE invoices ADD COLUMN offer_hash BLOB;
