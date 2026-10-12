package whatsapp

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestExtractGroupPictureChange(t *testing.T) {
	pictureNode := func(tag string, attrs waBinary.Attrs, children ...waBinary.Node) *waBinary.Node {
		node := &waBinary.Node{Tag: tag, Attrs: attrs}
		if len(children) > 0 {
			node.Content = children
		}
		return node
	}

	tests := []struct {
		name           string
		unknownChanges []*waBinary.Node
		wantFound      bool
		wantRemove     bool
		wantPictureID  string
	}{
		{
			name: "picture node with id",
			unknownChanges: []*waBinary.Node{
				pictureNode("picture", waBinary.Attrs{"id": "1635239861", "type": "image"}),
			},
			wantFound:     true,
			wantRemove:    false,
			wantPictureID: "1635239861",
		},
		{
			name: "picture node without id still emits",
			unknownChanges: []*waBinary.Node{
				pictureNode("picture", waBinary.Attrs{"type": "image"}),
			},
			wantFound:     true,
			wantRemove:    false,
			wantPictureID: "",
		},
		{
			name: "delete marker node means removed",
			unknownChanges: []*waBinary.Node{
				pictureNode("delete", nil),
			},
			wantFound:  true,
			wantRemove: true,
		},
		{
			name: "delete marker nested in picture node",
			unknownChanges: []*waBinary.Node{
				pictureNode("picture", nil, waBinary.Node{Tag: "delete"}),
			},
			wantFound:  true,
			wantRemove: true,
		},
		{
			name: "unrelated unknown nodes trigger fetch-on-event fallback",
			unknownChanges: []*waBinary.Node{
				pictureNode("membership_approval_mode", waBinary.Attrs{}),
				pictureNode("foo", waBinary.Attrs{"bar": "baz"}),
			},
			wantFound:     true,
			wantRemove:    false,
			wantPictureID: "",
		},
		{
			name:           "no unknown changes",
			unknownChanges: nil,
			wantFound:      false,
		},
		{
			name:           "nil nodes skipped",
			unknownChanges: []*waBinary.Node{nil},
			wantFound:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRemove, gotPictureID, gotFound := extractGroupPictureChange(tt.unknownChanges)
			if gotFound != tt.wantFound {
				t.Fatalf("found = %v, want %v", gotFound, tt.wantFound)
			}
			if gotRemove != tt.wantRemove {
				t.Fatalf("remove = %v, want %v", gotRemove, tt.wantRemove)
			}
			if gotPictureID != tt.wantPictureID {
				t.Fatalf("picture_id = %q, want %q", gotPictureID, tt.wantPictureID)
			}
		})
	}
}

// groupPictureFixture builds a GroupInfo carrying a captured-style picture node.
func groupPictureFixture() *events.GroupInfo {
	groupJID := types.NewJID("12025550123", types.GroupServer)
	senderJID := types.NewJID("18095550999", types.DefaultUserServer)
	return &events.GroupInfo{
		JID:       groupJID,
		Sender:    &senderJID,
		Timestamp: time.Date(2026, 10, 12, 11, 25, 0, 0, time.UTC),
		UnknownChanges: []*waBinary.Node{
			{Tag: "picture", Attrs: waBinary.Attrs{"id": "1635239861", "type": "image"}},
		},
	}
}

