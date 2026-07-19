// SPDX-License-Identifier: GPL-2.0-or-later

// Package ingest applies a source adapter's snapshot to the store: the
// upsert-and-merge core of the sync job (#10). It fetches from an adapter and
// writes the result through the store's upserts, which are keyed on provider
// identity so a re-run updates existing rows in place and never duplicates or
// clobbers tally-owned annotation (see internal/model's merge rule).
//
// This is the apply half of #10. The scheduling, incremental date windows, and
// sync log described there build on top of [Apply]; the idempotent write is
// modeled and tested here so those can layer on without re-litigating
// correctness.
package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/clarkbar-sys/tally/internal/source"
	"github.com/clarkbar-sys/tally/internal/store"
)

// Result reports what an [Apply] run touched.
type Result struct {
	Accounts     int
	Transactions int
}

// Apply fetches transactions transacted at or after since from a, then upserts
// every account and transaction into st. A zero since requests full history.
// It is idempotent: running it twice over the same provider state leaves the
// store unchanged past the first run, because the store upserts on provider
// identity.
//
// Each account is upserted first so its internal ID is known, then its
// transactions are upserted with AccountID set to it — resolving the linkage
// the adapter could not (see internal/source). LastSyncedAt is stamped to the
// wall clock at apply time so the browse view can surface staleness (#12).
func Apply(ctx context.Context, st *store.Store, a source.Adapter, since time.Time) (Result, error) {
	snap, err := a.Fetch(ctx, since)
	if err != nil {
		return Result{}, fmt.Errorf("ingest: fetch from %q: %w", a.Name(), err)
	}

	now := time.Now().UTC()
	var res Result
	for _, as := range snap.Accounts {
		acct := as.Account
		acct.Provider = a.Name()
		acct.LastSyncedAt = now

		stored, err := st.UpsertAccount(ctx, acct)
		if err != nil {
			return res, fmt.Errorf("ingest: upsert account %q: %w", acct.ProviderAccountID, err)
		}
		res.Accounts++

		for _, txn := range as.Transactions {
			txn.AccountID = stored.ID
			txn.Provider = a.Name()
			if _, err := st.UpsertTransaction(ctx, txn); err != nil {
				return res, fmt.Errorf("ingest: upsert transaction %q: %w", txn.ProviderTransactionID, err)
			}
			res.Transactions++
		}
	}
	return res, nil
}
