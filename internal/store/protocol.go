// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"

	"github.com/clarkbar-sys/tally/internal/model"
)

// The interfaces below are the seam the app-protocol engine (S1, #95 initiative)
// writes through — the durable backing for app.js's persist()/persistTally()
// no-op stubs. They are declared here, alongside the concrete [Store] that
// implements them, so S1 can be written and tested against a named contract (and
// its own fakes) in parallel with the implementation, and so the surface stays
// small and reviewable in one place.
//
// The split is per substrate concern; [ProtocolStore] is the union an engine
// that touches everything depends on. Nothing forces a caller to use these — the
// methods exist on *Store directly — but a consumer that accepts the narrowest
// interface it needs stays honest about what it writes.

// AppStore persists the registered actors.
type AppStore interface {
	UpsertApp(ctx context.Context, a model.App) (model.App, error)
	GetApp(ctx context.Context, id string) (model.App, error)
	ListApps(ctx context.Context) ([]model.App, error)
}

// NotchStore persists notches and their append-only event timelines.
type NotchStore interface {
	UpsertNotch(ctx context.Context, n model.Notch) (model.Notch, error)
	GetNotch(ctx context.Context, id string) (model.Notch, error)
	ListNotches(ctx context.Context) ([]model.Notch, error)
	ListChildNotches(ctx context.Context, parentID string) ([]model.Notch, error)
	AppendNotchEvent(ctx context.Context, notchID string, ev model.Event) (model.Event, error)
	ListNotchEvents(ctx context.Context, notchID string) ([]model.Event, error)
}

// ProposalStore persists proposals ("tallies") and their append-only event
// timelines.
type ProposalStore interface {
	UpsertProposal(ctx context.Context, p model.Proposal) (model.Proposal, error)
	GetProposal(ctx context.Context, id string) (model.Proposal, error)
	ListProposals(ctx context.Context) ([]model.Proposal, error)
	ListProposalsForNotch(ctx context.Context, notchID string) ([]model.Proposal, error)
	AppendProposalEvent(ctx context.Context, proposalID string, ev model.Event) (model.Event, error)
	ListProposalEvents(ctx context.Context, proposalID string) ([]model.Event, error)
}

// RecordStore persists the append-only data substrate a merged proposal admits.
type RecordStore interface {
	InsertRecord(ctx context.Context, r model.Record) (model.Record, error)
	ListRecords(ctx context.Context) ([]model.Record, error)
	ListRecordsByDataset(ctx context.Context, dataset string) ([]model.Record, error)
	ListRecordsByProposal(ctx context.Context, proposalID string) ([]model.Record, error)
}

// ProtocolStore is the full app-protocol persistence surface — the union an
// engine that spans every substrate concern depends on.
type ProtocolStore interface {
	AppStore
	NotchStore
	ProposalStore
	RecordStore
}

// *Store satisfies every app-protocol interface.
var (
	_ AppStore      = (*Store)(nil)
	_ NotchStore    = (*Store)(nil)
	_ ProposalStore = (*Store)(nil)
	_ RecordStore   = (*Store)(nil)
	_ ProtocolStore = (*Store)(nil)
)
