-- +goose Up
-- 00009 persists the monthly FREE cycle anchor on app.wallet_accounts
-- (P06-T06): the free plan period is anchored on the instant the wallet was
-- first funded, normally the account activation grant. The anchor is never
-- changed afterwards: rights already acquired are not rewritten
-- retroactively (docs/MONETIZATION.md §4), and the cycle calendar is always
-- computed in UTC, so a preference timezone can never shift a renewal.
--
-- The default covers wallets created before this column: their cycle starts
-- at wallet creation, the best available activation instant.

ALTER TABLE app.wallet_accounts
    ADD COLUMN IF NOT EXISTS free_cycle_anchor_at timestamptz NOT NULL DEFAULT now();

COMMENT ON COLUMN app.wallet_accounts.free_cycle_anchor_at IS 'Monthly FREE_INK cycle anchor (activation instant); immutable and evaluated in UTC';

-- +goose Down
ALTER TABLE app.wallet_accounts DROP COLUMN IF EXISTS free_cycle_anchor_at;
