// SPDX-License-Identifier: GPL-2.0-or-later

package ingest_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/clarkbar-sys/tally/internal/ingest"
	"github.com/clarkbar-sys/tally/internal/source/demo"
	"github.com/clarkbar-sys/tally/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tally.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestApplyPopulatesStore(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)

	res, err := ingest.Apply(ctx, st, demo.New(), time.Time{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Accounts == 0 || res.Transactions == 0 {
		t.Fatalf("Apply reported nothing ingested: %+v", res)
	}

	accts, err := st.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accts) != res.Accounts {
		t.Fatalf("stored %d accounts, Apply reported %d", len(accts), res.Accounts)
	}
	for _, a := range accts {
		if a.LastSyncedAt.IsZero() {
			t.Errorf("account %q missing LastSyncedAt after sync", a.Name)
		}
	}

	txns, err := st.ListAllTransactions(ctx)
	if err != nil {
		t.Fatalf("ListAllTransactions: %v", err)
	}
	if len(txns) != res.Transactions {
		t.Fatalf("stored %d transactions, Apply reported %d", len(txns), res.Transactions)
	}
	// Every transaction must carry a real internal account ID assigned by the
	// upsert — the linkage the adapter could not provide.
	byID := map[int64]bool{}
	for _, a := range accts {
		byID[a.ID] = true
	}
	for _, tx := range txns {
		if !byID[tx.AccountID] {
			t.Fatalf("transaction %d references unknown account %d", tx.ID, tx.AccountID)
		}
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	a := demo.New()

	first, err := ingest.Apply(ctx, st, a, time.Time{})
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	second, err := ingest.Apply(ctx, st, a, time.Time{})
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if first != second {
		t.Fatalf("Apply not idempotent in counts: first %+v, second %+v", first, second)
	}

	accts, _ := st.ListAccounts(ctx)
	txns, _ := st.ListAllTransactions(ctx)
	if len(accts) != first.Accounts {
		t.Fatalf("re-sync duplicated accounts: have %d, want %d", len(accts), first.Accounts)
	}
	if len(txns) != first.Transactions {
		t.Fatalf("re-sync duplicated transactions: have %d, want %d", len(txns), first.Transactions)
	}
}
