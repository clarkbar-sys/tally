// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/clarkbar-sys/tally/internal/model"
)

// UpsertTransaction inserts t, or updates it in place, keyed on
// (AccountID, ProviderTransactionID) per the merge rule documented in
// internal/model: every provider-sourced field is overwritten, the internal ID
// is preserved, and tally-owned annotation (referencing that ID) is never
// touched. The returned Transaction has ID, CreatedAt, and UpdatedAt populated
// from the row.
func (s *Store) UpsertTransaction(ctx context.Context, t model.Transaction) (model.Transaction, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO transactions (
			account_id, provider, provider_transaction_id, status, transacted_at,
			posted_at, amount_cents, currency, description, raw_payload,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (account_id, provider_transaction_id) DO UPDATE SET
			status        = excluded.status,
			transacted_at = excluded.transacted_at,
			posted_at     = excluded.posted_at,
			amount_cents  = excluded.amount_cents,
			currency      = excluded.currency,
			description   = excluded.description,
			raw_payload   = excluded.raw_payload,
			updated_at    = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		RETURNING id, created_at, updated_at`,
		t.AccountID, t.Provider, t.ProviderTransactionID, string(t.Status), formatTime(t.TransactedAt),
		formatNullTime(t.PostedAt), t.AmountCents, t.Currency, t.Description, t.RawPayload,
	)

	var createdAt, updatedAt string
	if err := row.Scan(&t.ID, &createdAt, &updatedAt); err != nil {
		return model.Transaction{}, fmt.Errorf(
			"store: upsert transaction %s/%s: %w", t.Provider, t.ProviderTransactionID, err)
	}
	var err error
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Transaction{}, fmt.Errorf("store: upsert transaction: parse created_at: %w", err)
	}
	if t.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Transaction{}, fmt.Errorf("store: upsert transaction: parse updated_at: %w", err)
	}
	return t, nil
}

// GetTransaction returns the transaction with the given internal ID.
func (s *Store) GetTransaction(ctx context.Context, id int64) (model.Transaction, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, provider, provider_transaction_id, status, transacted_at,
		       posted_at, amount_cents, currency, description, raw_payload, created_at, updated_at
		FROM transactions WHERE id = ?`, id)
	return scanTransaction(row)
}

// ListTransactionsByAccount returns every transaction for accountID, most
// recent first.
func (s *Store) ListTransactionsByAccount(ctx context.Context, accountID int64) ([]model.Transaction, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, provider, provider_transaction_id, status, transacted_at,
		       posted_at, amount_cents, currency, description, raw_payload, created_at, updated_at
		FROM transactions WHERE account_id = ? ORDER BY transacted_at DESC, id DESC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list transactions for account %d: %w", accountID, err)
	}
	defer rows.Close()

	var txns []model.Transaction
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list transactions for account %d: %w", accountID, err)
	}
	return txns, nil
}

func scanTransaction(row rowScanner) (model.Transaction, error) {
	var t model.Transaction
	var status, transactedAt, createdAt, updatedAt string
	var postedAt sql.NullString
	if err := row.Scan(
		&t.ID, &t.AccountID, &t.Provider, &t.ProviderTransactionID, &status, &transactedAt,
		&postedAt, &t.AmountCents, &t.Currency, &t.Description, &t.RawPayload, &createdAt, &updatedAt,
	); err != nil {
		return model.Transaction{}, fmt.Errorf("store: scan transaction: %w", err)
	}
	t.Status = model.TxStatus(status)

	var err error
	if t.TransactedAt, err = parseTime(transactedAt); err != nil {
		return model.Transaction{}, fmt.Errorf("store: scan transaction: parse transacted_at: %w", err)
	}
	if t.PostedAt, err = parseNullTime(postedAt); err != nil {
		return model.Transaction{}, fmt.Errorf("store: scan transaction: parse posted_at: %w", err)
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Transaction{}, fmt.Errorf("store: scan transaction: parse created_at: %w", err)
	}
	if t.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Transaction{}, fmt.Errorf("store: scan transaction: parse updated_at: %w", err)
	}
	return t, nil
}
