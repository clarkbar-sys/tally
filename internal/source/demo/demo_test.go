// SPDX-License-Identifier: GPL-2.0-or-later

package demo

import (
	"context"
	"testing"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
	"github.com/clarkbar-sys/tally/internal/source"
)

// pinned returns a demo adapter whose anchor date is fixed, so its output is
// fully deterministic for assertions.
func pinned(t *testing.T) *Adapter {
	t.Helper()
	a := New()
	a.now = func() time.Time { return time.Date(2026, 7, 19, 15, 30, 0, 0, time.UTC) }
	return a
}

func TestRegisteredOnDefaultRegistry(t *testing.T) {
	// The blank-import contract: importing this package registers "demo".
	if _, err := source.Open(context.Background(), Name); err != nil {
		t.Fatalf("Open(%q) on default registry: %v", Name, err)
	}
}

func TestFetchProducesEveryAccountAndCanonicalRows(t *testing.T) {
	snap, err := pinned(t).Fetch(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(snap.Accounts) != len(accounts) {
		t.Fatalf("got %d accounts, want %d", len(snap.Accounts), len(accounts))
	}

	var total int
	for _, as := range snap.Accounts {
		if as.Account.Provider != Name {
			t.Errorf("account provider = %q, want %q", as.Account.Provider, Name)
		}
		if as.Account.ProviderAccountID == "" || as.Account.Currency != currency {
			t.Errorf("account not canonicalised: %+v", as.Account)
		}
		// Adapters fill provider fields only; internal identity stays zero for
		// the sync job to assign.
		if as.Account.ID != 0 || !as.Account.CreatedAt.IsZero() {
			t.Errorf("account carries internal identity it should not: %+v", as.Account)
		}
		for _, txn := range as.Transactions {
			total++
			if txn.Provider != Name || txn.ProviderTransactionID == "" {
				t.Errorf("transaction not canonicalised: %+v", txn)
			}
			if txn.AccountID != 0 || txn.ID != 0 {
				t.Errorf("transaction carries internal identity it should not: %+v", txn)
			}
			if txn.Status == model.StatusPosted && txn.PostedAt.IsZero() {
				t.Errorf("posted transaction missing PostedAt: %+v", txn)
			}
			if txn.Status == model.StatusPending && !txn.PostedAt.IsZero() {
				t.Errorf("pending transaction has PostedAt set: %+v", txn)
			}
			if len(txn.RawPayload) == 0 {
				t.Errorf("transaction missing RawPayload: %+v", txn)
			}
		}
	}
	if total != len(entries) {
		t.Fatalf("distributed %d transactions, want all %d", total, len(entries))
	}
}

func TestFetchIsDeterministic(t *testing.T) {
	ctx := context.Background()
	first, err := pinned(t).Fetch(ctx, time.Time{})
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	second, err := pinned(t).Fetch(ctx, time.Time{})
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	for i := range first.Accounts {
		fa, sa := first.Accounts[i], second.Accounts[i]
		if fa.Account != sa.Account {
			t.Fatalf("account %d differs between fetches:\n %+v\n %+v", i, fa.Account, sa.Account)
		}
		if len(fa.Transactions) != len(sa.Transactions) {
			t.Fatalf("account %d transaction count differs: %d vs %d", i, len(fa.Transactions), len(sa.Transactions))
		}
		for j := range fa.Transactions {
			if fa.Transactions[j].ProviderTransactionID != sa.Transactions[j].ProviderTransactionID ||
				fa.Transactions[j].AmountCents != sa.Transactions[j].AmountCents ||
				!fa.Transactions[j].TransactedAt.Equal(sa.Transactions[j].TransactedAt) {
				t.Fatalf("account %d txn %d differs between fetches", i, j)
			}
		}
	}
}

func TestFetchSinceFiltersOldTransactions(t *testing.T) {
	ctx := context.Background()
	a := pinned(t)

	all, err := a.Fetch(ctx, time.Time{})
	if err != nil {
		t.Fatalf("Fetch(zero): %v", err)
	}
	// 20 days before the pinned anchor: a strict subset of history.
	since := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	recent, err := a.Fetch(ctx, since)
	if err != nil {
		t.Fatalf("Fetch(since): %v", err)
	}

	if countTxns(recent) == 0 {
		t.Fatal("windowed fetch returned no transactions")
	}
	if countTxns(recent) >= countTxns(all) {
		t.Fatalf("windowed fetch (%d) should return fewer than full history (%d)", countTxns(recent), countTxns(all))
	}
	for _, as := range recent.Accounts {
		for _, txn := range as.Transactions {
			if txn.TransactedAt.Before(since) {
				t.Fatalf("transaction dated %v is before since %v", txn.TransactedAt, since)
			}
		}
	}
	// Balances are the provider's current figure, independent of the window, so
	// they must match across a full and a windowed fetch.
	for i := range all.Accounts {
		if all.Accounts[i].Account.BalanceCents != recent.Accounts[i].Account.BalanceCents {
			t.Fatalf("account %d balance changed with the fetch window", i)
		}
	}
}

func countTxns(s *source.Snapshot) int {
	var n int
	for _, as := range s.Accounts {
		n += len(as.Transactions)
	}
	return n
}
