// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/clarkbar-sys/tally/internal/model"
)

// AnyVersion, passed as the base to SaveState, skips the compare-and-swap and
// saves unconditionally (last-writer-wins). It is the base a client uses when it
// has never synced — a first push / migration — where there is no prior version
// to conflict against.
const AnyVersion int64 = -1

// ErrVersionConflict is returned by SaveState when the caller's base version no
// longer matches the stored one: another writer bumped it in between. The caller
// should re-read the current snapshot (and version) rather than overwrite it.
var ErrVersionConflict = errors.New("store: state version conflict")

// execer is the subset of *sql.Tx the snapshot insert helpers need, so they read
// as plain row writers without carrying the whole transaction type.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// This file is the single-user persistence slice (#113): the whole of app.js's
// in-memory state, saved to and loaded from the app-protocol tables (0002) plus
// the labels table (0003) as one snapshot. It exists because that build is
// client-driven — app.js owns the model and its timestamps, and mirrors a full
// snapshot after every edit — so the seam the web handler needs is "replace all
// state with this snapshot" / "give me the whole snapshot", not the per-op,
// store-stamps-the-time methods the future engine (S1) writes through.
//
// The engine-facing UpsertNotch/AppendNotchEvent/InsertRecord methods stamp
// their own created_at/at server-side; that is correct for an authoritative
// engine but would clobber the client's timestamps on every save here. SaveState
// therefore writes the tables directly, preserving the caller's timestamps, all
// inside one transaction so a snapshot lands atomically.

// NotchState is a notch together with its append-only event timeline — the shape
// app.js carries inline on each notch.
type NotchState struct {
	model.Notch
	Events []model.Event
}

// ProposalState is a proposal ("tally") together with its event timeline.
type ProposalState struct {
	model.Proposal
	Events []model.Event
}

// Snapshot is the full app state app.js persists: the global label registry, the
// registered apps, every notch and proposal (each with its timeline), and the
// admitted records. It round-trips losslessly through SaveState/LoadState.
type Snapshot struct {
	Labels    []model.Label
	Apps      []model.App
	Notches   []NotchState
	Proposals []ProposalState
	Records   []model.Record
}

// LoadState returns the whole persisted snapshot, reconstructed from the tables.
// A never-initialized database yields an empty snapshot (all slices nil), which
// the client reads as a fresh install.
func (s *Store) LoadState(ctx context.Context) (Snapshot, error) {
	var snap Snapshot
	var err error

	if snap.Labels, err = s.listLabels(ctx); err != nil {
		return Snapshot{}, err
	}
	if snap.Apps, err = s.ListApps(ctx); err != nil {
		return Snapshot{}, err
	}

	notches, err := s.ListNotches(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	for _, n := range notches {
		events, err := s.ListNotchEvents(ctx, n.ID)
		if err != nil {
			return Snapshot{}, err
		}
		snap.Notches = append(snap.Notches, NotchState{Notch: n, Events: events})
	}

	proposals, err := s.ListProposals(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	for _, p := range proposals {
		events, err := s.ListProposalEvents(ctx, p.ID)
		if err != nil {
			return Snapshot{}, err
		}
		snap.Proposals = append(snap.Proposals, ProposalState{Proposal: p, Events: events})
	}

	if snap.Records, err = s.ListRecords(ctx); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// StateVersion returns the stored snapshot's monotonic version — 0 on a
// never-saved database. It is bumped by every successful SaveState and handed to
// the client (as an ETag) so the client can send it back for compare-and-swap.
func (s *Store) StateVersion(ctx context.Context) (int64, error) {
	var v int64
	err := s.db.QueryRowContext(ctx, `SELECT version FROM state_version WHERE id = 1`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: read state version: %w", err)
	}
	return v, nil
}

// SaveState replaces all persisted state with snap, atomically, and returns the
// new snapshot version. The client is the source of truth in this build, so a
// save is a full overwrite: it clears every table and re-inserts the snapshot,
// preserving the caller's timestamps rather than stamping new ones. Foreign keys
// are deferred to commit so the insert order within the transaction doesn't
// matter (a notch's parent, a proposal's app, a record's source proposal can all
// be written in any order).
//
// base is the version the caller last saw. If it is AnyVersion the save is
// unconditional (a first push / migration); otherwise it must equal the stored
// version — a compare-and-swap. A stale base returns ErrVersionConflict and
// writes nothing, so a concurrent edit surfaces as a conflict instead of a silent
// overwrite. On success the version is bumped by one and returned.
func (s *Store) SaveState(ctx context.Context, snap Snapshot, base int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: save state: begin: %w", err)
	}
	defer tx.Rollback()

	// Read the current version inside the transaction so the compare-and-swap and
	// the bump are atomic against a concurrent writer (single-writer WAL).
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT version FROM state_version WHERE id = 1`).Scan(&current); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("store: save state: read version: %w", err)
		}
		current = 0
	}
	if base != AnyVersion && base != current {
		return current, ErrVersionConflict
	}

	// Defer FK checks to COMMIT: the snapshot is internally consistent but its
	// rows can reference each other in any order (self-referencing notches, a
	// record's proposal), so per-statement FK enforcement would be too strict.
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return 0, fmt.Errorf("store: save state: defer fks: %w", err)
	}

	for _, table := range []string{
		"records", "notch_events", "proposal_events", "proposals", "notches", "labels", "apps",
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return 0, fmt.Errorf("store: save state: clear %s: %w", table, err)
		}
	}

	for _, a := range snap.Apps {
		if err := insertApp(ctx, tx, a); err != nil {
			return 0, err
		}
	}
	for _, l := range snap.Labels {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO labels (name, color, bg, fg) VALUES (?, ?, ?, ?)`,
			l.Name, l.Color, nullPtr(l.Bg), nullPtr(l.Fg),
		); err != nil {
			return 0, fmt.Errorf("store: save state: insert label %q: %w", l.Name, err)
		}
	}
	for _, n := range snap.Notches {
		if err := insertNotch(ctx, tx, n.Notch); err != nil {
			return 0, err
		}
		if err := insertEvents(ctx, tx, "notch_events", "notch_id", n.ID, n.Events); err != nil {
			return 0, err
		}
	}
	for _, p := range snap.Proposals {
		if err := insertProposal(ctx, tx, p.Proposal); err != nil {
			return 0, err
		}
		if err := insertEvents(ctx, tx, "proposal_events", "proposal_id", p.ID, p.Events); err != nil {
			return 0, err
		}
	}
	for _, r := range snap.Records {
		if err := insertRecord(ctx, tx, r); err != nil {
			return 0, err
		}
	}

	next := current + 1
	if _, err := tx.ExecContext(ctx,
		`UPDATE state_version SET version = ? WHERE id = 1`, next,
	); err != nil {
		return 0, fmt.Errorf("store: save state: bump version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: save state: commit: %w", err)
	}
	return next, nil
}