func TestHandleGroupPictureChangeEmitsSameEnvelopeAsPictureEvent(t *testing.T) {
	if log == nil {
		log = waLog.Noop
	}

	originalWebhooks := config.WhatsappWebhook
	config.WhatsappWebhook = []string{"https://webhook.example.com/hook"}
	defer func() { config.WhatsappWebhook = originalWebhooks }()

	called := make(chan map[string]any, 1)
	originalSubmit := submitWebhookFn
	submitWebhookFn = func(_ context.Context, payload map[string]any, _ string, _ *chatstorage.DeviceWebhookConfig) error {
		called <- payload
		return nil
	}
	defer func() { submitWebhookFn = originalSubmit }()

	evt := groupPictureFixture()
	handleGroupPictureChange(context.Background(), evt, "", nil)

	select {
	case body := <-called:
		if body["event"] != eventTypePicture {
			t.Fatalf("expected event %q, got %#v", eventTypePicture, body["event"])
		}
		if body["timestamp"] != "2026-10-12T11:25:00Z" {
			t.Fatalf("unexpected envelope timestamp %#v", body["timestamp"])
		}

		groupPayload := body["payload"].(map[string]any)

		// Envelope equality: same builder as the real Picture path.
		want := buildPicturePayload(context.Background(), &events.Picture{
			JID:       evt.JID,
			Author:    *evt.Sender,
			Timestamp: evt.Timestamp,
			PictureID: "1635239861",
		}, nil)
		if !reflect.DeepEqual(groupPayload, want) {
			t.Fatalf("group picture payload diverges from Picture path\n got %#v\nwant %#v", groupPayload, want)
		}

		if groupPayload["jid"] != "12025550123@g.us" {
			t.Fatalf("expected group jid, got %#v", groupPayload["jid"])
		}
		if groupPayload["picture_id"] != "1635239861" {
			t.Fatalf("expected picture_id in payload, got %#v", groupPayload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("group picture event was not forwarded")
	}
}

func TestHandleGroupPictureChangeGuardAndIgnored(t *testing.T) {
	if log == nil {
		log = waLog.Noop
	}

	originalWebhooks := config.WhatsappWebhook
	config.WhatsappWebhook = nil
	defer func() { config.WhatsappWebhook = originalWebhooks }()

	called := make(chan map[string]any, 1)
	originalSubmit := submitWebhookFn
	submitWebhookFn = func(_ context.Context, payload map[string]any, _ string, _ *chatstorage.DeviceWebhookConfig) error {
		called <- payload
		return nil
	}
	defer func() { submitWebhookFn = originalSubmit }()

	t.Run("no webhooks and no device id does not forward", func(t *testing.T) {
		handleGroupPictureChange(context.Background(), groupPictureFixture(), "  ", nil)
		select {
		case body := <-called:
			t.Fatalf("unexpected forward with guard active: %#v", body)
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("unknown untyped changes forward fetch-on-event", func(t *testing.T) {
		config.WhatsappWebhook = []string{"https://webhook.example.com/hook"}
		evt := groupPictureFixture()
		evt.UnknownChanges = []*waBinary.Node{{Tag: "some_future_feature", Attrs: waBinary.Attrs{"x": "y"}}}
		handleGroupPictureChange(context.Background(), evt, "device_1", nil)
		select {
		case body := <-called:
			payload, _ := body["payload"].(map[string]any)
			if payload == nil {
				t.Fatalf("malformed forward payload: %#v", body)
			}
			if id, _ := payload["picture_id"].(string); id != "" {
				t.Fatalf("fetch-on-event emission must not carry picture_id, got %q", id)
			}
			if rm, _ := payload["remove"].(bool); rm {
				t.Fatal("fetch-on-event emission must not be remove")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("fetch-on-event emission never forwarded for untyped unknown change")
		}
	})

	t.Run("nil event does not panic", func(t *testing.T) {
		handleGroupPictureChange(context.Background(), nil, "device_1", nil)
	})

	t.Run("non-group jid ignored", func(t *testing.T) {
		evt := groupPictureFixture()
		evt.JID = types.NewJID("18095550123", types.DefaultUserServer)
		handleGroupPictureChange(context.Background(), evt, "device_1", nil)
		select {
		case body := <-called:
			t.Fatalf("unexpected forward for non-group jid: %#v", body)
		case <-time.After(200 * time.Millisecond):
		}
	})
}

func TestHandleGroupPictureChangeRemoveOmitsPictureID(t *testing.T) {
	if log == nil {
		log = waLog.Noop
	}

	originalWebhooks := config.WhatsappWebhook
	config.WhatsappWebhook = []string{"https://webhook.example.com/hook"}
	defer func() { config.WhatsappWebhook = originalWebhooks }()

	called := make(chan map[string]any, 1)
	originalSubmit := submitWebhookFn
	submitWebhookFn = func(_ context.Context, payload map[string]any, _ string, _ *chatstorage.DeviceWebhookConfig) error {
		called <- payload
		return nil
	}
	defer func() { submitWebhookFn = originalSubmit }()

	evt := groupPictureFixture()
	evt.UnknownChanges = []*waBinary.Node{{Tag: "delete"}}
	handleGroupPictureChange(context.Background(), evt, "", nil)

	select {
	case body := <-called:
		payload := body["payload"].(map[string]any)
		if payload["remove"] != true {
			t.Fatalf("expected remove=true, got %#v", payload)
		}
		if _, ok := payload["picture_id"]; ok {
			t.Fatalf("picture_id must be omitted on removal, got %#v", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("group picture removal event was not forwarded")
	}
}

func TestHandleGroupPictureChangeDoesNotFireOnKnownChanges(t *testing.T) {
	if log == nil {
		log = waLog.Noop
	}

	originalWebhooks := config.WhatsappWebhook
	config.WhatsappWebhook = []string{"https://webhook.example.com/hook"}
	defer func() { config.WhatsappWebhook = originalWebhooks }()

	called := make(chan map[string]any, 1)
	originalSubmit := submitWebhookFn
	submitWebhookFn = func(_ context.Context, payload map[string]any, _ string, _ *chatstorage.DeviceWebhookConfig) error {
		called <- payload
		return nil
	}
	defer func() { submitWebhookFn = originalSubmit }()

	// A subject rename with no unknown nodes must not produce a picture event.
	evt := groupPictureFixture()
	evt.UnknownChanges = nil
	handleGroupPictureChange(context.Background(), evt, "", nil)
	select {
	case body := <-called:
		t.Fatalf("unexpected picture forward for subject-only change: %#v", body)
	case <-time.After(200 * time.Millisecond):
	}
}
