// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/clarkbar-sys/tally/internal/model"
)

// UpsertProposal inserts p, or updates it in place keyed on its ID. CreatedAt is
// stamped on first insert and preserved on update; UpdatedAt advances on every
// write. MergedAt is caller-owned (set it when merging) and stored as NULL until
// then. The proposal's event timeline is separate (see AppendProposalEvent) and
// is never touched here. The returned Proposal has CreatedAt/UpdatedAt populated.
func (s *Store) UpsertProposal(ctx context.Context, p model.Proposal) (model.Proposal, error) {
	linked, err := marshalStrings(p.LinkedNotches)
	if err != nil {
		return model.Proposal{}, fmt.Errorf("store: upsert proposal %s: encode linked_notches: %w", p.ID, err)
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO proposals (
			id, app_id, title, body, status, changes, linked_notches,
			created_at, updated_at, merged_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), ?)
		ON CONFLICT (id) DO UPDATE SET
			app_id         = excluded.app_id,
			title          = excluded.title,
			body           = excluded.body,
			status         = excluded.status,
			changes        = excluded.changes,
			linked_notches = excluded.linked_notches,
			merged_at      = excluded.merged_at,
			updated_at     = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		RETURNING created_at, updated_at`,
		p.ID, p.AppID, p.Title, p.Body, string(p.Status),
		rawJSON(p.Changes, "[]"), linked, formatNullTime(p.MergedAt),
	)

	var createdAt, updatedAt string
	if err := row.Scan(&createdAt, &updatedAt); err != nil {
		return model.Proposal{}, fmt.Errorf("store: upsert proposal %s: %w", p.ID, err)
	}
	if p.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Proposal{}, fmt.Errorf("store: upsert proposal: parse created_at: %w", err)
	}
	if p.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Proposal{}, fmt.Errorf("store: upsert proposal: parse updated_at: %w", err)
	}
	return p, nil
}

// GetProposal returns the proposal with the given ID.
func (s *Store) GetProposal(ctx context.Context, id string) (model.Proposal, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, app_id, title, body, status, changes, linked_notches,
		       created_at, updated_at, merged_at
		FROM proposals WHERE id = ?`, id)
	return scanProposal(row)
}

// ListProposals returns every proposal, most recently updated first.
func (s *Store) ListProposals(ctx context.Context) ([]model.Proposal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, app_id, title, body, status, changes, linked_notches,
		       created_at, updated_at, merged_at
		FROM proposals ORDER BY updated_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list proposals: %w", err)
	}
	defer rows.Close()

	var proposals []model.Proposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list proposals: %w", err)
	}
	return proposals, nil
}

// ListProposalsForNotch returns every proposal that links notchID (the notch↔PR
// relationship read from the notch's end). linked_notches is a JSON array, so
// membership is filtered in Go — fine at tally's single-user scale (ADR-0002).
func (s *Store) ListProposalsForNotch(ctx context.Context, notchID string) ([]model.Proposal, error) {
	all, err := s.ListProposals(ctx)
	if err != nil {
		return nil, err
	}
	var out []model.Proposal
	for _, p := range all {
		for _, id := range p.LinkedNotches {
			if id == notchID {
				out = append(out, p)
				break
			}
		}
	}
	return out, nil
}

// AppendProposalEvent appends ev to the proposal's append-only timeline.
func (s *Store) AppendProposalEvent(ctx context.Context, proposalID string, ev model.Event) (model.Event, error) {
	return s.appendEvent(ctx, "proposal_events", "proposal_id", proposalID, ev)
}

// ListProposalEvents returns the proposal's timeline in append order.
func (s *Store) ListProposalEvents(ctx context.Context, proposalID string) ([]model.Event, error) {
	return s.listEvents(ctx, "proposal_events", "proposal_id", proposalID)
}

func scanProposal(row rowScanner) (model.Proposal, error) {
	var p model.Proposal
	var status, changes, linked, createdAt, updatedAt string
	var mergedAt sql.NullString
	if err := row.Scan(
		&p.ID, &p.AppID, &p.Title, &p.Body, &status, &changes, &linked,
		&createdAt, &updatedAt, &mergedAt,
	); err != nil {
		return model.Proposal{}, fmt.Errorf("store: scan proposal: %w", err)
	}
	p.Status = model.ProposalStatus(status)
	p.Changes = []byte(changes)

	var err error
	if p.LinkedNotches, err = unmarshalStrings(linked); err != nil {
		return model.Proposal{}, fmt.Errorf("store: scan proposal: decode linked_notches: %w", err)
	}
	if p.CreatedAt, err = parseTime(createdAt); err != nil {
		return model.Proposal{}, fmt.Errorf("store: scan proposal: parse created_at: %w", err)
	}
	if p.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return model.Proposal{}, fmt.Errorf("store: scan proposal: parse updated_at: %w", err)
	}
	if p.MergedAt, err = parseNullTime(mergedAt); err != nil {
		return model.Proposal{}, fmt.Errorf("store: scan proposal: parse merged_at: %w", err)
	}
	return p, nil
}
