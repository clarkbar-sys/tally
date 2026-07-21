// SPDX-License-Identifier: GPL-2.0-or-later

package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
)

func testApp() model.App {
	return model.App{
		ID:     "spotify-demo",
		Name:   "Spotify (demo)",
		Kind:   "connected",
		Color:  "green",
		Blurb:  "A connected music provider.",
		Scopes: []string{"records:propose"},
		Action: &model.AppAction{Label: "Simulate a sync", Verb: "spotify-sync"},
		Status: model.AppActive,
	}
}

// seedYou registers the built-in "you" app, the default author for hand-opened
// proposals — a fixture the proposal/record tests build on.
func seedYou(t *testing.T, ctx context.Context, s *Store) model.App {
	t.Helper()
	you, err := s.UpsertApp(ctx, model.App{
		ID: "you", Name: "You", Kind: "you", Color: "blue",
		Scopes: []string{"notches:read", "notches:propose", "records:read", "records:propose"},
		Status: model.AppActive,
	})
	if err != nil {
		t.Fatalf("UpsertApp(you): %v", err)
	}
	return you
}

func TestUpsertAppInsertsThenUpdatesInPlace(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	inserted, err := s.UpsertApp(ctx, testApp())
	if err != nil {
		t.Fatalf("UpsertApp (insert): %v", err)
	}
	if inserted.InstalledAt.IsZero() {
		t.Fatal("inserted app missing installed_at")
	}

	// Revoking and re-blurbing the same app must update the row in place and
	// preserve the original installation time (re-registering isn't reinstalling).
	changed := testApp()
	changed.Status = model.AppRevoked
	changed.Blurb = "Revoked provider."
	updated, err := s.UpsertApp(ctx, changed)
	if err != nil {
		t.Fatalf("UpsertApp (update): %v", err)
	}
	if !updated.InstalledAt.Equal(inserted.InstalledAt) {
		t.Fatalf("InstalledAt changed on update: got %v, want %v", updated.InstalledAt, inserted.InstalledAt)
	}

	got, err := s.GetApp(ctx, "spotify-demo")
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Status != model.AppRevoked {
		t.Fatalf("Status = %q, want %q", got.Status, model.AppRevoked)
	}
	if got.Action == nil || got.Action.Verb != "spotify-sync" {
		t.Fatalf("Action = %+v, want verb spotify-sync", got.Action)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "records:propose" {
		t.Fatalf("Scopes = %v, want [records:propose]", got.Scopes)
	}

	apps, err := s.ListApps(ctx)
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("ListApps returned %d apps, want 1 (re-register must not duplicate)", len(apps))
	}
}

func TestAppWithoutActionStoresNull(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	you := seedYou(t, ctx, s) // seeded with Action nil
	if you.Action != nil {
		t.Fatalf("seed app Action = %+v, want nil", you.Action)
	}
	got, err := s.GetApp(ctx, "you")
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Action != nil {
		t.Fatalf("Action = %+v, want nil round-trip", got.Action)
	}
}

