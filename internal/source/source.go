// SPDX-License-Identifier: GPL-2.0-or-later

// Package source defines tally's source-adapter boundary: one interface every
// financial data provider implements, and a registry that lets a provider be
// added by dropping in a new package plus a single registration line (#8).
//
// Everything above this boundary — the sync job (#10), browse (#11-#12),
// annotation (#13-#15) — deals only in the canonical model (internal/model).
// Provider-specific auth, endpoints, and response shapes live entirely inside an
// adapter; a credential never crosses the boundary. An adapter's only job is to
// fetch from its provider and normalise the result into [model.Account] and
// [model.Transaction] values.
//
// # What an adapter returns
//
// An adapter fills only provider-sourced fields. Internal identity and
// timestamps — [model.Account.ID], [model.Transaction.ID]/AccountID, and the
// CreatedAt/UpdatedAt columns — are left zero: the sync job assigns them when it
// upserts the snapshot under the identity and merge rule documented in
// internal/model. Transactions come grouped under their provider account because
// [model.Transaction.AccountID] is an internal ID that cannot exist until that
// account is upserted; the grouping carries the provider→account linkage across
// the boundary so sync can resolve it.
//
// # One Fetch, not listAccounts + listTransactions
//
// #8 sketched separate listAccounts / listTransactions(since) calls. They are
// collapsed into a single [Adapter.Fetch] here for two reasons: providers (the
// v1 target, SimpleFIN) report accounts and their transactions together in one
// response, and the account grouping above is required regardless. One call that
// returns a [Snapshot] is the honest shape; splitting it would force an adapter
// to either round-trip twice or cache internally for no caller benefit.
//
// # Adding a provider
//
// Implement [Adapter] in a new package, register a [Factory] for it (one line,
// typically in an init), and blank-import that package where adapters are wired.
// Nothing above the boundary changes. See internal/source/demo for a worked
// example, and TestSecondProviderIsAdditive for the acceptance criterion.
package source

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
)

// Adapter is one financial data provider behind tally's canonical model. All
// provider-specific concerns — auth, endpoints, pagination, response shape —
// live inside the implementation; callers see only the model.
type Adapter interface {
	// Name is the provider key stamped onto every account and transaction this
	// adapter produces ([model.Account.Provider] / [model.Transaction.Provider]).
	// It matches the name the adapter is registered under and is part of the
	// model identity key, so it must be stable for the life of the provider.
	Name() string

	// Fetch returns the provider's current accounts, each grouped with the
	// transactions transacted at or after since. A zero since requests full
	// history. Only provider-sourced fields are populated; internal IDs and
	// timestamps are left for the sync job to assign on upsert (see the package
	// doc). since is advisory — an adapter may return transactions slightly
	// older than since (overlap to catch late-posting rows); the sync job's
	// upsert makes that harmless.
	Fetch(ctx context.Context, since time.Time) (*Snapshot, error)
}

// Snapshot is one adapter's normalised view of a provider at a point in time.
type Snapshot struct {
	// Accounts is every account the provider currently reports, each paired with
	// its transactions in the fetch window.
	Accounts []AccountSnapshot
}

// AccountSnapshot pairs a provider account with the transactions the provider
// reported for it. The pairing is the provider→account linkage the sync job
// needs: it upserts Account, learns the internal ID, then upserts each
// Transaction with AccountID set to it.
type AccountSnapshot struct {
	Account      model.Account
	Transactions []model.Transaction
}

// Factory builds a ready-to-use [Adapter], resolving the provider's own
// configuration and credentials internally — typically from the environment or
// a systemd credential, never from a committed file or a flag (see
// docs/security). It may perform I/O (a token exchange, a config read), so it
// takes a context and can fail.
type Factory func(ctx context.Context) (Adapter, error)

// Registry maps provider names to their factories. The zero value is not
// usable; construct one with [NewRegistry]. A [Registry] is safe for concurrent
// use.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty registry. Most code uses the package-level
// [Register]/[Open]/[Names] against the default registry; a distinct Registry
// is useful in tests that must not touch global state.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register records factory under name. It returns an error if name is empty,
// factory is nil, or name is already registered — re-registering a provider is
// a programming error, not a runtime condition.
func (r *Registry) Register(name string, factory Factory) error {
	if name == "" {
		return fmt.Errorf("source: register: empty provider name")
	}
	if factory == nil {
		return fmt.Errorf("source: register %q: nil factory", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.factories[name]; dup {
		return fmt.Errorf("source: register %q: already registered", name)
	}
	r.factories[name] = factory
	return nil
}

// Open builds the adapter registered under name. It returns an error if no
// provider is registered under name, or if the factory itself fails.
func (r *Registry) Open(ctx context.Context, name string) (Adapter, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("source: open %q: no such provider (have %v)", name, r.Names())
	}
	a, err := factory(ctx)
	if err != nil {
		return nil, fmt.Errorf("source: open %q: %w", name, err)
	}
	return a, nil
}

// Names returns the registered provider names in sorted order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// defaultRegistry backs the package-level Register/Open/Names. Adapter packages
// register onto it from an init, so a blank import is all it takes to make a
// provider available.
var defaultRegistry = NewRegistry()

// Register records factory under name on the default registry, panicking on
// error. It is meant to be called from an adapter package's init, where a
// duplicate or malformed registration is a build-time-fixable bug that should
// fail loudly rather than be swallowed (this mirrors database/sql.Register).
func Register(name string, factory Factory) {
	if err := defaultRegistry.Register(name, factory); err != nil {
		panic(err)
	}
}

// Open builds the adapter registered under name on the default registry.
func Open(ctx context.Context, name string) (Adapter, error) {
	return defaultRegistry.Open(ctx, name)
}

// Names returns the provider names registered on the default registry, sorted.
func Names() []string {
	return defaultRegistry.Names()
}
