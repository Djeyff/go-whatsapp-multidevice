package utils

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

// fakeLIDStore records every pn user queried and answers from a static
// digits->lid table, mirroring whatsmeow_lid_map semantics (exact string
// match on the pn user, E.164-without-plus rows written by WhatsApp).
type fakeLIDStore struct {
	table   map[string]string
	lookups []string
}

func (f *fakeLIDStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	f.lookups = append(f.lookups, pn.User)
	if lid, ok := f.table[pn.User]; ok {
		return types.NewJID(lid, "lid"), nil
	}
	return types.JID{}, nil
}

func (f *fakeLIDStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	return types.JID{}, nil
}

func (f *fakeLIDStore) PutLIDMapping(ctx context.Context, lid, jid types.JID) error {
	return nil
}

func (f *fakeLIDStore) PutManyLIDMappings(ctx context.Context, mappings []store.LIDMapping) error {
	return nil
}

func (f *fakeLIDStore) GetManyLIDsForPNs(ctx context.Context, pns []types.JID) (map[types.JID]types.JID, error) {
	return map[types.JID]types.JID{}, nil
}

func lidTestClient(tbl map[string]string, ownUser string) (*whatsmeow.Client, *fakeLIDStore) {
	f := &fakeLIDStore{table: tbl}
	dev := &store.Device{LIDs: f}
	if ownUser != "" {
		jid := types.NewJID(ownUser, types.DefaultUserServer)
		dev.ID = &jid
	}
	return &whatsmeow.Client{Store: dev}, f
}

// RET-613: exact E.164 query must hit on the first lookup (behavior preserved).
func TestResolvePhoneToLID_ExactE164(t *testing.T) {
	client, f := lidTestClient(map[string]string{"18292953856": "158785544949956"}, "18291234567")
	q := types.NewJID("18292953856", types.DefaultUserServer)
	lid := ResolvePhoneToLID(context.Background(), q, client)
	if lid.Server != "lid" || lid.User != "158785544949956" {
		t.Fatalf("ResolvePhoneToLID exact = %v, want lid:158785544949956", lid)
	}
	if len(f.lookups) != 1 || f.lookups[0] != "18292953856" {
		t.Fatalf("lookups = %v, want single exact hit", f.lookups)
	}
}

// RET-613 live-probed class: contact stored national (no CC), device map holds
// E.164 — exact misses, the device's own country code prefix must rescue it.
func TestResolvePhoneToLID_NationalFallsBackToOwnCC(t *testing.T) {
	client, f := lidTestClient(map[string]string{"18292953856": "158785544949956"}, "18291234567")
	q := types.NewJID("8292953856", types.DefaultUserServer) // national, no CC
	lid := ResolvePhoneToLID(context.Background(), q, client)
	if lid.Server != "lid" || lid.User != "158785544949956" {
		t.Fatalf("ResolvePhoneToLID national = %v, want lid:158785544949956", lid)
	}
	if len(f.lookups) < 2 || f.lookups[0] != "8292953856" {
		t.Fatalf("lookups = %v, want exact-first then fallback", f.lookups)
	}
	found := false
	for _, l := range f.lookups[1:] {
		if l == "18292953856" {
			found = true
		}
	}
	if !found {
		t.Fatalf("lookups = %v, want CC-prefixed candidate 18292953856", f.lookups)
	}
}

// No connected device identity: fall back must be skipped entirely, no panic.
func TestResolvePhoneToLID_NoOwnJIDSkipsFallback(t *testing.T) {
	client, f := lidTestClient(map[string]string{"18292953856": "158785544949956"}, "")
	q := types.NewJID("8292953856", types.DefaultUserServer)
	lid := ResolvePhoneToLID(context.Background(), q, client)
	if !lid.IsEmpty() {
		t.Fatalf("ResolvePhoneToLID without own JID = %v, want empty", lid)
	}
	if len(f.lookups) != 1 || f.lookups[0] != "8292953856" {
		t.Fatalf("lookups = %v, want exact-only", f.lookups)
	}
}

// Non-user JIDs must short-circuit untouched.
func TestResolvePhoneToLID_NonUserServer(t *testing.T) {
	client, f := lidTestClient(map[string]string{"18292953856": "158785544949956"}, "18291234567")
	q := types.NewJID("120341234567@g.us", "g.us")
	lid := ResolvePhoneToLID(context.Background(), q, client)
	if !lid.IsEmpty() {
		t.Fatalf("ResolvePhoneToLID group = %v, want empty", lid)
	}
	if len(f.lookups) != 0 {
		t.Fatalf("lookups = %v, want none for non-user JID", f.lookups)
	}
}

// Fully unmapped: empty result, bounded candidate list (no lookup storm).
func TestResolvePhoneToLID_UnmappedBounded(t *testing.T) {
	client, f := lidTestClient(map[string]string{}, "18291234567")
	q := types.NewJID("8292953856", types.DefaultUserServer)
	lid := ResolvePhoneToLID(context.Background(), q, client)
	if !lid.IsEmpty() {
		t.Fatalf("ResolvePhoneToLID unmapped = %v, want empty", lid)
	}
	if len(f.lookups) == 0 || len(f.lookups) > 8 {
		t.Fatalf("lookups = %v, want 1..8 bounded candidates", f.lookups)
	}
}

// Base already carrying the device CC must never be double-prefixed.
func TestLidLookupCandidates_NoDoublePrefix(t *testing.T) {
	cands := lidLookupCandidates("18292953856", "18291234567")
	for _, c := range cands {
		if c != "18292953856" {
			t.Fatalf("candidates = %v, want exact only (no CC double-prefix)", cands)
		}
	}
}

// Digits-only normalization: noisy input resolves to the clean base candidate.
func TestLidLookupCandidates_DigitsOnlyBase(t *testing.T) {
	cands := lidLookupCandidates("+1 (829) 295-3856", "")
	if len(cands) == 0 || cands[0] != "18292953856" {
		t.Fatalf("candidates = %v, want base 18292953856 first", cands)
	}
}

// Junk / oversized input yields no candidates at all.
func TestLidLookupCandidates_JunkAndOversize(t *testing.T) {
	if got := lidLookupCandidates("not-a-number", "18291234567"); got != nil {
		t.Fatalf("junk candidates = %v, want nil", got)
	}
	if got := lidLookupCandidates("12345678901234567890", "18291234567"); got != nil {
		t.Fatalf("oversize candidates = %v, want nil", got)
	}
}
