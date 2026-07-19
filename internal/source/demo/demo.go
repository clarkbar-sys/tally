// SPDX-License-Identifier: GPL-2.0-or-later

// Package demo is a source adapter (#8) that fabricates believable accounts and
// transactions with no credentials and no network. It exists so tally can be
// deployed and browsed end to end — a populated app at tally.<tailnet>.ts.net —
// before the SimpleFIN adapter (#9) and real provider access are wired up.
//
// The data is deterministic: the same fetch window always yields the same
// accounts and the same transaction identities, so re-syncing is idempotent
// under the merge rule (internal/model) exactly as a real provider must be.
// Only the anchor date floats — transactions are dated relative to "now" so the
// list always looks current — which a re-sync harmlessly folds into the same
// rows via the stable provider transaction IDs.
//
// Blank-import this package to register it:
//
//	import _ "github.com/clarkbar-sys/tally/internal/source/demo"
//
// It registers under the name "demo".
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
	"github.com/clarkbar-sys/tally/internal/source"
)

// Name is the provider key this adapter registers and stamps onto its rows.
const Name = "demo"

func init() {
	source.Register(Name, func(context.Context) (source.Adapter, error) {
		return New(), nil
	})
}

// Adapter is the demo source adapter. Construct it with [New].
type Adapter struct {
	// now supplies the anchor date transactions are dated relative to. It is a
	// field so tests can pin it and assert deterministic output; production uses
	// time.Now.
	now func() time.Time
}

// New returns a demo adapter anchored to the wall clock.
func New() *Adapter {
	return &Adapter{now: time.Now}
}

// Name reports the provider key. It satisfies [source.Adapter].
func (a *Adapter) Name() string { return Name }

// account is a static description of one fabricated account; balances and
// transactions are generated per fetch.
type account struct {
	providerID  string
	institution string
	name        string
	typ         string
}

var accounts = []account{
	{providerID: "demo-chk-001", institution: "Tailleaf Credit Union", name: "Everyday Checking", typ: "checking"},
	{providerID: "demo-sav-002", institution: "Tailleaf Credit Union", name: "Rainy Day Savings", typ: "savings"},
	{providerID: "demo-cc-003", institution: "Northwind Card", name: "Northwind Rewards Visa", typ: "credit"},
}

// entry is one fabricated transaction template, resolved to a real date and
// identity at fetch time. amountCents is signed: negative money out, positive
// money in. daysAgo is the transaction's age relative to the anchor date;
// entries with a smaller daysAgo than pendingWithinDays are reported pending.
type entry struct {
	acctIndex   int
	daysAgo     int
	amountCents int64
	description string
}

// pendingWithinDays: transactions dated within this many days of the anchor are
// reported as still pending, older ones as posted — mirroring how a real
// provider settles recent activity.
const pendingWithinDays = 2

// entries is a curated, deterministic activity log across the demo accounts,
// ordered oldest-first. Amounts and cadence are chosen to look like real
// spending (recurring bills, groceries, the occasional deposit) so the browse
// and annotation UIs have something honest to render.
var entries = []entry{
	{acctIndex: 0, daysAgo: 58, amountCents: 3_200_00, description: "PAYROLL DIRECT DEPOSIT"},
	{acctIndex: 0, daysAgo: 57, amountCents: -1_450_00, description: "SUNRISE PROPERTY MGMT RENT"},
	{acctIndex: 0, daysAgo: 55, amountCents: -84_23, description: "GREEN VALLEY GROCERS"},
	{acctIndex: 2, daysAgo: 54, amountCents: -62_10, description: "NORTHWIND GAS #221"},
	{acctIndex: 0, daysAgo: 52, amountCents: -19_99, description: "STREAMFLIX MONTHLY"},
	{acctIndex: 1, daysAgo: 50, amountCents: 500_00, description: "TRANSFER FROM CHECKING"},
	{acctIndex: 2, daysAgo: 48, amountCents: -142_87, description: "HOME DEPOT #4471"},
	{acctIndex: 0, daysAgo: 47, amountCents: -46_50, description: "CORNER BISTRO"},
	{acctIndex: 2, daysAgo: 45, amountCents: -31_44, description: "GREEN VALLEY GROCERS"},
	{acctIndex: 0, daysAgo: 44, amountCents: 3_200_00, description: "PAYROLL DIRECT DEPOSIT"},
	{acctIndex: 0, daysAgo: 43, amountCents: -1_450_00, description: "SUNRISE PROPERTY MGMT RENT"},
	{acctIndex: 2, daysAgo: 41, amountCents: -217_65, description: "CITY POWER & LIGHT"},
	{acctIndex: 0, daysAgo: 39, amountCents: -58_12, description: "GREEN VALLEY GROCERS"},
	{acctIndex: 2, daysAgo: 37, amountCents: -12_99, description: "CLOUDTUNES SUBSCRIPTION"},
	{acctIndex: 0, daysAgo: 35, amountCents: -73_40, description: "THE HARDWARE STORE"},
	{acctIndex: 1, daysAgo: 33, amountCents: 12_88, description: "INTEREST PAYMENT"},
	{acctIndex: 2, daysAgo: 31, amountCents: -95_02, description: "NORTHWIND GAS #221"},
	{acctIndex: 0, daysAgo: 30, amountCents: 3_200_00, description: "PAYROLL DIRECT DEPOSIT"},
	{acctIndex: 0, daysAgo: 29, amountCents: -1_450_00, description: "SUNRISE PROPERTY MGMT RENT"},
	{acctIndex: 0, daysAgo: 27, amountCents: -88_76, description: "GREEN VALLEY GROCERS"},
	{acctIndex: 2, daysAgo: 25, amountCents: -410_00, description: "AUTOCARE COMPLETE SERVICE"},
	{acctIndex: 0, daysAgo: 23, amountCents: -19_99, description: "STREAMFLIX MONTHLY"},
	{acctIndex: 2, daysAgo: 21, amountCents: -54_31, description: "CORNER BISTRO"},
	{acctIndex: 0, daysAgo: 19, amountCents: -67_90, description: "GREEN VALLEY GROCERS"},
	{acctIndex: 1, daysAgo: 18, amountCents: 500_00, description: "TRANSFER FROM CHECKING"},
	{acctIndex: 2, daysAgo: 16, amountCents: -128_44, description: "PARKSIDE PHARMACY"},
	{acctIndex: 0, daysAgo: 15, amountCents: 3_200_00, description: "PAYROLL DIRECT DEPOSIT"},
	{acctIndex: 0, daysAgo: 14, amountCents: -1_450_00, description: "SUNRISE PROPERTY MGMT RENT"},
	{acctIndex: 2, daysAgo: 12, amountCents: -39_18, description: "NORTHWIND GAS #221"},
	{acctIndex: 0, daysAgo: 10, amountCents: -102_57, description: "GREEN VALLEY GROCERS"},
	{acctIndex: 2, daysAgo: 8, amountCents: -22_50, description: "CORNER BISTRO"},
	{acctIndex: 0, daysAgo: 6, amountCents: -14_99, description: "CLOUDTUNES SUBSCRIPTION"},
	{acctIndex: 2, daysAgo: 4, amountCents: -76_83, description: "THE HARDWARE STORE"},
	{acctIndex: 0, daysAgo: 3, amountCents: -48_02, description: "GREEN VALLEY GROCERS"},
	{acctIndex: 2, daysAgo: 1, amountCents: -33_71, description: "CORNER BISTRO"},
	{acctIndex: 0, daysAgo: 0, amountCents: -9_25, description: "DAYBREAK COFFEE"},
}

