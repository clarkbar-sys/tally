// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
)

func ptr(s string) *string { return &s }

// sampleSnapshot is a small but shape-complete snapshot: a label with a color
// override and one without, the built-in `you` app plus a connected one, a
// parent/child notch pair with timelines, an open and a merged proposal, and a
// record whose provenance points at the merged proposal.
func sampleSnapshot() Snapshot {
	t := func(ms int64) time.Time { return time.UnixMilli(ms).UTC() }
	return Snapshot{
		Labels: []model.Label{
			{Name: "demo", Color: "blue"},
			{Name: "errand", Color: "green", Bg: ptr("#0a5"), Fg: ptr("#fff")},
		},
		Apps: []model.App{
			{ID: "you", Name: "You", Kind: "you", Color: "blue", Blurb: "you",
				Scopes: []string{"notches:read", "records:propose"}, Status: model.AppActive, InstalledAt: t(1000)},
			{ID: "spotify", Name: "Spotify", Kind: "connected", Color: "green",
				Scopes: []string{"records:propose"}, Action: &model.AppAction{Label: "Sync", Verb: "sync"},
				Status: model.AppActive, InstalledAt: t(2000)},
		},
		Notches: []NotchState{
			{Notch: model.Notch{ID: "n_parent", Title: "Parent", Body: "b", Tags: []string{"demo"},
				Status: model.NotchOpen, CreatedAt: t(3000), UpdatedAt: t(3500)},
				Events: []model.Event{
					{ID: "e1", Kind: "opened", At: t(3000), Payload: json.RawMessage(`{}`)},
					{ID: "e2", Kind: "comment", At: t(3200), Payload: json.RawMessage(`{"body":"hi","deleted":true}`)},
				}},
			{Notch: model.Notch{ID: "n_child", Title: "Child", ParentID: "n_parent",
				Status: model.NotchDone, CreatedAt: t(4000), UpdatedAt: t(4000)},
				Events: []model.Event{{ID: "e3", Kind: "opened", At: t(4000), Payload: json.RawMessage(`{}`)}}},
		},
		Proposals: []ProposalState{
			{Proposal: model.Proposal{ID: "t_open", AppID: "spotify", Title: "Import", Status: model.ProposalOpen,
				Changes:   json.RawMessage(`[{"op":"add-records","dataset":"d","rows":[{"summary":"x"}]}]`),
				CreatedAt: t(5000), UpdatedAt: t(5000)},
				Events: []model.Event{{ID: "e4", Kind: "opened", At: t(5000), Payload: json.RawMessage(`{"author":"Spotify"}`)}}},
			{Proposal: model.Proposal{ID: "t_merged", AppID: "you", Title: "Seed", Status: model.ProposalMerged,
				LinkedNotches: []string{"n_child"}, CreatedAt: t(6000), UpdatedAt: t(6500), MergedAt: t(6500)},
				Events: []model.Event{{ID: "e5", Kind: "merged", At: t(6500), Payload: json.RawMessage(`{"changes":1}`)}}},
		},
		Records: []model.Record{
			{ID: "r1", Dataset: "d", Kind: "text", Summary: "x", Source: "You",
				AppID: "you", ProposedBy: "t_merged", At: t(6600)},
		},
	}
}

func TestSaveLoadStateRoundTrips(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	want := sampleSnapshot()
	if err := s.SaveState(ctx, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := s.LoadState(ctx)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if !reflect.DeepEqual(normalize(want), normalize(got)) {
		t.Fatalf("snapshot did not round-trip.\n want: %+v\n got:  %+v", normalize(want), normalize(got))
	}
}

func TestSaveStateReplacesPriorState(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	if err := s.SaveState(ctx, sampleSnapshot()); err != nil {
		t.Fatalf("SaveState first: %v", err)
	}
	// A second save with only the `you` app must wipe everything else.
	next := Snapshot{Apps: []model.App{{ID: "you", Name: "You", Kind: "you",
		Status: model.AppActive, InstalledAt: time.UnixMilli(1000).UTC()}}}
	if err := s.SaveState(ctx, next); err != nil {
		t.Fatalf("SaveState second: %v", err)
	}

	got, err := s.LoadState(ctx)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(got.Notches) != 0 || len(got.Proposals) != 0 || len(got.Records) != 0 || len(got.Labels) != 0 {
		t.Fatalf("replace left stale rows: %+v", got)
	}
	if len(got.Apps) != 1 || got.Apps[0].ID != "you" {
		t.Fatalf("apps = %+v, want just you", got.Apps)
	}
}

func TestLoadStateEmptyIsZero(t *testing.T) {
	s := openTest(t)
	got, err := s.LoadState(context.Background())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(got.Apps) != 0 || len(got.Notches) != 0 || len(got.Labels) != 0 ||
		len(got.Proposals) != 0 || len(got.Records) != 0 {
		t.Fatalf("empty DB should load an empty snapshot, got %+v", got)
	}
}

// normalize makes two snapshots comparable regardless of slice ordering and the
// nil-vs-empty-slice distinction, and re-encodes event/change JSON so equivalent
// payloads compare equal. Timestamps are compared via their UnixMilli values,
// which is the precision the client works in.
func normalize(s Snapshot) map[string]any {
	m := map[string]any{}
	labels := map[string]any{}
	for _, l := range s.Labels {
		labels[l.Name] = []any{l.Color, deref(l.Bg), deref(l.Fg)}
	}
	m["labels"] = labels

	apps := map[string]any{}
	for _, a := range s.Apps {
		action := "<nil>"
		if a.Action != nil {
			action = a.Action.Label + "/" + a.Action.Verb
		}
		apps[a.ID] = []any{a.Name, a.Kind, a.Color, a.Blurb, a.Scopes, action, string(a.Status), a.InstalledAt.UnixMilli()}
	}
	m["apps"] = apps

	notches := map[string]any{}
	for _, n := range s.Notches {
		notches[n.ID] = []any{n.Title, n.Body, n.Tags, n.ParentID, string(n.Status),
			n.CreatedAt.UnixMilli(), n.UpdatedAt.UnixMilli(), normEvents(n.Events)}
	}
	m["notches"] = notches

	proposals := map[string]any{}
	for _, p := range s.Proposals {
		proposals[p.ID] = []any{p.AppID, p.Title, p.Body, string(p.Status), normJSON(p.Changes),
			p.LinkedNotches, p.CreatedAt.UnixMilli(), p.UpdatedAt.UnixMilli(), p.MergedAt.UnixMilli(), normEvents(p.Events)}
	}
	m["proposals"] = proposals

	records := map[string]any{}
	for _, r := range s.Records {
		records[r.ID] = []any{r.Dataset, r.Kind, r.Summary, r.Name, r.Mime, r.Size, r.BlobURL,
			r.Source, r.AppID, r.ProposedBy, r.At.UnixMilli()}
	}
	m["records"] = records
	return m
}

func normEvents(events []model.Event) []any {
	out := make([]any, 0, len(events))
	for _, e := range events {
		out = append(out, []any{e.ID, e.Kind, e.At.UnixMilli(), normJSON(e.Payload)})
	}
	return out
}

// normJSON canonicalizes an engine-owned payload so equivalent values compare
// equal: nil, "[]" and "{}" all collapse to "" (an unset diff round-trips as the
// column default "[]", which is equivalent to having sent nothing).
func normJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	switch t := v.(type) {
	case nil:
		return ""
	case []any:
		if len(t) == 0 {
			return ""
		}
	case map[string]any:
		if len(t) == 0 {
			return ""
		}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
