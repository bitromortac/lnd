DROP INDEX IF EXISTS idx_payment_intents_offer_hash;

ALTER TABLE payment_intents DROP COLUMN IF EXISTS offer_hash;
