// SPDX-License-Identifier: GPL-2.0-or-later

package model

import (
	"encoding/json"
	"time"
)

// App-protocol shapes (initiative #95). These model the app→proposal→merge loop
// that today runs entirely in-memory in internal/web/static/app.js: a registered
// actor (an [App]) authors a [Proposal] (app.js calls it a "tally") whose typed
// diff, once merged, writes to tally's substrate — [Notch]es and [Record]s. The
// field shapes follow app.js so the eventual client swap (S8) is mechanical.
//
// # Engine-owned payloads
//
// A proposal's diff (Changes), an event's kind-specific fields (Event.Payload),
// an app's scopes/action, and a notch's tags are polymorphic and evolve with the
// change vocabulary. The store persists them as opaque JSON rather than a
// per-op relational schema — the engine (S1) owns their semantics, the store
// just round-trips them. Changes and Event.Payload are therefore
// [json.RawMessage]: the store never inspects them.
//
// # Timestamps
//
// Like [Account]/[Transaction], the creation and update timestamps below are
// owned by the store: it stamps them on write and populates them on read, so
// the fields are output-only on the types passed in. See internal/store.

// ProposalStatus is a proposal's position in the review lifecycle.
type ProposalStatus string

const (
	ProposalOpen     ProposalStatus = "open"
	ProposalMerged   ProposalStatus = "merged"
	ProposalDeclined ProposalStatus = "declined"
)

// NotchStatus is a notch's open/closed state. A notch is never deleted — it is
// closed as done or not planned (and can be reopened), like a GitHub issue.
type NotchStatus string

const (
	NotchOpen       NotchStatus = "open"
	NotchDone       NotchStatus = "done"
	NotchNotPlanned NotchStatus = "not_planned"
)

// AppStatus is whether a registered app may still act. Revocation is
// forward-looking: a revoked app can no longer author proposals, but everything
// it already proposed stays on the record.
type AppStatus string

const (
	AppActive  AppStatus = "active"
	AppRevoked AppStatus = "revoked"
)

// Record kinds. A text record carries a Summary; a blob record carries
// Name/Mime/Size and a BlobURL to its bytes.
const (
	RecordText = "text"
	RecordBlob = "blob"
)

// AppAction is an app's one live, user-triggerable verb (its button in the Apps
// view). Nil when the app has no action to offer.
type AppAction struct {
	Label string `json:"label"`
	Verb  string `json:"verb"`
}

// App is a registered non-human actor — a provider, a local helper, or the
// built-in "you". Its one write verb is authoring a [Proposal], so every write
// still passes the merge gate. Reading is the permission surface: an app holds
// typed Scopes ("resource:verb", e.g. "records:propose") and may only act within
// them. Matches pact's app.schema.json.
type App struct {
	// ID is the app's stable identifier (e.g. "you", "spotify-demo"). Proposals
	// and records reference an app by this ID for provenance.
	ID    string
	Name  string
	Kind  string // "you" | "connected" | "local"
	Color string
	Blurb string
	// Scopes are the typed permissions the app holds, each "resource:verb"
	// (notches/records × read/propose). There is no "write" verb — writes are
	// always proposals.
	Scopes []string
	Action *AppAction
	Status AppStatus

	// InstalledAt is when the app was registered. Store-owned (see package doc).
	InstalledAt time.Time
}

// Proposal is the PR-like reviewable object app.js calls a "tally": a typed
// batch of Changes plus a set of LinkedNotches. Merging it applies the changes
// to the substrate and closes the linked notches; the user, not the author,
// decides — that is the consent gate. Its timeline lives in a separate
// append-only event log ([Event]).
type Proposal struct {
	ID string
	// AppID is the authoring app's ID. Every proposal has an author; a
	// hand-opened one is authored by "you".
	AppID  string
	Title  string
	Body   string
	Status ProposalStatus

	// Changes is the proposal's diff — a JSON array of typed change ops, opaque
	// to the store and owned by the engine (S1). See package doc.
	Changes json.RawMessage
	// LinkedNotches are the IDs of notches this proposal closes on merge.
	LinkedNotches []string

	// CreatedAt/UpdatedAt are store-owned. MergedAt is set by the caller on
	// merge and is the zero value until then.
	CreatedAt time.Time
	UpdatedAt time.Time
	MergedAt  time.Time
}

// Event is one entry on an append-only timeline — a proposal's or a notch's. The
// log is never rewritten: a "deleted" comment is only flagged in its payload,
// never removed. Kind-specific fields ride in Payload, opaque to the store.
type Event struct {
	// ID is the caller-assigned event ID (e.g. "e_..."), preserved as written.
	ID   string
	Kind string
	// At is when the event was recorded. Store-owned: stamped on append.
	At      time.Time
	Payload json.RawMessage
}

// Label is one entry in the global label registry. Labels are not owned by any
// one notch: a notch's Tags are label names, joined to this registry by name, so
// coloring "bug" once colors it everywhere it appears. Color is the auto-assigned
// palette swatch (a theme-aware CSS name); Bg/Fg are set only once a color picker
// is touched and then override the palette with a fixed hex pair. They are
// pointers so an untouched label round-trips as JSON null, not "".
type Label struct {
	Name  string
	Color string
	Bg    *string
	Fg    *string
}

// Notch is the generic issue-like container that is tally's substrate: a title,
// a Markdown body (tasks live inline as `- [ ]` lists), tags, and a parent for
// nesting. Its timeline is a separate append-only event log ([Event]).
type Notch struct {
	ID    string
	Title string
	Body  string
	// Tags are label names (the global label registry is joined by name).
	Tags []string
	// ParentID is the parent notch's ID, or "" for a top-level notch.
	ParentID string
	Status   NotchStatus

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Record is one row in tally's data substrate, admitted by merging a proposal.
// It carries provenance: AppID (which app admitted it) and ProposedBy (the
// proposal it came from — app.js calls this `talliedFrom`).
type Record struct {
	ID      string
	Dataset string
	Kind    string // "text" | "blob"

	// Summary is the human-readable line for a text record.
	Summary string
	// Name/Mime/Size/BlobURL describe a blob record; BlobURL is the seam a real
	// object store replaces (in the demo the bytes ride as a data: URL).
	Name    string
	Mime    string
	Size    int64
	BlobURL string

	// Source is the author's display name (e.g. "You", "Spotify (demo)").
	Source string
	// AppID and ProposedBy are the provenance chain: the app and the proposal
	// that admitted this record.
	AppID      string
	ProposedBy string

	// At is when the record was admitted. Store-owned: stamped on insert.
	At time.Time
}
