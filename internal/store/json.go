// SPDX-License-Identifier: GPL-2.0-or-later

package store

import "encoding/json"

// The app-protocol tables store a few small, polymorphic values as JSON text
// columns (an app's scopes, a proposal's linked-notch ids, a notch's tags, and
// the engine-owned change/event payloads). These helpers keep that encoding in
// one place. Columns default to '[]'/'{}' at the schema level, so a stored value
// is always valid JSON; a nil slice round-trips as an empty array, not null.

// marshalStrings encodes a string slice as a JSON array, treating nil as empty.
func marshalStrings(v []string) (string, error) {
	if v == nil {
		v = []string{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalStrings decodes a JSON array column into a string slice. An empty or
// "[]" column yields a nil slice.
func unmarshalStrings(s string) ([]string, error) {
	if s == "" || s == "[]" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// rawJSON normalizes an engine-owned JSON payload for storage: an empty value
// becomes the given default ('[]' for a diff, '{}' for an event payload) so the
// column stays valid JSON.
func rawJSON(raw json.RawMessage, def string) string {
	if len(raw) == 0 {
		return def
	}
	return string(raw)
}

// nullString stores an empty string as SQL NULL, so an optional column (e.g. a
// notch's parent_id) distinguishes "unset" from the empty string.
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
