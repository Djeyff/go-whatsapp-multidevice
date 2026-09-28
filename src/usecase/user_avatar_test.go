package usecase

import (
	"testing"

	domainUser "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/user"
	"go.mau.fi/whatsmeow/types"
)

func TestAvatarResponseFromPicture(t *testing.T) {
	t.Run("unchanged when existing_id and nil pic", func(t *testing.T) {
		got, err := avatarResponseFromPicture(nil, "1635239861")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := domainUser.AvatarResponse{Unchanged: true}
		if got != want {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})

	t.Run("no avatar found without existing_id", func(t *testing.T) {
		_, err := avatarResponseFromPicture(nil, "")
		if err == nil || err.Error() != "no avatar found" {
			t.Fatalf("expected no avatar found, got %v", err)
		}
	})

	t.Run("maps url id type", func(t *testing.T) {
		got, err := avatarResponseFromPicture(&types.ProfilePictureInfo{
			URL:  "https://pps.whatsapp.net/a.jpg",
			ID:   "new-id",
			Type: "preview",
		}, "old-id")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.Unchanged {
			t.Fatal("changed picture must not set unchanged")
		}
		if got.URL == "" || got.ID != "new-id" || got.Type != "preview" {
			t.Fatalf("unexpected mapping %#v", got)
		}
	})

	t.Run("whitespace existing_id is not unchanged", func(t *testing.T) {
		_, err := avatarResponseFromPicture(nil, "   ")
		if err == nil || err.Error() != "no avatar found" {
			t.Fatalf("expected no avatar found for whitespace existing_id, got %v", err)
		}
	})
}
