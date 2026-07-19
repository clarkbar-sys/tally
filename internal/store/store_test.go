// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tally.db")
	s, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenEnablesWAL(t *testing.T) {
	s := openTest(t)

	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tally.db")
	ctx := context.Background()

	s1, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s1.Close()

	// Reopening an already-migrated database must not error or reapply
	// migrations.
	s2, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	var count int
	if err := s2.db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("schema_migrations rows = %d, want 1", count)
	}
}

func testAccount() model.Account {
	return model.Account{
		Provider:          "simplefin",
		ProviderAccountID: "acct-001",
		Institution:       "Test Credit Union",
		Name:              "Checking",
		Type:              "checking",
		Currency:          "USD",
		BalanceCents:      123_45,
		LastSyncedAt:      time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC),
	}
}

func TestUpsertAccountInsertsThenUpdatesInPlace(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	inserted, err := s.UpsertAccount(ctx, testAccount())
	if err != nil {
		t.Fatalf("UpsertAccount (insert): %v", err)
	}
	if inserted.ID == 0 {
		t.Fatal("inserted account has zero ID")
	}
	if inserted.CreatedAt.IsZero() || inserted.UpdatedAt.IsZero() {
		t.Fatal("inserted account missing timestamps")
	}

	// A re-sync with the same provider identity but changed provider fields
	// (balance moved, name relabeled by the bank) must update the same row,
	// not create a second one.
	changed := testAccount()
	changed.BalanceCents = 456_78
	changed.Name = "Checking (renamed)"
	updated, err := s.UpsertAccount(ctx, changed)
	if err != nil {
		t.Fatalf("UpsertAccount (update): %v", err)
	}

	if updated.ID != inserted.ID {
		t.Fatalf("update changed ID: got %d, want %d", updated.ID, inserted.ID)
	}
	if updated.BalanceCents != 456_78 {
		t.Fatalf("BalanceCents = %d, want %d", updated.BalanceCents, 456_78)
	}
	if updated.Name != "Checking (renamed)" {
		t.Fatalf("Name = %q, want %q", updated.Name, "Checking (renamed)")
	}
	if !updated.CreatedAt.Equal(inserted.CreatedAt) {
		t.Fatalf("CreatedAt changed on update: got %v, want %v", updated.CreatedAt, inserted.CreatedAt)
	}

	accounts, err := s.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("ListAccounts returned %d accounts, want 1 (re-sync must not duplicate)", len(accounts))
	}
}

func testTransaction(accountID int64) model.Transaction {
	return model.Transaction{
		AccountID:             accountID,
		Provider:              "simplefin",
		ProviderTransactionID: "txn-001",
		Status:                model.StatusPending,
		TransactedAt:          time.Date(2026, 7, 18, 9, 30, 0, 0, time.UTC),
		AmountCents:           -4599,
		Currency:              "USD",
		Description:           "COFFEE SHOP",
		RawPayload:            []byte(`{"pending":true}`),
	}
}

func TestUpsertTransactionPendingToPostedPreservesIdentity(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	acct, err := s.UpsertAccount(ctx, testAccount())
	if err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}

	pending, err := s.UpsertTransaction(ctx, testTransaction(acct.ID))
	if err != nil {
		t.Fatalf("UpsertTransaction (pending): %v", err)
	}
	if pending.ID == 0 {
		t.Fatal("pending transaction has zero ID")
	}
	if pending.Status != model.StatusPending {
		t.Fatalf("Status = %q, want %q", pending.Status, model.StatusPending)
	}
	if !pending.PostedAt.IsZero() {
		t.Fatalf("PostedAt = %v, want zero for a pending transaction", pending.PostedAt)
	}

	// Same provider identity, now posted with a provider-rewritten
	// description — must land on the same internal row so that any
	// annotation keyed on pending.ID (tags, notes, stories — future issues)
	// stays attached.
	posted := testTransaction(acct.ID)
	posted.Status = model.StatusPosted
	posted.PostedAt = time.Date(2026, 7, 19, 3, 0, 0, 0, time.UTC)
	posted.Description = "COFFEE SHOP #4521"
	posted.RawPayload = []byte(`{"pending":false}`)

	updated, err := s.UpsertTransaction(ctx, posted)
	if err != nil {
		t.Fatalf("UpsertTransaction (posted): %v", err)
	}

	if updated.ID != pending.ID {
		t.Fatalf("posting changed ID: got %d, want %d", updated.ID, pending.ID)
	}
	if updated.Status != model.StatusPosted {
		t.Fatalf("Status = %q, want %q", updated.Status, model.StatusPosted)
	}
	if updated.PostedAt.IsZero() {
		t.Fatal("PostedAt still zero after posting")
	}
	if updated.Description != "COFFEE SHOP #4521" {
		t.Fatalf("Description = %q, want %q", updated.Description, "COFFEE SHOP #4521")
	}

	txns, err := s.ListTransactionsByAccount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("ListTransactionsByAccount: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("ListTransactionsByAccount returned %d transactions, want 1 (pending->posted must not duplicate)", len(txns))
	}
}

func TestGetAccountAndGetTransaction(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	acct, err := s.UpsertAccount(ctx, testAccount())
	if err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}
	txn, err := s.UpsertTransaction(ctx, testTransaction(acct.ID))
	if err != nil {
		t.Fatalf("UpsertTransaction: %v", err)
	}

	gotAcct, err := s.GetAccount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if gotAcct != acct {
		t.Fatalf("GetAccount = %+v, want %+v", gotAcct, acct)
	}

	gotTxn, err := s.GetTransaction(ctx, txn.ID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if gotTxn.ID != txn.ID || gotTxn.Description != txn.Description {
		t.Fatalf("GetTransaction = %+v, want %+v", gotTxn, txn)
	}
}

func TestTransactionAccountForeignKeyEnforced(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	_, err := s.UpsertTransaction(ctx, testTransaction(9999))
	if err == nil {
		t.Fatal("UpsertTransaction with a nonexistent account_id should fail foreign key check")
	}
}
