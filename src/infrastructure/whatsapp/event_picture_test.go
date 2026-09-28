package whatsapp

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestBuildPicturePayload(t *testing.T) {
	ts := time.Date(2026, 9, 28, 18, 0, 0, 0, time.UTC)
	phoneJID := types.NewJID("18095550123", types.DefaultUserServer)
	authorJID := types.NewJID("18095550999", types.DefaultUserServer)
	lidJID := types.NewJID("223754944819424", "lid")

	tests := []struct {
		name    string
		evt     *events.Picture
		payload map[string]any
	}{
		{
			name: "photo changed",
			evt: &events.Picture{
				JID:       phoneJID,
				Author:    authorJID,
				Timestamp: ts,
				Remove:    false,
				PictureID: "1635239861",
			},
			payload: map[string]any{
				"jid":        "18095550123@s.whatsapp.net",
				"author":     "18095550999@s.whatsapp.net",
				"timestamp":  "2026-09-28T18:00:00Z",
				"remove":     false,
				"picture_id": "1635239861",
			},
		},
		{
			name: "photo removed omits picture_id",
			evt: &events.Picture{
				JID:       phoneJID,
				Author:    authorJID,
				Timestamp: ts,
				Remove:    true,
				PictureID: "stale",
			},
			payload: map[string]any{
				"jid":       "18095550123@s.whatsapp.net",
				"author":    "18095550999@s.whatsapp.net",
				"timestamp": "2026-09-28T18:00:00Z",
				"remove":    true,
			},
		},
		{
			name: "lid jid keeps lid fields",
			evt: &events.Picture{
				JID:       lidJID,
				Author:    lidJID,
				Timestamp: ts,
				Remove:    false,
				PictureID: "pic-1",
			},
			payload: map[string]any{
				"jid":        "223754944819424@lid",
				"author":     "223754944819424@lid",
				"timestamp":  "2026-09-28T18:00:00Z",
				"remove":     false,
				"picture_id": "pic-1",
				"jid_lid":    "223754944819424@lid",
				"author_lid": "223754944819424@lid",
			},
		},
		{
			name:    "nil evt",
			evt:     nil,
			payload: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPicturePayload(context.Background(), tt.evt, nil)
			if !reflect.DeepEqual(got, tt.payload) {
				t.Fatalf("payload mismatch\n got %#v\nwant %#v", got, tt.payload)
			}
		})
	}
}

func TestHandlePictureForwardsDeviceWebhook(t *testing.T) {
	if log == nil {
		log = waLog.Noop
	}

	originalWebhooks := config.WhatsappWebhook
	config.WhatsappWebhook = nil
	defer func() { config.WhatsappWebhook = originalWebhooks }()

	deviceWebhookURL := "https://device-only-webhook.example.com"
	originalStorageForTest := webhookStorageForTest
	webhookStorageForTest = func(deviceJID string) (*chatstorage.DeviceRecord, error) {
		return &chatstorage.DeviceRecord{
			DeviceID:   deviceJID,
			WebhookURL: &deviceWebhookURL,
		}, nil
	}
	defer func() { webhookStorageForTest = originalStorageForTest }()

	called := make(chan map[string]any, 1)
	originalSubmit := submitWebhookFn
	submitWebhookFn = func(_ context.Context, payload map[string]any, url string, _ *chatstorage.DeviceWebhookConfig) error {
		if url != deviceWebhookURL {
			t.Errorf("unexpected webhook url %s", url)
		}
		called <- payload
		return nil
	}
	defer func() { submitWebhookFn = originalSubmit }()

	evt := &events.Picture{
		JID:       types.NewJID("18095550123", types.DefaultUserServer),
		Author:    types.NewJID("18095550999", types.DefaultUserServer),
		Timestamp: time.Date(2026, 9, 28, 18, 0, 0, 0, time.UTC),
		PictureID: "1635239861",
	}
	handlePicture(context.Background(), evt, "device_1", nil)

	select {
	case body := <-called:
		if body["event"] != eventTypePicture {
			t.Fatalf("expected event %q, got %#v", eventTypePicture, body["event"])
		}
		payload, _ := body["payload"].(map[string]any)
		if payload["picture_id"] != "1635239861" {
			t.Fatalf("expected picture_id in payload, got %#v", payload)
		}
		if _, ok := payload["url"]; ok {
			t.Fatal("picture webhook must not include image url")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("picture event was not forwarded when only a device webhook is configured")
	}
}
