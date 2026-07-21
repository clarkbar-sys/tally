// SPDX-License-Identifier: GPL-2.0-or-later

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/clarkbar-sys/tally/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tally.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func serve(h http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	h.ServeHTTP(rec, r)
	return rec
}

// A representative snapshot in the exact wire shape app.js sends: epoch-ms
// timestamps, "tallies" for proposals, flat events, a null and a non-null label
// override, and a blob record.
const sampleState = `{
  "labels": [
    {"name":"demo","color":"blue","bg":null,"fg":null},
    {"name":"errand","color":"green","bg":"#0a5","fg":"#fff"}
  ],
  "apps": [
    {"id":"you","name":"You","kind":"you","color":"blue","blurb":"you","scopes":["notches:read","records:propose"],"action":null,"status":"active","installedAt":1000},
    {"id":"spotify","name":"Spotify","kind":"connected","color":"green","blurb":"","scopes":["records:propose"],"action":{"label":"Sync","verb":"sync"},"status":"active","installedAt":2000}
  ],
  "notches": [
    {"id":"n_parent","title":"Parent","body":"b","tags":["demo"],"parentId":null,"status":"open","createdAt":3000,"updatedAt":3500,
     "events":[
       {"id":"e1","kind":"opened","at":3000},
       {"id":"e2","kind":"comment","at":3200,"body":"hi","deleted":true}
     ]},
    {"id":"n_child","title":"Child","body":"","tags":[],"parentId":"n_parent","status":"done","createdAt":4000,"updatedAt":4000,
     "events":[{"id":"e3","kind":"opened","at":4000}]}
  ],
  "tallies": [
    {"id":"t_open","title":"Import","body":"","appId":"spotify","status":"open","changes":[{"op":"add-records","dataset":"d","rows":[{"summary":"x"}]}],"linkedNotches":[],"createdAt":5000,"updatedAt":5000,"mergedAt":null,
     "events":[{"id":"e4","kind":"opened","at":5000,"author":"Spotify"}]},
    {"id":"t_merged","title":"Seed","body":"","appId":"you","status":"merged","changes":[],"linkedNotches":["n_child"],"createdAt":6000,"updatedAt":6500,"mergedAt":6500,
     "events":[{"id":"e5","kind":"merged","at":6500,"changes":1}]}
  ],
  "records": [
    {"id":"r1","dataset":"d","kind":"text","summary":"x","source":"You","appId":"you","talliedFrom":"t_merged","at":6600},
    {"id":"r2","dataset":"f","kind":"blob","name":"a.svg","mime":"image/svg+xml","size":512,"blobUrl":"data:image/svg+xml,x","source":"You","appId":"you","talliedFrom":"t_merged","at":6700}
  ]
}`

func TestStatePutThenGetRoundTrips(t *testing.T) {
	h := Handler(openStore(t))

	if rec := serve(h, http.MethodPut, "/api/state", []byte(sampleState)); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d (%s), want 204", rec.Code, rec.Body.String())
	}

	rec := serve(h, http.MethodGet, "/api/state", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}

	// Compare structurally: the wire shape must survive the trip through SQLite.
	var got, want map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET body: %v", err)
	}
	if err := json.Unmarshal([]byte(sampleState), &want); err != nil {
		t.Fatalf("decode want: %v", err)
	}

	for _, key := range []string{"labels", "apps", "notches", "tallies", "records"} {
		g, _ := json.Marshal(canon(got[key]))
		w, _ := json.Marshal(canon(want[key]))
		if string(g) != string(w) {
			t.Errorf("%s did not round-trip.\n got:  %s\n want: %s", key, g, w)
		}
	}
}

func TestStateGetOnFreshDBIsEmpty(t *testing.T) {
	h := Handler(openStore(t))
	rec := serve(h, http.MethodGet, "/api/state", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	var got map[string][]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"labels", "apps", "notches", "tallies", "records"} {
		v, ok := got[key]
		if !ok {
			t.Errorf("fresh state missing %q key", key)
		}
		if len(v) != 0 {
			t.Errorf("fresh %s = %v, want empty", key, v)
		}
	}
}

func TestStatePutRejectsBadJSON(t *testing.T) {
	h := Handler(openStore(t))
	if rec := serve(h, http.MethodPut, "/api/state", []byte("{not json")); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT bad json status = %d, want 400", rec.Code)
	}
}

func TestStateRejectsUnsupportedMethod(t *testing.T) {
	h := Handler(openStore(t))
	if rec := serve(h, http.MethodDelete, "/api/state", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE status = %d, want 405", rec.Code)
	}
}

// canon recursively sorts objects into a stable form so two JSON values compare
// equal regardless of key order or the array element order within each resource
// list. Elements are keyed by their "id"/"name" when present.
func canon(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = canon(e)
		}
		// Sort by the element's identity key so ordering is irrelevant.
		sortByKey(out)
		return out
	case map[string]any:
		out := map[string]any{}
		for k, e := range t {
			out[k] = canon(e)
		}
		return out
	default:
		return t
	}
}

func sortByKey(a []any) {
	key := func(x any) string {
		m, ok := x.(map[string]any)
		if !ok {
			b, _ := json.Marshal(x)
			return string(b)
		}
		for _, k := range []string{"id", "name"} {
			if s, ok := m[k].(string); ok {
				return s
			}
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && key(a[j-1]) > key(a[j]); j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
