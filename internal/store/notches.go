// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/clarkbar-sys/tally/internal/model"
)

// UpsertNotch inserts n, or updates it in place keyed on its ID. CreatedAt is
// stamped on first insert and preserved on update; UpdatedAt advances on every
// write. The notch's event timeline is separate (see AppendNotchEvent) and is
// never touched here. The returned Notch has CreatedAt/UpdatedAt populated.
func (s *Store) UpsertNotch(ctx context.Context, n model.Notch) (model.Notch, error) {
	tags, err := marshalStrings(n.Tags)
	if err != nil {
		return model.Notch{}, fmt.Errorf("store: upsert notch %s: encode tags: %w", n.ID, err)
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO notches (
			id, title, body, tags, parent_id, status, due_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (id) DO UPDATE SET
			title      = excluded.title,
			body       = excluded.body,
			tags       = excluded.tags,
			parent_id  = excluded.parent_id,
			status     = excluded.status,
			due_at     = excluded.due_at,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		RETURNING created_at, updated_at`,
		n.ID, n.Title, n.Body, tags, nullString(n.ParentID), string(n.Status), formatNullTime(n.DueAt),
	)

	var createdAt, updatedAt string
	if err := row.Scan(&createdAt, &updatedAt); err != nil {
		return model.Notch{}, fmt.Errorf("store: upsert notch %s: %w", n.ID, err)
	}
	if n.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Notch{}, fmt.Errorf("store: upsert notch: parse created_at: %w", err)
	}
	if n.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Notch{}, fmt.Errorf("store: upsert notch: parse updated_at: %w", err)
	}
	return n, nil
}

// GetNotch returns the notch with the given ID.
func (s *Store) GetNotch(ctx context.Context, id string) (model.Notch, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, body, tags, parent_id, status, due_at, created_at, updated_at
		FROM notches WHERE id = ?`, id)
	return scanNotch(row)
}

// ListNotches returns every notch, most recently updated first.
func (s *Store) ListNotches(ctx context.Context) ([]model.Notch, error) {
	return s.queryNotches(ctx, `
		SELECT id, title, body, tags, parent_id, status, due_at, created_at, updated_at
		FROM notches ORDER BY updated_at DESC, id`)
}

// ListChildNotches returns the direct children of parentID, most recently
// updated first.
func (s *Store) ListChildNotches(ctx context.Context, parentID string) ([]model.Notch, error) {
	return s.queryNotches(ctx, `
		SELECT id, title, body, tags, parent_id, status, due_at, created_at, updated_at
		FROM notches WHERE parent_id = ? ORDER BY updated_at DESC, id`, parentID)
}

// AppendNotchEvent appends ev to the notch's append-only timeline.
func (s *Store) AppendNotchEvent(ctx context.Context, notchID string, ev model.Event) (model.Event, error) {
	return s.appendEvent(ctx, "notch_events", "notch_id", notchID, ev)
}

// ListNotchEvents returns the notch's timeline in append order.
func (s *Store) ListNotchEvents(ctx context.Context, notchID string) ([]model.Event, error) {
	return s.listEvents(ctx, "notch_events", "notch_id", notchID)
}

func (s *Store) queryNotches(ctx context.Context, query string, args ...any) ([]model.Notch, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query notches: %w", err)
	}
	defer rows.Close()

	var notches []model.Notch
	for rows.Next() {
		n, err := scanNotch(rows)
		if err != nil {
			return nil, err
		}
		notches = append(notches, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: query notches: %w", err)
	}
	return notches, nil
}

func scanNotch(row rowScanner) (model.Notch, error) {
	var n model.Notch
	var tags, status, createdAt, updatedAt string
	var parentID, dueAt sql.NullString
	if err := row.Scan(
		&n.ID, &n.Title, &n.Body, &tags, &parentID, &status, &dueAt, &createdAt, &updatedAt,
	); err != nil {
		return model.Notch{}, fmt.Errorf("store: scan notch: %w", err)
	}
	n.ParentID = parentID.String
	n.Status = model.NotchStatus(status)

	var err error
	if n.Tags, err = unmarshalStrings(tags); err != nil {
		return model.Notch{}, fmt.Errorf("store: scan notch: decode tags: %w", err)
	}
	if n.DueAt, err = parseNullTime(dueAt); err != nil {
		return model.Notch{}, fmt.Errorf("store: scan notch: parse due_at: %w", err)
	}
	if n.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Notch{}, fmt.Errorf("store: scan notch: parse created_at: %w", err)
	}
	if n.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Notch{}, fmt.Errorf("store: scan notch: parse updated_at: %w", err)
	}
	return n, nil
}
