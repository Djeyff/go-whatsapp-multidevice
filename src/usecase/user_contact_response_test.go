package usecase

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestMyListContactResponseDataKeepsSavedContactMetadata(t *testing.T) {
	jid := types.NewJID("18095550123", types.DefaultUserServer)
	contact := types.ContactInfo{
		FirstName:    "Gregory",
		FullName:     "Gregory Ventana Suriel",
		PushName:     "PUERTAS Y VENTANA SURIEL",
		BusinessName: "Puertas Suriel",
	}

	row := myListContactResponseData(jid, contact)

	if row.JID != jid {
		t.Fatalf("expected JID %s, got %s", jid, row.JID)
	}
	if row.Phone != "18095550123" {
		t.Fatalf("expected phone digits, got %q", row.Phone)
	}
	if row.Name != "Gregory Ventana Suriel" {
		t.Fatalf("expected saved full name to stay backward-compatible name, got %q", row.Name)
	}
	if row.DisplayName != "Gregory Ventana Suriel" {
		t.Fatalf("expected display name from saved full name, got %q", row.DisplayName)
	}
	if row.FirstName != "Gregory" || row.FullName != "Gregory Ventana Suriel" {
		t.Fatalf("expected first/full saved names, got first=%q full=%q", row.FirstName, row.FullName)
	}
	if row.PushName != "PUERTAS Y VENTANA SURIEL" || row.BusinessName != "Puertas Suriel" {
		t.Fatalf("expected provider names to be exposed as metadata, got push=%q business=%q", row.PushName, row.BusinessName)
	}
	if !row.HasSavedName {
		t.Fatalf("expected saved WhatsApp contact name to be marked")
	}
}

func TestMyListContactResponseDataDoesNotPromotePushOnlyName(t *testing.T) {
	jid := types.NewJID("18095550124", types.DefaultUserServer)
	contact := types.ContactInfo{
		PushName:     "Random Push",
		BusinessName: "Random Business",
	}

	row := myListContactResponseData(jid, contact)

	if row.Name != "" || row.DisplayName != "" {
		t.Fatalf("push/business-only contact must not look user-saved, got name=%q display=%q", row.Name, row.DisplayName)
	}
	if row.PushName != "Random Push" || row.BusinessName != "Random Business" {
		t.Fatalf("expected push/business metadata to remain visible")
	}
	if row.HasSavedName {
		t.Fatalf("push/business-only contact must not be marked as saved")
	}
}
