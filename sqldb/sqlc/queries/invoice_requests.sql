-- name: InsertInvoiceRequest :one
-- Insert an outgoing invoice request record. invoice_bytes is NULL
-- until the reply arrives.
INSERT INTO invoice_request_store (
    offer_id,
    invreq_bytes,
    invoice_bytes,
    payer_key,
    created_at
) VALUES (
    @offer_id,
    @invreq_bytes,
    @invoice_bytes,
    @payer_key,
    @created_at
) RETURNING id;

-- name: UpdateInvoiceRequestInvoice :exec
-- Set the invoice_bytes on an existing record after the invoice
-- reply arrives.
UPDATE invoice_request_store
SET invoice_bytes = @invoice_bytes
WHERE id = @id;

-- name: FetchInvoiceRequestsByOfferID :many
-- Fetch all invoice request records for a given offer, ordered by
-- creation time descending.
SELECT *
FROM invoice_request_store
WHERE offer_id = $1
ORDER BY created_at DESC;
