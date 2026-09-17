-- +goose Up
-- 00011 establishes the Arena Pass entitlement schema (P07-T01):
-- app.arena_pass_lots and app.arena_pass_consumptions.
--
-- Invariants (docs/MONETIZATION.md §3):
-- 1. A lot records its origin (PURCHASE, MEMBER or ADMIN), the granted
--    quantity, an optional expiration and the stable reference of the cause.
--    Idempotency of grants is anchored on (account_id, origin, reference):
--    a retry can never create a second lot for the same grant.
-- 2. quantity > 0 and 0 <= remaining_quantity <= quantity: consuming above
--    the lot is impossible at the database level (the conditional decrement
--    plus this CHECK make over-consumption fail, not merely be avoided).
-- 3. Expiration is optional and immutable: bought passes do not expire
--    initially, Member passes expire at the end of their period and expiry is
--    derived from expires_at, never by deleting or mutating the lot.
-- 4. Consumption is append-only and unique per Arena: publishing an Arena
--    consumes exactly one pass, and the same Arena can never consume twice.
--    arena_id has no foreign key yet: app.arenas arrives with phase 08 and
--    will add the reference in its own migration.
-- 5. Entitlements are retained: no cascade from accounts (ON DELETE
--    RESTRICT); account deletion is anonymization, not silent removal.
-- 6. Least privilege: arena_owner owns the objects; arena_app may read and
--    insert, update the remaining projection, and never delete.

CREATE TABLE IF NOT EXISTS app.arena_pass_lots (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    account_id uuid NOT NULL REFERENCES app.accounts(id) ON DELETE RESTRICT,
    origin text NOT NULL,
    quantity integer NOT NULL,
    remaining_quantity integer NOT NULL,
    expires_at timestamptz,
    reference text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT arena_pass_lots_origin_check CHECK (origin IN ('PURCHASE', 'MEMBER', 'ADMIN')),
    CONSTRAINT arena_pass_lots_quantity_check CHECK (quantity > 0),
    CONSTRAINT arena_pass_lots_remaining_check CHECK (remaining_quantity >= 0 AND remaining_quantity <= quantity),
    CONSTRAINT arena_pass_lots_reference_check CHECK (trim(reference) <> ''),
    CONSTRAINT arena_pass_lots_grant_unique UNIQUE (account_id, origin, reference)
);

CREATE INDEX IF NOT EXISTS arena_pass_lots_account_id_idx ON app.arena_pass_lots (account_id, expires_at NULLS LAST, created_at);

COMMENT ON TABLE app.arena_pass_lots IS 'Arena Pass lots: origin, granted quantity, optional immutable expiration and stable reference';
COMMENT ON COLUMN app.arena_pass_lots.remaining_quantity IS 'Consumable projection of the lot; over-consumption fails the CHECK constraint';
COMMENT ON COLUMN app.arena_pass_lots.expires_at IS 'Optional immutable expiration (NULL never expires); Member lots expire at their period end';

CREATE TABLE IF NOT EXISTS app.arena_pass_consumptions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    lot_id uuid NOT NULL REFERENCES app.arena_pass_lots(id) ON DELETE RESTRICT,
    arena_id uuid NOT NULL,
    consumed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT arena_pass_consumptions_arena_unique UNIQUE (arena_id)
);

CREATE INDEX IF NOT EXISTS arena_pass_consumptions_lot_id_idx ON app.arena_pass_consumptions (lot_id);

COMMENT ON TABLE app.arena_pass_consumptions IS 'Append-only Arena Pass consumptions; one row per published Arena, never updated or deleted at runtime';

ALTER TABLE app.arena_pass_lots OWNER TO arena_owner;
ALTER TABLE app.arena_pass_consumptions OWNER TO arena_owner;

-- Runtime surface: lots are a mutable projection (no DELETE), consumptions
-- are append-only (SELECT/INSERT only): history can never be rewritten.
GRANT SELECT, INSERT, UPDATE ON app.arena_pass_lots TO arena_app;
GRANT SELECT, INSERT ON app.arena_pass_consumptions TO arena_app;

-- +goose Down
DROP TABLE IF EXISTS app.arena_pass_consumptions;
DROP TABLE IF EXISTS app.arena_pass_lots;
