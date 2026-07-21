// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/clarkbar-sys/tally/internal/model"
)

// UpsertApp inserts a, or updates it in place keyed on its ID. InstalledAt is
// store-owned: stamped on first insert and preserved across updates (re-registering
// an app doesn't reset when it was installed). The returned App has InstalledAt
// populated from the row.
func (s *Store) UpsertApp(ctx context.Context, a model.App) (model.App, error) {
	scopes, err := marshalStrings(a.Scopes)
	if err != nil {
		return model.App{}, fmt.Errorf("store: upsert app %s: encode scopes: %w", a.ID, err)
	}
	var action any // NULL when the app has no action
	if a.Action != nil {
		b, err := json.Marshal(a.Action)
		if err != nil {
			return model.App{}, fmt.Errorf("store: upsert app %s: encode action: %w", a.ID, err)
		}
		action = string(b)
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO apps (
			id, name, kind, color, blurb, scopes, action, status, installed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (id) DO UPDATE SET
			name   = excluded.name,
			kind   = excluded.kind,
			color  = excluded.color,
			blurb  = excluded.blurb,
			scopes = excluded.scopes,
			action = excluded.action,
			status = excluded.status
		RETURNING installed_at`,
		a.ID, a.Name, a.Kind, a.Color, a.Blurb, scopes, action, string(a.Status),
	)

	var installedAt string
	if err := row.Scan(&installedAt); err != nil {
		return model.App{}, fmt.Errorf("store: upsert app %s: %w", a.ID, err)
	}
	if a.InstalledAt, err = parseTime(installedAt); err != nil {
		return model.App{}, fmt.Errorf("store: upsert app: parse installed_at: %w", err)
	}
	return a, nil
}

// GetApp returns the app with the given ID.
func (s *Store) GetApp(ctx context.Context, id string) (model.App, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, kind, color, blurb, scopes, action, status, installed_at
		FROM apps WHERE id = ?`, id)
	return scanApp(row)
}

// ListApps returns every registered app, ordered by installation time.
func (s *Store) ListApps(ctx context.Context) ([]model.App, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, kind, color, blurb, scopes, action, status, installed_at
		FROM apps ORDER BY installed_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list apps: %w", err)
	}
	defer rows.Close()

	var apps []model.App
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list apps: %w", err)
	}
	return apps, nil
}

func scanApp(row rowScanner) (model.App, error) {
	var a model.App
	var scopes, status, installedAt string
	var action sql.NullString
	if err := row.Scan(
		&a.ID, &a.Name, &a.Kind, &a.Color, &a.Blurb, &scopes, &action, &status, &installedAt,
	); err != nil {
		return model.App{}, fmt.Errorf("store: scan app: %w", err)
	}
	a.Status = model.AppStatus(status)

	var err error
	if a.Scopes, err = unmarshalStrings(scopes); err != nil {
		return model.App{}, fmt.Errorf("store: scan app: decode scopes: %w", err)
	}
	if action.Valid {
		var act model.AppAction
		if err := json.Unmarshal([]byte(action.String), &act); err != nil {
			return model.App{}, fmt.Errorf("store: scan app: decode action: %w", err)
		}
		a.Action = &act
	}
	if a.InstalledAt, err = parseTime(installedAt); err != nil {
		return model.App{}, fmt.Errorf("store: scan app: parse installed_at: %w", err)
	}
	return a, nil
}
