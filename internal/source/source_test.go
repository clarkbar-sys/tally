// SPDX-License-Identifier: GPL-2.0-or-later

package source_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/clarkbar-sys/tally/internal/model"
	"github.com/clarkbar-sys/tally/internal/source"
)

// fakeAdapter is a minimal in-test Adapter. Two of these, registered under
// different names, stand in for "two providers" in the acceptance test below.
type fakeAdapter struct {
	name string
	snap *source.Snapshot
	err  error
}

func (f *fakeAdapter) Name() string { return f.name }

func (f *fakeAdapter) Fetch(context.Context, time.Time) (*source.Snapshot, error) {
	return f.snap, f.err
}

func fakeFactory(name string) source.Factory {
	return func(context.Context) (source.Adapter, error) {
		return &fakeAdapter{
			name: name,
			snap: &source.Snapshot{Accounts: []source.AccountSnapshot{{
				Account: model.Account{Provider: name, ProviderAccountID: "a1"},
			}}},
		}, nil
	}
}

// TestSecondProviderIsAdditive is the #8 acceptance criterion: adding a second
// provider is a registration line against the same interface — no existing
// registration changes, and both open and produce their own snapshots through
// the one boundary.
func TestSecondProviderIsAdditive(t *testing.T) {
	ctx := context.Background()
	reg := source.NewRegistry()

	if err := reg.Register("alpha", fakeFactory("alpha")); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	// The "one registration line" for a second provider. Nothing above changed.
	if err := reg.Register("beta", fakeFactory("beta")); err != nil {
		t.Fatalf("register beta: %v", err)
	}

	if got, want := reg.Names(), []string{"alpha", "beta"}; !equal(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}

	for _, name := range []string{"alpha", "beta"} {
		a, err := reg.Open(ctx, name)
		if err != nil {
			t.Fatalf("Open(%q): %v", name, err)
		}
		if a.Name() != name {
			t.Fatalf("adapter Name() = %q, want %q", a.Name(), name)
		}
		snap, err := a.Fetch(ctx, time.Time{})
		if err != nil {
			t.Fatalf("Fetch(%q): %v", name, err)
		}
		if len(snap.Accounts) != 1 || snap.Accounts[0].Account.Provider != name {
			t.Fatalf("Fetch(%q) snapshot = %+v, want one account stamped %q", name, snap, name)
		}
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	reg := source.NewRegistry()
	if err := reg.Register("dup", fakeFactory("dup")); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := reg.Register("dup", fakeFactory("dup")); err == nil {
		t.Fatal("second register of same name should error")
	}
}

func TestRegisterRejectsEmptyAndNil(t *testing.T) {
	reg := source.NewRegistry()
	if err := reg.Register("", fakeFactory("x")); err == nil {
		t.Fatal("empty name should error")
	}
	if err := reg.Register("nilf", nil); err == nil {
		t.Fatal("nil factory should error")
	}
}

func TestOpenUnknownProvider(t *testing.T) {
	reg := source.NewRegistry()
	if _, err := reg.Open(context.Background(), "nope"); err == nil {
		t.Fatal("Open of unregistered provider should error")
	}
}

func TestOpenPropagatesFactoryError(t *testing.T) {
	reg := source.NewRegistry()
	sentinel := errors.New("boom")
	if err := reg.Register("bad", func(context.Context) (source.Adapter, error) {
		return nil, sentinel
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := reg.Open(context.Background(), "bad")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open error = %v, want it to wrap %v", err, sentinel)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
