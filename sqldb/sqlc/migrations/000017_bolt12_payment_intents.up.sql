-- Add offer_hash to payment_intents for BOLT 12 offer-level queries.
-- NULL for BOLT 11 payments. The 32-byte SHA256 hash of the TLV-encoded
-- offer is a deterministic content-addressable identifier.

ALTER TABLE payment_intents ADD COLUMN offer_hash BLOB;

CREATE INDEX IF NOT EXISTS idx_payment_intents_offer_hash
ON payment_intents(offer_hash);
