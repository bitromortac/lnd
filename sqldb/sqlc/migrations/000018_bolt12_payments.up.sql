-- bolt12_payments holds one row for each idempotency key of an offer
-- payment. A key names one intended payment. At most one payment that has not
-- failed can belong to a key, and a key whose payment failed moves to the
-- retry. Only BOLT 12 queries touch this table.
CREATE TABLE IF NOT EXISTS bolt12_payments (
    -- The caller's idempotency key.
    idempotency_key BLOB PRIMARY KEY,

    -- The current payment of the key. It becomes NULL when that payment is
    -- deleted, and the key then stays used for good.
    payment_id BIGINT UNIQUE REFERENCES payments(id) ON DELETE SET NULL,

    -- The hash of the offer the key pays. 32 bytes.
    offer_hash BLOB NOT NULL,

    -- The hash of the parameters that define the payment: offer hash,
    -- amount, quantity and payer note. A known key with other parameters
    -- is refused. 32 bytes.
    params_hash BLOB NOT NULL,

    -- Timestamp of when the key was first used.
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS bolt12_payments_offer_hash_idx
ON bolt12_payments(offer_hash);
