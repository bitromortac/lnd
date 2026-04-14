-- name: InsertBolt12Invoice :exec
INSERT INTO bolt12_invoices (
    invoice_id, offer_id, invreq_payer_id, invreq_quantity
) VALUES (
    $1, $2, $3, $4
);

-- name: FetchBolt12Invoice :one
SELECT b.offer_id, b.invreq_payer_id, b.invreq_quantity, o.hash AS offer_hash
FROM bolt12_invoices b
LEFT JOIN offers o ON o.id = b.offer_id
WHERE b.invoice_id = $1;

-- name: FetchBolt12Payment :one
SELECT k.idempotency_key, k.payment_id, k.offer_hash, k.params_hash,
    p.payment_identifier
FROM bolt12_payments k
LEFT JOIN payments p ON p.id = k.payment_id
WHERE k.idempotency_key = $1;

-- name: UpsertBolt12Payment :exec
INSERT INTO bolt12_payments (
    idempotency_key, payment_id, offer_hash, params_hash, created_at
) VALUES (
    $1, $2, $3, $4, $5
) ON CONFLICT (idempotency_key) DO UPDATE SET
    payment_id = excluded.payment_id;

-- name: FetchBolt12PaymentIDsByOffer :many
SELECT k.payment_id
FROM bolt12_payments k
WHERE k.offer_hash = $1 AND k.payment_id IS NOT NULL
ORDER BY k.payment_id;