func (s *Store) listLabels(ctx context.Context) ([]model.Label, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, color, bg, fg FROM labels ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list labels: %w", err)
	}
	defer rows.Close()

	var labels []model.Label
	for rows.Next() {
		var l model.Label
		if err := rows.Scan(&l.Name, &l.Color, &l.Bg, &l.Fg); err != nil {
			return nil, fmt.Errorf("store: scan label: %w", err)
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list labels: %w", err)
	}
	return labels, nil
}

// The insert* helpers below write one row with the caller's timestamps intact,
// the deliberate difference from the Upsert*/Insert*/Append* methods elsewhere in
// this package (which stamp the time themselves). They take a *sql.Tx so SaveState
// can batch a whole snapshot atomically.

func insertApp(ctx context.Context, tx execer, a model.App) error {
	scopes, err := marshalStrings(a.Scopes)
	if err != nil {
		return fmt.Errorf("store: save state: encode scopes for app %s: %w", a.ID, err)
	}
	var action any
	if a.Action != nil {
		b, err := json.Marshal(a.Action)
		if err != nil {
			return fmt.Errorf("store: save state: encode action for app %s: %w", a.ID, err)
		}
		action = string(b)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO apps (id, name, kind, color, blurb, scopes, action, status, installed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Kind, a.Color, a.Blurb, scopes, action, string(a.Status), formatTime(a.InstalledAt),
	); err != nil {
		return fmt.Errorf("store: save state: insert app %s: %w", a.ID, err)
	}
	return nil
}

func insertNotch(ctx context.Context, tx execer, n model.Notch) error {
	tags, err := marshalStrings(n.Tags)
	if err != nil {
		return fmt.Errorf("store: save state: encode tags for notch %s: %w", n.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO notches (id, title, body, tags, parent_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Title, n.Body, tags, nullString(n.ParentID), string(n.Status),
		formatTime(n.CreatedAt), formatTime(n.UpdatedAt),
	); err != nil {
		return fmt.Errorf("store: save state: insert notch %s: %w", n.ID, err)
	}
	return nil
}

func insertProposal(ctx context.Context, tx execer, p model.Proposal) error {
	linked, err := marshalStrings(p.LinkedNotches)
	if err != nil {
		return fmt.Errorf("store: save state: encode linked notches for proposal %s: %w", p.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO proposals (id, app_id, title, body, status, changes, linked_notches, created_at, updated_at, merged_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.AppID, p.Title, p.Body, string(p.Status), rawJSON(p.Changes, "[]"), linked,
		formatTime(p.CreatedAt), formatTime(p.UpdatedAt), formatNullTime(p.MergedAt),
	); err != nil {
		return fmt.Errorf("store: save state: insert proposal %s: %w", p.ID, err)
	}
	return nil
}

func insertRecord(ctx context.Context, tx execer, r model.Record) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO records (id, dataset, kind, summary, name, mime, size, blob_url, source, app_id, proposed_by, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Dataset, r.Kind, r.Summary, r.Name, r.Mime, r.Size, r.BlobURL,
		r.Source, r.AppID, r.ProposedBy, formatTime(r.At),
	); err != nil {
		return fmt.Errorf("store: save state: insert record %s: %w", r.ID, err)
	}
	return nil
}

func insertEvents(ctx context.Context, tx execer, table, fkCol, fkID string, events []model.Event) error {
	query := fmt.Sprintf(`INSERT INTO %s (id, %s, kind, at, payload) VALUES (?, ?, ?, ?, ?)`, table, fkCol)
	for _, ev := range events {
		if _, err := tx.ExecContext(ctx, query,
			ev.ID, fkID, ev.Kind, formatTime(ev.At), rawJSON(ev.Payload, "{}"),
		); err != nil {
			return fmt.Errorf("store: save state: insert event into %s for %s: %w", table, fkID, err)
		}
	}
	return nil
}

// nullPtr stores a nil *string as SQL NULL and a non-nil one as its value, so an
// untouched label color override round-trips as NULL rather than "".
func nullPtr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
