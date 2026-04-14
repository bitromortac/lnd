-- Add BOLT 12 specific columns to the invoices table. These are nullable
-- because BOLT 11 invoices do not populate them.

-- Type discriminator following the existing is_amp, is_hodl, is_keysend
-- pattern.
ALTER TABLE invoices ADD COLUMN is_bolt12 BOOLEAN NOT NULL DEFAULT FALSE;

-- Foreign key to the offers table. NULL for BOLT 11 invoices and for
-- BOLT 12 invoices not tied to an offer.
ALTER TABLE invoices ADD COLUMN offer_id BIGINT REFERENCES offers(id);

-- The 33-byte compressed pubkey that signed the BOLT 12 invoice
-- (invoice_node_id, TLV type 176). NULL for BOLT 11.
ALTER TABLE invoices ADD COLUMN invoice_node_id BLOB;

-- The 33-byte compressed pubkey from invreq_payer_id (TLV type 88).
-- NULL for BOLT 11.
ALTER TABLE invoices ADD COLUMN invreq_payer_id BLOB;

CREATE INDEX IF NOT EXISTS invoices_is_bolt12_idx ON invoices(is_bolt12);
CREATE INDEX IF NOT EXISTS invoices_offer_id_idx ON invoices(offer_id);
