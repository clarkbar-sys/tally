// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/clarkbar-sys/tally/internal/model"
)

// UpsertAccount inserts a, or updates it in place, keyed on
// (Provider, ProviderAccountID) per the merge rule documented in
// internal/model. The returned Account has ID, CreatedAt, and UpdatedAt
// populated from the row.
func (s *Store) UpsertAccount(ctx context.Context, a model.Account) (model.Account, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO accounts (
			provider, provider_account_id, institution, name, type, currency,
			balance_cents, last_synced_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (provider, provider_account_id) DO UPDATE SET
			institution    = excluded.institution,
			name           = excluded.name,
			type           = excluded.type,
			currency       = excluded.currency,
			balance_cents  = excluded.balance_cents,
			last_synced_at = excluded.last_synced_at,
			updated_at     = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		RETURNING id, created_at, updated_at`,
		a.Provider, a.ProviderAccountID, a.Institution, a.Name, a.Type, a.Currency,
		a.BalanceCents, formatNullTime(a.LastSyncedAt),
	)

	var createdAt, updatedAt string
	if err := row.Scan(&a.ID, &createdAt, &updatedAt); err != nil {
		return model.Account{}, fmt.Errorf("store: upsert account %s/%s: %w", a.Provider, a.ProviderAccountID, err)
	}
	var err error
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Account{}, fmt.Errorf("store: upsert account: parse created_at: %w", err)
	}
	if a.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Account{}, fmt.Errorf("store: upsert account: parse updated_at: %w", err)
	}
	return a, nil
}

// GetAccount returns the account with the given internal ID.
func (s *Store) GetAccount(ctx context.Context, id int64) (model.Account, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, provider, provider_account_id, institution, name, type, currency,
		       balance_cents, last_synced_at, created_at, updated_at
		FROM accounts WHERE id = ?`, id)
	return scanAccount(row)
}

// ListAccounts returns every account, ordered by institution then name.
func (s *Store) ListAccounts(ctx context.Context) ([]model.Account, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, provider, provider_account_id, institution, name, type, currency,
		       balance_cents, last_synced_at, created_at, updated_at
		FROM accounts ORDER BY institution, name`)
	if err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []model.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	return accounts, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccount(row rowScanner) (model.Account, error) {
	var a model.Account
	var lastSyncedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(
		&a.ID, &a.Provider, &a.ProviderAccountID, &a.Institution, &a.Name, &a.Type, &a.Currency,
		&a.BalanceCents, &lastSyncedAt, &createdAt, &updatedAt,
	); err != nil {
		return model.Account{}, fmt.Errorf("store: scan account: %w", err)
	}

	var err error
	if a.LastSyncedAt, err = parseNullTime(lastSyncedAt); err != nil {
		return model.Account{}, fmt.Errorf("store: scan account: parse last_synced_at: %w", err)
	}
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Account{}, fmt.Errorf("store: scan account: parse created_at: %w", err)
	}
	if a.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Account{}, fmt.Errorf("store: scan account: parse updated_at: %w", err)
	}
	return a, nil
}
