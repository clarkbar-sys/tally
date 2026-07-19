// SPDX-License-Identifier: GPL-2.0-or-later

// Package model defines tally's canonical account and transaction shapes.
//
// These types are provider-agnostic: a source adapter (#8) maps a provider's
// native response into an [Account] or [Transaction], and everything above the
// adapter boundary — sync (#10), browse (#11-#12), annotation (#13-#15) — deals
// only in these shapes.
//
// # Provider fields vs. tally-owned fields
//
// Per the datastore decision (docs/adr/0002-datastore.md), provider-sourced
// fields and tally-owned annotation are kept separate at the schema level. The
// types below hold only provider-sourced fields plus the identity needed to
// reference a row. Tags, notes, and story links (#13, #14, #15) live in their
// own tables that reference Transaction.ID by foreign key — a re-sync only ever
// writes to the columns modeled here, so it can never clobber annotation.
//
// # Identity and the merge rule
//
// A transaction is re-fetched over its lifetime and can change shape: it starts
// pending and later posts, and providers sometimes rewrite the description on
// posting. Annotation must survive that change, which means re-sync must update
// the *same* row rather than insert a new one.
//
// The identity key is (AccountID, ProviderTransactionID) — the pair a source
// adapter is expected to hold stable for a given real-world transaction across
// its pending-to-posted lifecycle. [Account] identity is (Provider,
// ProviderAccountID).
//
// On re-sync, a transaction (or account) matching that key is updated in place:
// every provider-sourced field is overwritten with the latest fetch, the
// internal ID is preserved, and UpdatedAt advances. A key that doesn't match any
// existing row is inserted as new. This upsert is implemented by
// internal/store's UpsertAccount and UpsertTransaction.
package model

import "time"

// TxStatus is a transaction's settlement state as reported by the provider.
type TxStatus string

const (
	StatusPending TxStatus = "pending"
	StatusPosted  TxStatus = "posted"
)

// Account is a financial account pulled from a source adapter (#8).
type Account struct {
	// ID is tally's internal, stable primary key. Annotation and transactions
	// reference this, never the provider ID.
	ID int64

	// Provider identifies the source adapter that produced this account (e.g.
	// "simplefin"). Provider + ProviderAccountID is the identity key.
	Provider          string
	ProviderAccountID string
	Institution       string
	Name              string
	Type              string
	Currency          string
	// BalanceCents is the account balance in integer minor units (cents) to
	// avoid floating-point drift on money.
	BalanceCents int64
	// LastSyncedAt is when this account was last successfully fetched from the
	// provider. Zero means never synced.
	LastSyncedAt time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Transaction is a single ledger entry pulled from a source adapter (#8).
type Transaction struct {
	// ID is tally's internal, stable primary key. Tags, notes, and story links
	// reference this, never the provider ID — see the package doc's merge rule.
	ID        int64
	AccountID int64

	// Provider identifies the source adapter that produced this transaction.
	// Provider + AccountID + ProviderTransactionID is the identity key used by
	// the upsert merge rule.
	Provider              string
	ProviderTransactionID string

	Status TxStatus
	// TransactedAt is when the transaction occurred (provider's authoritative
	// timestamp, present whether pending or posted). PostedAt is set once the
	// provider reports the transaction as posted; it is the zero value while
	// Status is StatusPending.
	TransactedAt time.Time
	PostedAt     time.Time

	// AmountCents is signed: negative for money out, positive for money in.
	AmountCents int64
	Currency    string
	Description string

	// RawPayload is the provider's raw response for this transaction, kept for
	// debugging and for remapping if the canonical model gains fields later. It
	// is provider-owned, like every other field on this type.
	RawPayload []byte

	CreatedAt time.Time
	UpdatedAt time.Time
}
