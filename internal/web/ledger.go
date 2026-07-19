// SPDX-License-Identifier: GPL-2.0-or-later

package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
	"github.com/clarkbar-sys/tally/internal/store"
	"github.com/clarkbar-sys/tally/internal/version"
)

// LedgerView is the fully-resolved data the ledger page (#11, #12) renders:
// account cards with balances and staleness, and the transaction list, already
// formatted for display. Building it in Go keeps the templ template free of
// money/date logic.
type LedgerView struct {
	Accounts     []AccountCard
	Total        string
	AccountCount int

	Query        string
	Transactions []TxRow
	ResultCount  int
	Filtered     bool
}

// AccountCard is one account's balance summary (#12).
type AccountCard struct {
	Name      string
	Balance   string
	Negative  bool
	SyncLabel string
	Stale     bool
}

// TxRow is one transaction as shown in the list (#11).
type TxRow struct {
	Merchant string
	Meta     string
	Amount   string
	Credit   bool
	Pending  bool
}

// buildLedgerView assembles the view from the store. now is passed in (not read
// from the clock) so staleness rendering is testable; query filters the list by
// merchant or account, case-insensitively.
func buildLedgerView(ctx context.Context, st *store.Store, now time.Time, query string) (LedgerView, error) {
	accounts, err := st.ListAccounts(ctx)
	if err != nil {
		return LedgerView{}, err
	}
	txns, err := st.ListAllTransactions(ctx)
	if err != nil {
		return LedgerView{}, err
	}

	v := LedgerView{
		Query:        query,
		AccountCount: len(accounts),
	}

	var total int64
	names := make(map[int64]string, len(accounts))
	for _, a := range accounts {
		names[a.ID] = a.Name
		total += a.BalanceCents
		label, stale := syncLabel(a.LastSyncedAt, now)
		v.Accounts = append(v.Accounts, AccountCard{
			Name:      a.Name,
			Balance:   money(a.BalanceCents, false),
			Negative:  a.BalanceCents < 0,
			SyncLabel: label,
			Stale:     stale,
		})
	}
	v.Total = money(total, false)

	q := strings.ToLower(strings.TrimSpace(query))
	v.Filtered = q != ""
	for _, t := range txns {
		acctName := names[t.AccountID]
		if q != "" && !strings.Contains(strings.ToLower(t.Description), q) && !strings.Contains(strings.ToLower(acctName), q) {
			continue
		}
		v.Transactions = append(v.Transactions, TxRow{
			Merchant: t.Description,
			Meta:     t.TransactedAt.Format("Jan 2") + " · " + acctName,
			Amount:   money(t.AmountCents, true),
			Credit:   t.AmountCents > 0,
			Pending:  t.Status == model.StatusPending,
		})
	}
	v.ResultCount = len(v.Transactions)
	return v, nil
}

// The helpers below are small string builders the templ template calls, kept in
// Go so the template stays declarative.

func accountsAcross(n int) string {
	if n == 1 {
		return "across 1 account"
	}
	return fmt.Sprintf("across %d accounts", n)
}

func resultLabel(n int) string {
	if n == 1 {
		return "1 result"
	}
	return fmt.Sprintf("%d results", n)
}

func searchSummary(v LedgerView) string {
	return fmt.Sprintf("%s for %q", resultLabel(v.ResultCount), v.Query)
}

func emptySuffix(filtered bool) string {
	if filtered {
		return " match your search"
	}
	return " yet"
}

func versionString() string { return version.String() }

// money formats integer cents as currency. A negative amount always shows a
// leading U+2212 minus and "$"; a positive amount shows a leading "+" only when
// signed is true (transaction amounts), not for balances.
func money(cents int64, signed bool) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := group(cents / 100)
	body := fmt.Sprintf("%s.%02d", whole, cents%100)
	switch {
	case neg:
		return "−$" + body
	case signed:
		return "+$" + body
	default:
		return "$" + body
	}
}

// group inserts thousands separators into a non-negative integer.
func group(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
		if len(s) > lead {
			b.WriteByte(',')
		}
	}
	for i := lead; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// syncLabel renders how long ago an account was synced and whether that is
// stale enough to warn about. Stale financial data that looks current is worse
// than an obvious error (#12), so the threshold is deliberately tight: a source
// that updates roughly daily is flagged once it is two days behind.
func syncLabel(last, now time.Time) (label string, stale bool) {
	if last.IsZero() {
		return "never synced", true
	}
	d := now.Sub(last)
	switch {
	case d < 0:
		return "synced just now", false
	case d < time.Minute:
		return "synced just now", false
	case d < time.Hour:
		return fmt.Sprintf("synced %d min ago", int(d.Minutes())), false
	case d < 24*time.Hour:
		return fmt.Sprintf("synced %d hr ago", int(d.Hours())), false
	default:
		days := int(d.Hours()) / 24
		return fmt.Sprintf("stale — synced %d days ago", days), days >= 2
	}
}
