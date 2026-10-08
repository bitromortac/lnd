-- bolt12_invoice_requests holds the invoice requests without an offer that
-- this node published as a payer. An lnr1 string is an offer to send money, so
-- the answering invoice arrives later with no call waiting for it, and the
-- node needs the request to match and check that invoice. Only the operator
-- creates rows, so an incoming invoice never writes here before it is paid.
CREATE TABLE IF NOT EXISTS bolt12_invoice_requests (
    -- Primary key for the published request.
    id INTEGER PRIMARY KEY,

    -- The caller's idempotency key. The metadata and the payer key of the
    -- request are derived from it, and creating a request with a known key
    -- returns the stored one.
    idempotency_key BLOB NOT NULL UNIQUE,

    -- The invreq_metadata of the request. An invoice mirrors it, so it finds
    -- the request an incoming invoice answers.
    invreq_metadata BLOB NOT NULL UNIQUE,

    -- The signed request as an lnr1 string. It is the authoritative source
    -- for all request fields.
    encoded TEXT NOT NULL,

    -- The invreq_amount the request pays, in msat.
    amount_msat BIGINT NOT NULL,

    -- The time after which the node no longer pays an invoice for the
    -- request. NULL when the request does not expire.
    expires_at TIMESTAMP,

    -- The invoice_node_id the payer agreed with the payee out of band. An
    -- invoice from this node is paid at once. When NULL, the node pays only
    -- after the user approves the invoice. 33 bytes.
    expected_node_id BLOB,

    -- The routing fee budget the operator approved for paying an invoice
    -- at once, in msat. 0 selects the default fee limit.
    fee_limit_msat BIGINT NOT NULL DEFAULT 0,

    -- Whether a payment ever started for the request. A request pays at
    -- most once, so a used request whose payment was deleted stays used.
    used BOOLEAN NOT NULL DEFAULT FALSE,

    -- The current payment of the request. A failed payment frees the
    -- request for a new invoice.
    payment_id BIGINT UNIQUE REFERENCES payments(id) ON DELETE SET NULL,

    -- Timestamp of when the request was created.
    created_at TIMESTAMP NOT NULL
);