const currency = "USD"

// Fetch fabricates the demo provider's accounts and their transactions dated at
// or after since. It satisfies [source.Adapter].
func (a *Adapter) Fetch(_ context.Context, since time.Time) (*source.Snapshot, error) {
	anchor := a.now().UTC()
	// Anchor to the start of the day so a transaction's date is stable across
	// fetches within the same day, keeping intra-day re-syncs fully idempotent.
	day := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)

	// One AccountSnapshot per account, in the fixed account order. Balance is
	// derived from the account's transactions so it reconciles with the list.
	snaps := make([]source.AccountSnapshot, len(accounts))
	for i, acct := range accounts {
		snaps[i] = source.AccountSnapshot{
			Account: model.Account{
				Provider:          Name,
				ProviderAccountID: acct.providerID,
				Institution:       acct.institution,
				Name:              acct.name,
				Type:              acct.typ,
				Currency:          currency,
			},
		}
	}

	for i, e := range entries {
		txnTime := day.AddDate(0, 0, -e.daysAgo).Add(time.Duration(i%24) * time.Hour)
		if !since.IsZero() && txnTime.Before(since) {
			continue
		}

		status := model.StatusPosted
		var postedAt time.Time
		if e.daysAgo < pendingWithinDays {
			status = model.StatusPending
		} else {
			// Posted a day after it was transacted, as most providers report.
			postedAt = txnTime.AddDate(0, 0, 1)
		}

		txn := model.Transaction{
			Provider:              Name,
			ProviderTransactionID: fmt.Sprintf("demo-txn-%04d", i),
			Status:                status,
			TransactedAt:          txnTime,
			PostedAt:              postedAt,
			AmountCents:           e.amountCents,
			Currency:              currency,
			Description:           e.description,
			RawPayload:            rawPayload(i, e, status, txnTime),
		}
		snaps[e.acctIndex].Transactions = append(snaps[e.acctIndex].Transactions, txn)
	}

	// Balance reflects the full curated history regardless of the fetch window,
	// like a provider's authoritative current balance — it is not a running sum
	// of only the returned rows.
	for i := range snaps {
		snaps[i].Account.BalanceCents = balanceFor(i, day)
	}

	return &source.Snapshot{Accounts: snaps}, nil
}

// balanceFor sums every curated entry for the account at accountIndex into a
// current balance. Credit accounts (money owed) are reported as a negative
// balance, matching how the ledger models a card you carry.
func balanceFor(accountIndex int, _ time.Time) int64 {
	var sum int64
	for _, e := range entries {
		if e.acctIndex == accountIndex {
			sum += e.amountCents
		}
	}
	return sum
}

func rawPayload(index int, e entry, status model.TxStatus, txnTime time.Time) []byte {
	// A small, stable JSON blob so the RawPayload column is exercised end to end
	// (debugging, future remapping) the way a real adapter's would be.
	b, err := json.Marshal(map[string]any{
		"source":       Name,
		"seq":          index,
		"status":       string(status),
		"transacted":   txnTime.Format(time.RFC3339),
		"amount_cents": e.amountCents,
		"description":  e.description,
	})
	if err != nil {
		// The map is fixed and always marshals; a failure here is impossible in
		// practice, but never panic in an adapter — degrade to no payload.
		return nil
	}
	return b
}
