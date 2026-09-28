package whatsapp

import (
	"context"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const eventTypePicture = "picture"

func handlePicture(_ context.Context, evt *events.Picture, deviceID string, client *whatsmeow.Client) {
	if evt == nil {
		return
	}
	if len(config.WhatsappWebhook) == 0 && strings.TrimSpace(deviceID) == "" {
		return
	}

	go func() {
		webhookCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := forwardPictureToWebhook(webhookCtx, evt, deviceID, client); err != nil {
			logrus.Errorf("Failed to forward picture event to webhook: %v", err)
		}
	}()
}

func forwardPictureToWebhook(ctx context.Context, evt *events.Picture, deviceID string, client *whatsmeow.Client) error {
	if evt == nil {
		return nil
	}

	body := map[string]any{
		"event":     eventTypePicture,
		"timestamp": evt.Timestamp.Format(time.RFC3339),
		"payload":   buildPicturePayload(ctx, evt, client),
	}
	if deviceID != "" {
		body["device_id"] = deviceID
	}

	return forwardPayloadToConfiguredWebhooks(ctx, body, eventTypePicture)
}

func buildPicturePayload(ctx context.Context, evt *events.Picture, client *whatsmeow.Client) map[string]any {
	if evt == nil {
		return map[string]any{}
	}

	jid := normalizePictureJID(ctx, evt.JID, client)
	author := normalizePictureJID(ctx, evt.Author, client)
	payload := map[string]any{
		"jid":       jid.ToNonAD().String(),
		"author":    author.ToNonAD().String(),
		"timestamp": evt.Timestamp.Format(time.RFC3339),
		"remove":    evt.Remove,
	}
	if !evt.Remove && evt.PictureID != "" {
		payload["picture_id"] = evt.PictureID
	}
	if evt.JID.Server == "lid" {
		payload["jid_lid"] = evt.JID.ToNonAD().String()
	}
	if evt.Author.Server == "lid" {
		payload["author_lid"] = evt.Author.ToNonAD().String()
	}
	return payload
}

func normalizePictureJID(ctx context.Context, jid types.JID, client *whatsmeow.Client) types.JID {
	if jid.IsEmpty() || client == nil {
		return jid
	}
	return NormalizeJIDFromLID(ctx, jid, client)
}
