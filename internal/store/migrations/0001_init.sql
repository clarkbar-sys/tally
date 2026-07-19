-- accounts and transactions hold only provider-sourced fields (plus the
-- identity needed to reference a row). Tally-owned annotation -- tags, notes,
-- story links (#13, #14, #15) -- will live in their own tables that reference
-- transactions.id by foreign key, so that a re-sync (which only ever upserts
-- the columns below) can never clobber annotation. See internal/model's
-- package doc for the full identity and merge rule.

CREATE TABLE accounts (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    provider             TEXT NOT NULL,
    provider_account_id  TEXT NOT NULL,
    institution          TEXT NOT NULL,
    name                 TEXT NOT NULL,
    type                 TEXT NOT NULL,
    currency             TEXT NOT NULL,
    balance_cents        INTEGER NOT NULL DEFAULT 0,
    last_synced_at       TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    UNIQUE (provider, provider_account_id)
);

CREATE TABLE transactions (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id               INTEGER NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    provider                 TEXT NOT NULL,
    provider_transaction_id  TEXT NOT NULL,
    status                   TEXT NOT NULL,
    transacted_at            TEXT NOT NULL,
    posted_at                TEXT,
    amount_cents             INTEGER NOT NULL,
    currency                 TEXT NOT NULL,
    description              TEXT NOT NULL,
    raw_payload              TEXT,
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL,
    UNIQUE (account_id, provider_transaction_id)
);

CREATE INDEX idx_transactions_account_transacted
    ON transactions (account_id, transacted_at DESC);
