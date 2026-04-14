-- Stores outgoing BOLT 12 invoice request/response pairs for
-- proof-of-payer and per-offer negotiation history.

CREATE TABLE IF NOT EXISTS invoice_request_store (
    -- Primary key for the record.
    id INTEGER PRIMARY KEY,

    -- The 32-byte SHA256 hash of the TLV-encoded offer. Indexed
    -- for per-offer queries. Denormalized from the invreq content
    -- for fast lookups without blob parsing.
    offer_id BLOB NOT NULL,

    -- The full TLV-encoded invoice request (includes payer
    -- signature, TLV type 240).
    invreq_bytes BLOB NOT NULL,

    -- The full TLV-encoded BOLT 12 invoice received in response.
    -- NULL until the invoice reply arrives.
    invoice_bytes BLOB,

    -- The 32-byte ephemeral private key scalar for
    -- invreq_payer_id. Never appears in any wire message.
    payer_key BLOB NOT NULL,

    -- Timestamp of when this negotiation was initiated.
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_invreq_store_offer_id
ON invoice_request_store(offer_id);