func TestUpsertNotchAndTimeline(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	parent, err := s.UpsertNotch(ctx, model.Notch{
		ID: "n_parent", Title: "Welcome", Body: "hi", Tags: []string{"demo"}, Status: model.NotchOpen,
	})
	if err != nil {
		t.Fatalf("UpsertNotch (parent): %v", err)
	}
	if parent.CreatedAt.IsZero() || parent.UpdatedAt.IsZero() {
		t.Fatal("parent notch missing timestamps")
	}

	// The child carries a due date, so the round-trip covers due_at as well.
	due := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	if _, err := s.UpsertNotch(ctx, model.Notch{
		ID: "n_child", Title: "Sub", ParentID: "n_parent", Status: model.NotchOpen, DueAt: due,
	}); err != nil {
		t.Fatalf("UpsertNotch (child): %v", err)
	}

	// Re-parenting-free update in place: change the status, keep created_at.
	done := model.Notch{ID: "n_parent", Title: "Welcome", Body: "hi", Tags: []string{"demo"}, Status: model.NotchDone}
	updated, err := s.UpsertNotch(ctx, done)
	if err != nil {
		t.Fatalf("UpsertNotch (update): %v", err)
	}
	if !updated.CreatedAt.Equal(parent.CreatedAt) {
		t.Fatalf("CreatedAt changed on update: got %v, want %v", updated.CreatedAt, parent.CreatedAt)
	}

	got, err := s.GetNotch(ctx, "n_parent")
	if err != nil {
		t.Fatalf("GetNotch: %v", err)
	}
	if got.Status != model.NotchDone {
		t.Fatalf("Status = %q, want %q", got.Status, model.NotchDone)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "demo" {
		t.Fatalf("Tags = %v, want [demo]", got.Tags)
	}

	children, err := s.ListChildNotches(ctx, "n_parent")
	if err != nil {
		t.Fatalf("ListChildNotches: %v", err)
	}
	if len(children) != 1 || children[0].ID != "n_child" {
		t.Fatalf("ListChildNotches = %+v, want [n_child]", children)
	}
	if children[0].ParentID != "n_parent" {
		t.Fatalf("child ParentID = %q, want n_parent", children[0].ParentID)
	}
	if !children[0].DueAt.Equal(due) {
		t.Fatalf("child DueAt = %v, want %v", children[0].DueAt, due)
	}
	// The parent has no due date, so it round-trips as the zero time (SQL NULL).
	if !got.DueAt.IsZero() {
		t.Fatalf("parent DueAt = %v, want zero", got.DueAt)
	}

	// Append-only timeline, returned in append order.
	for _, k := range []string{"opened", "labeled", "status"} {
		if _, err := s.AppendNotchEvent(ctx, "n_parent", model.Event{
			ID: "e_" + k, Kind: k, Payload: json.RawMessage(`{"by":"You"}`),
		}); err != nil {
			t.Fatalf("AppendNotchEvent(%s): %v", k, err)
		}
	}
	events, err := s.ListNotchEvents(ctx, "n_parent")
	if err != nil {
		t.Fatalf("ListNotchEvents: %v", err)
	}
	if len(events) != 3 || events[0].Kind != "opened" || events[2].Kind != "status" {
		t.Fatalf("timeline = %+v, want opened,labeled,status in order", events)
	}
	if got := string(events[0].Payload); got != `{"by":"You"}` {
		t.Fatalf("event payload = %s, want {\"by\":\"You\"}", got)
	}
	if events[0].At.IsZero() {
		t.Fatal("appended event missing at timestamp")
	}
}

func TestProposalLifecycleAndEvents(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	seedYou(t, ctx, s)

	if _, err := s.UpsertNotch(ctx, model.Notch{ID: "n_list", Title: "Shopping", Status: model.NotchOpen}); err != nil {
		t.Fatalf("UpsertNotch: %v", err)
	}

	open, err := s.UpsertProposal(ctx, model.Proposal{
		ID: "t_wrap", AppID: "you", Title: "Wrap up the shopping list", Status: model.ProposalOpen,
		Changes:       json.RawMessage(`[{"op":"comment","notchId":"n_list","body":"done"}]`),
		LinkedNotches: []string{"n_list"},
	})
	if err != nil {
		t.Fatalf("UpsertProposal (open): %v", err)
	}
	if !open.MergedAt.IsZero() {
		t.Fatalf("MergedAt = %v, want zero for an open proposal", open.MergedAt)
	}

	// Merge: same id, status advances, merged_at set, created_at preserved.
	merged := model.Proposal{
		ID: "t_wrap", AppID: "you", Title: "Wrap up the shopping list", Status: model.ProposalMerged,
		Changes: open.Changes, LinkedNotches: []string{"n_list"},
		MergedAt: open.CreatedAt, // any non-zero time
	}
	got, err := s.UpsertProposal(ctx, merged)
	if err != nil {
		t.Fatalf("UpsertProposal (merge): %v", err)
	}
	if !got.CreatedAt.Equal(open.CreatedAt) {
		t.Fatalf("CreatedAt changed on merge: got %v, want %v", got.CreatedAt, open.CreatedAt)
	}

	reloaded, err := s.GetProposal(ctx, "t_wrap")
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if reloaded.Status != model.ProposalMerged {
		t.Fatalf("Status = %q, want %q", reloaded.Status, model.ProposalMerged)
	}
	if reloaded.MergedAt.IsZero() {
		t.Fatal("MergedAt still zero after merge")
	}
	if string(reloaded.Changes) != `[{"op":"comment","notchId":"n_list","body":"done"}]` {
		t.Fatalf("Changes = %s, want the stored diff verbatim", reloaded.Changes)
	}

	// The notch↔PR link reads from the notch's end.
	forNotch, err := s.ListProposalsForNotch(ctx, "n_list")
	if err != nil {
		t.Fatalf("ListProposalsForNotch: %v", err)
	}
	if len(forNotch) != 1 || forNotch[0].ID != "t_wrap" {
		t.Fatalf("ListProposalsForNotch = %+v, want [t_wrap]", forNotch)
	}
	if none, _ := s.ListProposalsForNotch(ctx, "n_other"); len(none) != 0 {
		t.Fatalf("ListProposalsForNotch(n_other) = %+v, want none", none)
	}

	// Timeline.
	if _, err := s.AppendProposalEvent(ctx, "t_wrap", model.Event{ID: "e_open", Kind: "opened"}); err != nil {
		t.Fatalf("AppendProposalEvent(opened): %v", err)
	}
	if _, err := s.AppendProposalEvent(ctx, "t_wrap", model.Event{ID: "e_merge", Kind: "merged"}); err != nil {
		t.Fatalf("AppendProposalEvent(merged): %v", err)
	}
	events, err := s.ListProposalEvents(ctx, "t_wrap")
	if err != nil {
		t.Fatalf("ListProposalEvents: %v", err)
	}
	if len(events) != 2 || events[0].Kind != "opened" || events[1].Kind != "merged" {
		t.Fatalf("timeline = %+v, want opened,merged in order", events)
	}
}

func TestInsertRecordCarriesProvenance(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	seedYou(t, ctx, s)
	if _, err := s.UpsertProposal(ctx, model.Proposal{ID: "t_done", AppID: "you", Status: model.ProposalMerged}); err != nil {
		t.Fatalf("UpsertProposal: %v", err)
	}

	rec, err := s.InsertRecord(ctx, model.Record{
		ID: "r_1", Dataset: "pantry.items", Kind: model.RecordText, Summary: "Olive oil ×1",
		Source: "You", AppID: "you", ProposedBy: "t_done",
	})
	if err != nil {
		t.Fatalf("InsertRecord: %v", err)
	}
	if rec.At.IsZero() {
		t.Fatal("inserted record missing at timestamp")
	}

	blob, err := s.InsertRecord(ctx, model.Record{
		ID: "r_2", Dataset: "webclip.files", Kind: model.RecordBlob,
		Name: "diagram.svg", Mime: "image/svg+xml", Size: 512, BlobURL: "data:image/svg+xml,x",
		Source: "You", AppID: "you", ProposedBy: "t_done",
	})
	if err != nil {
		t.Fatalf("InsertRecord (blob): %v", err)
	}

	byDataset, err := s.ListRecordsByDataset(ctx, "pantry.items")
	if err != nil {
		t.Fatalf("ListRecordsByDataset: %v", err)
	}
	if len(byDataset) != 1 || byDataset[0].Summary != "Olive oil ×1" {
		t.Fatalf("ListRecordsByDataset = %+v, want [Olive oil ×1]", byDataset)
	}

	byProposal, err := s.ListRecordsByProposal(ctx, "t_done")
	if err != nil {
		t.Fatalf("ListRecordsByProposal: %v", err)
	}
	if len(byProposal) != 2 {
		t.Fatalf("ListRecordsByProposal returned %d, want 2", len(byProposal))
	}
	for _, r := range byProposal {
		if r.AppID != "you" || r.ProposedBy != "t_done" {
			t.Fatalf("record %s lost provenance: appID=%q proposedBy=%q", r.ID, r.AppID, r.ProposedBy)
		}
	}
	if blob.BlobURL == "" {
		t.Fatal("blob record lost its blob_url")
	}
}

func TestProtocolForeignKeysEnforced(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	// A proposal authored by an unregistered app is rejected.
	if _, err := s.UpsertProposal(ctx, model.Proposal{ID: "t_x", AppID: "ghost", Status: model.ProposalOpen}); err == nil {
		t.Fatal("UpsertProposal with a nonexistent app_id should fail the foreign key check")
	}

	// A record whose proposal doesn't exist is rejected.
	seedYou(t, ctx, s)
	if _, err := s.InsertRecord(ctx, model.Record{
		ID: "r_x", Dataset: "d", Kind: model.RecordText, AppID: "you", ProposedBy: "t_missing",
	}); err == nil {
		t.Fatal("InsertRecord with a nonexistent proposed_by should fail the foreign key check")
	}
}
