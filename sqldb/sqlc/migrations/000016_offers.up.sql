-- offers stores long-lived BOLT 12 offer templates. Each offer can generate
-- many invoices over its lifetime.
CREATE TABLE IF NOT EXISTS offers (
    -- Primary key for the offer record.
    id INTEGER PRIMARY KEY,

    -- The SHA256 hash of the TLV-encoded offer, used as a unique external
    -- identifier. 32 bytes.
    offer_id BLOB NOT NULL UNIQUE,

    -- The full bech32-encoded offer string (lno1...). This is the
    -- authoritative source for all offer fields.
    encoded TEXT NOT NULL,

    -- The 33-byte compressed public key of the offer issuer
    -- (offer_issuer_id, TLV type 22).
    issuer_node_id BLOB NOT NULL,

    -- A UTF-8 description of the purpose of the payment
    -- (offer_description, TLV type 10).
    description TEXT,

    -- The amount expected per item in millisatoshis. NULL when the offer
    -- uses a non-lightning currency or has no fixed amount.
    amount_msat BIGINT,

    -- ISO 4217 currency code when the offer amount is not in
    -- millisatoshis. NULL for msat-denominated offers.
    currency TEXT,

    -- Seconds since epoch after which the offer should not be used
    -- (offer_absolute_expiry, TLV type 14). NULL means no expiry.
    absolute_expiry BIGINT,

    -- Maximum number of items that can be requested in a single invoice
    -- (offer_quantity_max, TLV type 20). 0 means unlimited. NULL means
    -- quantity is not supported.
    quantity_max BIGINT,

    -- Whether the offer has been administratively disabled. Disabled
    -- offers reject new invoice requests but allow in-flight invoices
    -- to settle.
    is_disabled BOOLEAN NOT NULL DEFAULT FALSE,

    -- Timestamp of when this offer was created.
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS offers_offer_id_idx ON offers(offer_id);
CREATE INDEX IF NOT EXISTS offers_issuer_node_id_idx ON offers(issuer_node_id);
CREATE INDEX IF NOT EXISTS offers_created_at_idx ON offers(created_at);
