// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"

	"github.com/clarkbar-sys/tally/internal/model"
)

// Notches and proposals both keep an append-only event timeline with the same
// shape (see the *_events tables in 0002). appendEvent and listEvents are the
// shared implementation; the per-entity methods (AppendNotchEvent,
// AppendProposalEvent, ...) name the table and foreign-key column.
//
// The event's At is store-owned — stamped on append so the log stays monotonic —
// while its ID and Payload are preserved as written (the engine owns them).

func (s *Store) appendEvent(ctx context.Context, table, fkCol, fkID string, ev model.Event) (model.Event, error) {
	query := fmt.Sprintf(`
		INSERT INTO %s (id, %s, kind, at, payload)
		VALUES (?, ?, ?, strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ', 'now'), ?)
		RETURNING at`, table, fkCol)

	row := s.db.QueryRowContext(ctx, query, ev.ID, fkID, ev.Kind, rawJSON(ev.Payload, "{}"))
	var at string
	if err := row.Scan(&at); err != nil {
		return model.Event{}, fmt.Errorf("store: append event to %s %s: %w", table, fkID, err)
	}
	var err error
	if ev.At, err = parseTime(at); err != nil {
		return model.Event{}, fmt.Errorf("store: append event: parse at: %w", err)
	}
	return ev, nil
}

func (s *Store) listEvents(ctx context.Context, table, fkCol, fkID string) ([]model.Event, error) {
	query := fmt.Sprintf(`SELECT id, kind, at, payload FROM %s WHERE %s = ? ORDER BY seq`, table, fkCol)
	rows, err := s.db.QueryContext(ctx, query, fkID)
	if err != nil {
		return nil, fmt.Errorf("store: list events from %s %s: %w", table, fkID, err)
	}
	defer rows.Close()

	var events []model.Event
	for rows.Next() {
		var ev model.Event
		var at, payload string
		if err := rows.Scan(&ev.ID, &ev.Kind, &at, &payload); err != nil {
			return nil, fmt.Errorf("store: scan event from %s: %w", table, err)
		}
		if ev.At, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("store: scan event: parse at: %w", err)
		}
		ev.Payload = []byte(payload)
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list events from %s %s: %w", table, fkID, err)
	}
	return events, nil
}
