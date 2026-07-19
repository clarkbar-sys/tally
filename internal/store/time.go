// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"database/sql"
	"time"
)

// timestamps are stored as RFC3339Nano text in UTC, SQLite has no native
// datetime type and this keeps values sortable as strings.

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// formatNullTime returns nil for the zero time, so an unset LastSyncedAt or
// PostedAt is stored as SQL NULL rather than a formatted zero-value string.
func formatNullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func parseNullTime(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	return parseTime(s.String)
}
