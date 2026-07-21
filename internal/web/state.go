// SPDX-License-Identifier: GPL-2.0-or-later

package web

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
	"github.com/clarkbar-sys/tally/internal/store"
)

// The /api/state endpoints are the live build's persistence seam (#113). app.js
// is the source of truth: it keeps the whole model in memory and, after every
// edit, PUTs a full snapshot here; on boot it GETs the snapshot back. The server
// just round-trips that snapshot through the SQLite store (internal/store) — no
// engine, no per-op writes. Demo mode (the static export) never registers these
// routes and makes no network calls, so it is untouched.
//
// The wire shape is exactly app.js's in-memory shape: timestamps are epoch
// milliseconds (Date.now()), a tally is the client's word for a proposal, and an
// event is a flat object whose kind-specific fields ride alongside id/kind/at.
// The DTOs below carry that shape and convert to/from the store's typed model,
// so the client contract stays put while the data lands in the real tables.

// maxStateBytes caps a PUT /api/state body. A single-user snapshot with inline
// blob data: URLs can be large, but this still bounds a runaway request.
const maxStateBytes = 64 << 20 // 64 MiB

type stateDTO struct {
	Labels  []labelDTO  `json:"labels"`
	Apps    []appDTO    `json:"apps"`
	Notches []notchDTO  `json:"notches"`
	Tallies []tallyDTO  `json:"tallies"`
	Records []recordDTO `json:"records"`
}

type labelDTO struct {
	Name  string  `json:"name"`
	Color string  `json:"color"`
	Bg    *string `json:"bg"`
	Fg    *string `json:"fg"`
}

type actionDTO struct {
	Label string `json:"label"`
	Verb  string `json:"verb"`
}

type appDTO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	Color       string     `json:"color"`
	Blurb       string     `json:"blurb"`
	Scopes      []string   `json:"scopes"`
	Action      *actionDTO `json:"action"`
	Status      string     `json:"status"`
	InstalledAt int64      `json:"installedAt"`
}

type notchDTO struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Tags      []string          `json:"tags"`
	Events    []json.RawMessage `json:"events"`
	ParentID  *string           `json:"parentId"` // null for a top-level notch
	Status    string            `json:"status"`
	CreatedAt int64             `json:"createdAt"`
	UpdatedAt int64             `json:"updatedAt"`
}

type tallyDTO struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Body          string            `json:"body"`
	AppID         string            `json:"appId"`
	Status        string            `json:"status"`
	Changes       json.RawMessage   `json:"changes"`
	LinkedNotches []string          `json:"linkedNotches"`
	Events        []json.RawMessage `json:"events"`
	CreatedAt     int64             `json:"createdAt"`
	UpdatedAt     int64             `json:"updatedAt"`
	MergedAt      *int64            `json:"mergedAt"`
}

type recordDTO struct {
	ID          string `json:"id"`
	Dataset     string `json:"dataset"`
	Kind        string `json:"kind"`
	Summary     string `json:"summary,omitempty"`
	Name        string `json:"name,omitempty"`
	Mime        string `json:"mime,omitempty"`
	Size        int64  `json:"size,omitempty"`
	BlobURL     string `json:"blobUrl,omitempty"`
	Source      string `json:"source"`
	AppID       string `json:"appId"`
	TalliedFrom string `json:"talliedFrom"`
	At          int64  `json:"at"`
}

// stateHandler serves GET/PUT /api/state against st. It is only mounted in the
// live build (st != nil); the demo export never reaches it.
func stateHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getState(w, r, st)
		case http.MethodPut:
			putState(w, r, st)
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func getState(w http.ResponseWriter, r *http.Request, st *store.Store) {
	snap, err := st.LoadState(r.Context())
	if err != nil {
		log.Printf("web: load state: %v", err)
		http.Error(w, "could not load state", http.StatusInternalServerError)
		return
	}
	version, err := st.StateVersion(r.Context())
	if err != nil {
		log.Printf("web: state version: %v", err)
		http.Error(w, "could not load state", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("ETag", etag(version))
	if err := json.NewEncoder(w).Encode(snapshotToDTO(snap)); err != nil {
		log.Printf("web: encode state: %v", err)
	}
}

func putState(w http.ResponseWriter, r *http.Request, st *store.Store) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxStateBytes))
	var dto stateDTO
	if err := dec.Decode(&dto); err != nil {
		http.Error(w, "invalid state payload", http.StatusBadRequest)
		return
	}
	snap, err := dtoToSnapshot(dto)
	if err != nil {
		http.Error(w, "invalid state payload", http.StatusBadRequest)
		return
	}

	// The client sends the version it last saw as If-Match, so the save is a
	// compare-and-swap; a client that has never synced (a first push / migration)
	// omits the header and saves unconditionally (store.AnyVersion).
	base := store.AnyVersion
	if v, ok := parseIfMatch(r.Header.Get("If-Match")); ok {
		base = v
	}

	version, err := st.SaveState(r.Context(), snap, base)
	if errors.Is(err, store.ErrVersionConflict) {
		// Stale base: another writer moved the snapshot underneath this one. Don't
		// clobber — hand back the current server snapshot (and its version) so the
		// client can quarantine its dirty edits and adopt the hub's copy.
		writeConflict(w, r, st, version)
		return
	}
	if err != nil {
		log.Printf("web: save state: %v", err)
		http.Error(w, "could not save state", http.StatusInternalServerError)
		return
	}
	w.Header().Set("ETag", etag(version))
	w.WriteHeader(http.StatusNoContent)
}

