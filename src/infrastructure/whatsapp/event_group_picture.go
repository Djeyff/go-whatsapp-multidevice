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
// Mirrors whatsmeow's own picture-notification parsing: a "delete" marker means
// the photo was removed, while "picture" (and picture-ish "add"/"set") nodes
// carry the new picture id in their "id" attribute. Unrelated unknown nodes
// yield no event, so ordinary group changes never emit a picture webhook.
func extractGroupPictureChange(unknownChanges []*waBinary.Node) (remove bool, pictureID string, found bool) {
	for _, node := range unknownChanges {
		if node == nil {
			continue
		}

		if node.Tag == "delete" {
			return true, "", true
		}
		if node.Tag != "picture" && node.Tag != "add" && node.Tag != "set" {
			continue
		}

		id, _ := node.Attrs["id"].(string)

		// A delete marker may ride as a nested child instead of the node tag.
		if id == "" {
			if _, hasDelete := node.GetOptionalChildByTag("delete"); hasDelete {
				return true, "", true
			}
		}

		return false, id, true
	}
	return false, "", false
}
