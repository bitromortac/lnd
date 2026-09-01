-- name: InsertOffer :one
INSERT INTO offers (
    offer_id, encoded, issuer_node_id, description, amount_msat,
    currency, absolute_expiry, quantity_max, is_disabled, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING id;

-- name: GetOfferByID :one
SELECT *
FROM offers
WHERE id = $1;

-- name: GetOfferByOfferID :one
SELECT *
FROM offers
WHERE offer_id = $1;

-- name: ListOffers :many
-- ListOffers returns offers ordered by creation time. The caller supplies
-- Go-side defaults when a filter is not needed:
--   include_disabled → true  (include all offers)
SELECT *
FROM offers
WHERE (NOT @active_only OR is_disabled = FALSE)
ORDER BY id ASC
LIMIT @num_limit OFFSET @num_offset;

-- name: DisableOffer :execresult
UPDATE offers
SET is_disabled = TRUE
WHERE id = $1;

-- name: EnableOffer :execresult
UPDATE offers
SET is_disabled = FALSE
WHERE id = $1;