// writeConflict answers a stale PUT with 409 plus the current server snapshot in
// the GET body shape, so the client can adopt it as the live state without a
// second round-trip. The ETag carries the version that snapshot is at.
func writeConflict(w http.ResponseWriter, r *http.Request, st *store.Store, version int64) {
	snap, err := st.LoadState(r.Context())
	if err != nil {
		log.Printf("web: load state (conflict): %v", err)
		http.Error(w, "could not load state", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("ETag", etag(version))
	w.WriteHeader(http.StatusConflict)
	if err := json.NewEncoder(w).Encode(snapshotToDTO(snap)); err != nil {
		log.Printf("web: encode state (conflict): %v", err)
	}
}

// etag renders a snapshot version as a strong ETag (a quoted decimal), e.g.
// version 5 → `"5"`.
func etag(version int64) string {
	return strconv.Quote(strconv.FormatInt(version, 10))
}

// parseIfMatch reads a version out of an If-Match header value, tolerating the
// quoting and weak-validator prefix an entity-tag may carry (`"5"`, `W/"5"`, or a
// bare `5`). A `*` (match-any) or an unparseable value yields ok=false, so the
// caller falls back to an unconditional save.
func parseIfMatch(raw string) (int64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || s == "*" {
		return 0, false
	}
	s = strings.TrimPrefix(s, "W/")
	s = strings.Trim(s, `"`)
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ---- DTO <-> model conversion ----

func dtoToSnapshot(dto stateDTO) (store.Snapshot, error) {
	var snap store.Snapshot

	for _, l := range dto.Labels {
		snap.Labels = append(snap.Labels, model.Label{Name: l.Name, Color: l.Color, Bg: l.Bg, Fg: l.Fg})
	}
	for _, a := range dto.Apps {
		app := model.App{
			ID: a.ID, Name: a.Name, Kind: a.Kind, Color: a.Color, Blurb: a.Blurb,
			Scopes: a.Scopes, Status: model.AppStatus(a.Status), InstalledAt: fromMillis(a.InstalledAt),
		}
		if a.Action != nil {
			app.Action = &model.AppAction{Label: a.Action.Label, Verb: a.Action.Verb}
		}
		snap.Apps = append(snap.Apps, app)
	}
	for _, n := range dto.Notches {
		events, err := eventsFromJSON(n.Events)
		if err != nil {
			return store.Snapshot{}, err
		}
		parentID := ""
		if n.ParentID != nil {
			parentID = *n.ParentID
		}
		snap.Notches = append(snap.Notches, store.NotchState{
			Notch: model.Notch{
				ID: n.ID, Title: n.Title, Body: n.Body, Tags: n.Tags, ParentID: parentID,
				Status: model.NotchStatus(n.Status), CreatedAt: fromMillis(n.CreatedAt), UpdatedAt: fromMillis(n.UpdatedAt),
			},
			Events: events,
		})
	}
	for _, t := range dto.Tallies {
		events, err := eventsFromJSON(t.Events)
		if err != nil {
			return store.Snapshot{}, err
		}
		p := model.Proposal{
			ID: t.ID, AppID: t.AppID, Title: t.Title, Body: t.Body, Status: model.ProposalStatus(t.Status),
			Changes: t.Changes, LinkedNotches: t.LinkedNotches,
			CreatedAt: fromMillis(t.CreatedAt), UpdatedAt: fromMillis(t.UpdatedAt),
		}
		if t.MergedAt != nil {
			p.MergedAt = fromMillis(*t.MergedAt)
		}
		snap.Proposals = append(snap.Proposals, store.ProposalState{Proposal: p, Events: events})
	}
	for _, r := range dto.Records {
		snap.Records = append(snap.Records, model.Record{
			ID: r.ID, Dataset: r.Dataset, Kind: r.Kind, Summary: r.Summary,
			Name: r.Name, Mime: r.Mime, Size: r.Size, BlobURL: r.BlobURL,
			Source: r.Source, AppID: r.AppID, ProposedBy: r.TalliedFrom, At: fromMillis(r.At),
		})
	}
	return snap, nil
}

func snapshotToDTO(snap store.Snapshot) stateDTO {
	// Emit empty arrays, never null, so the client always sees the fields it
	// expects (a fresh install returns {"labels":[],...}).
	dto := stateDTO{
		Labels:  []labelDTO{},
		Apps:    []appDTO{},
		Notches: []notchDTO{},
		Tallies: []tallyDTO{},
		Records: []recordDTO{},
	}
	for _, l := range snap.Labels {
		dto.Labels = append(dto.Labels, labelDTO{Name: l.Name, Color: l.Color, Bg: l.Bg, Fg: l.Fg})
	}
	for _, a := range snap.Apps {
		app := appDTO{
			ID: a.ID, Name: a.Name, Kind: a.Kind, Color: a.Color, Blurb: a.Blurb,
			Scopes: a.Scopes, Status: string(a.Status), InstalledAt: toMillis(a.InstalledAt),
		}
		if a.Action != nil {
			app.Action = &actionDTO{Label: a.Action.Label, Verb: a.Action.Verb}
		}
		if app.Scopes == nil {
			app.Scopes = []string{}
		}
		dto.Apps = append(dto.Apps, app)
	}
	for _, n := range snap.Notches {
		var parentID *string
		if n.ParentID != "" {
			p := n.ParentID
			parentID = &p
		}
		dto.Notches = append(dto.Notches, notchDTO{
			ID: n.ID, Title: n.Title, Body: n.Body, Tags: orEmpty(n.Tags), Events: eventsToJSON(n.Events),
			ParentID: parentID, Status: string(n.Status),
			CreatedAt: toMillis(n.CreatedAt), UpdatedAt: toMillis(n.UpdatedAt),
		})
	}
	for _, p := range snap.Proposals {
		t := tallyDTO{
			ID: p.ID, Title: p.Title, Body: p.Body, AppID: p.AppID, Status: string(p.Status),
			Changes: orEmptyJSON(p.Changes), LinkedNotches: orEmpty(p.LinkedNotches), Events: eventsToJSON(p.Events),
			CreatedAt: toMillis(p.CreatedAt), UpdatedAt: toMillis(p.UpdatedAt),
		}
		if !p.MergedAt.IsZero() {
			ms := toMillis(p.MergedAt)
			t.MergedAt = &ms
		}
		dto.Tallies = append(dto.Tallies, t)
	}
	for _, r := range snap.Records {
		dto.Records = append(dto.Records, recordDTO{
			ID: r.ID, Dataset: r.Dataset, Kind: r.Kind, Summary: r.Summary,
			Name: r.Name, Mime: r.Mime, Size: r.Size, BlobURL: r.BlobURL,
			Source: r.Source, AppID: r.AppID, TalliedFrom: r.ProposedBy, At: toMillis(r.At),
		})
	}
	return dto
}

// eventsFromJSON splits each flat client event object {id,kind,at,...payload}
// into a model.Event: id/kind/at are lifted out and everything else is preserved
// verbatim as the opaque Payload, so kind-specific fields (a comment's body, an
// attachment's data URL, a tally's author) survive the round-trip untouched.
func eventsFromJSON(raw []json.RawMessage) ([]model.Event, error) {
	var out []model.Event
	for _, r := range raw {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(r, &fields); err != nil {
			return nil, err
		}
		var ev model.Event
		if v, ok := fields["id"]; ok {
			if err := json.Unmarshal(v, &ev.ID); err != nil {
				return nil, err
			}
		}
		if v, ok := fields["kind"]; ok {
			if err := json.Unmarshal(v, &ev.Kind); err != nil {
				return nil, err
			}
		}
		if v, ok := fields["at"]; ok {
			var ms int64
			if err := json.Unmarshal(v, &ms); err != nil {
				return nil, err
			}
			ev.At = fromMillis(ms)
		}
		delete(fields, "id")
		delete(fields, "kind")
		delete(fields, "at")
		payload, err := json.Marshal(fields)
		if err != nil {
			return nil, err
		}
		ev.Payload = payload
		out = append(out, ev)
	}
	return out, nil
}

// eventsToJSON is the inverse: it merges id/kind/at back onto the payload fields
// to rebuild the flat event object the client expects.
func eventsToJSON(events []model.Event) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(events))
	for _, ev := range events {
		fields := map[string]json.RawMessage{}
		if len(ev.Payload) > 0 {
			// Best effort: a malformed payload just yields no extra fields.
			_ = json.Unmarshal(ev.Payload, &fields)
		}
		fields["id"], _ = json.Marshal(ev.ID)
		fields["kind"], _ = json.Marshal(ev.Kind)
		fields["at"], _ = json.Marshal(toMillis(ev.At))
		b, err := json.Marshal(fields)
		if err != nil {
			b = []byte("{}")
		}
		out = append(out, b)
	}
	return out
}

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

func toMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return raw
}
