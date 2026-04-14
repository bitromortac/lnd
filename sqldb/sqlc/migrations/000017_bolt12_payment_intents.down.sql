DROP INDEX IF EXISTS idx_payment_intents_offer_id;

ALTER TABLE payment_intents DROP COLUMN IF EXISTS offer_id;
