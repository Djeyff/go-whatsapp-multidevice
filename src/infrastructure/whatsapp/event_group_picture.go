package whatsapp

import (
	"context"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// handleGroupPictureChange detects a group photo change inside an events.GroupInfo
// and emits the same picture webhook the profile-picture path already sends.
//
// Group photo updates ride the w:gp2 group notification and land in
// events.GroupInfo.UnknownChanges — WhatsApp does not send the separate "picture"
// notification type for groups, so this is the only place the event can be created.
//
// Fire-and-forget: never blocks or panics the notification loop.
func handleGroupPictureChange(ctx context.Context, evt *events.GroupInfo, deviceID string, client *whatsmeow.Client) {
	if evt == nil || evt.JID.Server != types.GroupServer {
		return
	}

	remove, pictureID, ok := extractGroupPictureChange(evt.UnknownChanges)
	if !ok {
		return
	}

	var author types.JID
	if evt.Sender != nil {
		author = *evt.Sender
	}
	timestamp := evt.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	pic := &events.Picture{
		JID:       evt.JID,
		Author:    author,
		Timestamp: timestamp,
		Remove:    remove,
		PictureID: pictureID,
	}

	logrus.Infof("Group %s photo changed (remove=%v, picture_id=%q)", evt.JID, remove, pictureID)

	// Same guard as handlePicture: nothing to deliver without webhooks or a device binding.
	if len(config.WhatsappWebhook) == 0 && strings.TrimSpace(deviceID) == "" {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("Recovered from panic forwarding group picture event: %v", r)
			}
		}()
		webhookCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := forwardPictureToWebhook(webhookCtx, pic, deviceID, client); err != nil {
			logrus.Errorf("Failed to forward group picture event to webhook: %v", err)
		}
	}()
}

// extractGroupPictureChange scans the unknown w:gp2 change nodes for a picture
// change. Returns remove, the new picture id ("" when unknown), and whether a
// picture change was found.
//
// Typed shapes first: a "delete" marker means the photo was removed; "picture"
// / "add" / "set" nodes carry the new picture id in their "id" attribute.
//
// Fall back to the spec's fetch-on-event path: WhatsApp's live wire shape for
// group photo changes does not always match the typed picture nodes (observed
// live 2026-10-12: a Familia Prueba photo change produced zero matches). When
// the notification carries unknown change(s) but NO typed change of any kind,
// treat it as a possible photo change and emit with no picture id — the
// consumer fetches the avatar on the event itself. The api layer debounces
// per group (60s window), so occasional spurious emissions are cheap; a photo
// change must never be swallowed for want of a guessed node shape.
func extractGroupPictureChange(unknownChanges []*waBinary.Node) (remove bool, pictureID string, found bool) {
	sawUnknown := false
	for _, node := range unknownChanges {
		if node == nil {
			continue
		}
		sawUnknown = true

		if node.Tag == "delete" {
			return true, "", true
		}
		if node.Tag == "picture" || node.Tag == "add" || node.Tag == "set" {
			id, _ := node.Attrs["id"].(string)

			// A delete marker may ride as a nested child instead of the node tag.
			if id == "" {
				if _, hasDelete := node.GetOptionalChildByTag("delete"); hasDelete {
					return true, "", true
				}
			}

			return false, id, true
		}
	}

	// Shape-agnostic fallback: unknown change(s) present, no typed change matched.
	if sawUnknown {
		return false, "", true
	}
	return false, "", false
}
