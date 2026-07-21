// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"fmt"

	"github.com/clarkbar-sys/tally/internal/model"
)

// InsertRecord writes r into the substrate. Records are append-only — a merged
// proposal admits them and nothing rewrites them — so this inserts rather than
// upserts. At is store-owned: stamped on insert. The returned Record has At
// populated. Its AppID and ProposedBy must reference existing rows (the
// provenance chain is foreign-key enforced).
func (s *Store) InsertRecord(ctx context.Context, r model.Record) (model.Record, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO records (
			id, dataset, kind, summary, name, mime, size, blob_url,
			source, app_id, proposed_by, at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		RETURNING at`,
		r.ID, r.Dataset, r.Kind, r.Summary, r.Name, r.Mime, r.Size, r.BlobURL,
		r.Source, r.AppID, r.ProposedBy,
	)

	var at string
	if err := row.Scan(&at); err != nil {
		return model.Record{}, fmt.Errorf("store: insert record %s: %w", r.ID, err)
	}
	var err error
	if r.At, err = parseTime(at); err != nil {
		return model.Record{}, fmt.Errorf("store: insert record: parse at: %w", err)
	}
	return r, nil
}

// ListRecords returns every record, most recent first.
func (s *Store) ListRecords(ctx context.Context) ([]model.Record, error) {
	return s.queryRecords(ctx, `
		SELECT id, dataset, kind, summary, name, mime, size, blob_url,
		       source, app_id, proposed_by, at
		FROM records ORDER BY at DESC, id`)
}

// ListRecordsByDataset returns every record in a dataset, most recent first.
func (s *Store) ListRecordsByDataset(ctx context.Context, dataset string) ([]model.Record, error) {
	return s.queryRecords(ctx, `
		SELECT id, dataset, kind, summary, name, mime, size, blob_url,
		       source, app_id, proposed_by, at
		FROM records WHERE dataset = ? ORDER BY at DESC, id`, dataset)
}

// ListRecordsByProposal returns the records a given proposal admitted, most
// recent first.
func (s *Store) ListRecordsByProposal(ctx context.Context, proposalID string) ([]model.Record, error) {
	return s.queryRecords(ctx, `
		SELECT id, dataset, kind, summary, name, mime, size, blob_url,
		       source, app_id, proposed_by, at
		FROM records WHERE proposed_by = ? ORDER BY at DESC, id`, proposalID)
}

func (s *Store) queryRecords(ctx context.Context, query string, args ...any) ([]model.Record, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query records: %w", err)
	}
	defer rows.Close()

	var records []model.Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: query records: %w", err)
	}
	return records, nil
}

func scanRecord(row rowScanner) (model.Record, error) {
	var r model.Record
	var at string
	if err := row.Scan(
		&r.ID, &r.Dataset, &r.Kind, &r.Summary, &r.Name, &r.Mime, &r.Size, &r.BlobURL,
		&r.Source, &r.AppID, &r.ProposedBy, &at,
	); err != nil {
		return model.Record{}, fmt.Errorf("store: scan record: %w", err)
	}
	var err error
	if r.At, err = parseTime(at); err != nil {
		return model.Record{}, fmt.Errorf("store: scan record: parse at: %w", err)
	}
	return r, nil
}
