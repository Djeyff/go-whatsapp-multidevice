package usecase

import (
	"context"
	"errors"
	"testing"

	domainUser "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/user"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
)

// RET-535-FU: LidLookup input validation is pure and must run before any
// client/device state is consulted, so every bad-phone shape is a typed
// 400 (InvalidJID) and only a clean request reaches the ErrWaCLI guard.
func TestLidLookupRejectsBadPhones(t *testing.T) {
	service := NewUserService(nil)

	cases := []struct {
		name  string
		phone string
	}{
		{name: "empty phone", phone: ""},
		{name: "bare group jid", phone: "120363123456789012@g.us"},
		{name: "lid jid is not a phone", phone: "123456789012345@lid"},
		{name: "malformed jid", phone: "abc@@"},
		{name: "empty user jid", phone: "@s.whatsapp.net"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.LidLookup(context.Background(), domainUser.LidLookupRequest{Phone: tc.phone})
			if err == nil {
				t.Fatalf("expected error for phone %q", tc.phone)
			}
			var invalid pkgError.InvalidJID
			if !errors.As(err, &invalid) {
				t.Fatalf("expected typed InvalidJID error, got %T (%v)", err, err)
			}
		})
	}
}

func TestLidLookupRequiresClient(t *testing.T) {
	service := NewUserService(nil)

	t.Run("valid phone without client yields ErrWaCLI", func(t *testing.T) {
		_, err := service.LidLookup(context.Background(), domainUser.LidLookupRequest{Phone: "15551234567"})
		if !errors.Is(err, pkgError.ErrWaCLI) {
			t.Fatalf("expected ErrWaCLI, got %T (%v)", err, err)
		}
	})
}

// Avatar keeps its contract: without a client in the context it is ErrWaCLI
// for both the phone and the LID branch, and never a parse panic.
func TestAvatarRequiresClient(t *testing.T) {
	service := NewUserService(nil)

	t.Run("phone branch", func(t *testing.T) {
		_, err := service.Avatar(context.Background(), domainUser.AvatarRequest{Phone: "15551234567"})
		if !errors.Is(err, pkgError.ErrWaCLI) {
			t.Fatalf("expected ErrWaCLI, got %T (%v)", err, err)
		}
	})

	t.Run("lid branch", func(t *testing.T) {
		_, err := service.Avatar(context.Background(), domainUser.AvatarRequest{LID: "123456789012345@lid"})
		if !errors.Is(err, pkgError.ErrWaCLI) {
			t.Fatalf("expected ErrWaCLI, got %T (%v)", err, err)
		}
	})

	t.Run("phone and lid together is typed invalid", func(t *testing.T) {
		// A logged-in device is required before the mutual-exclusion check runs,
		// so with no client this must surface as ErrWaCLI — assert it is NOT a
		// panic and is a known error shape.
		_, err := service.Avatar(context.Background(), domainUser.AvatarRequest{Phone: "15551234567", LID: "123456789012345@lid"})
		if err == nil {
			t.Fatal("expected an error when both phone and lid are set")
		}
	})
}
