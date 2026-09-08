package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/stretchr/testify/assert"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestBuildEventPayloadIncludesIsFromMe(t *testing.T) {
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("123", types.DefaultUserServer),
				IsFromMe: true,
			},
			ID:        "MSG123",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			Conversation: protoString("hello"),
		},
	}

	eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eventType != EventTypeMessage {
		t.Fatalf("expected event type %s, got %s", EventTypeMessage, eventType)
	}
	if value, ok := payload["is_from_me"]; !ok {
		t.Fatalf("expected is_from_me in payload")
	} else if isFromMe, ok := value.(bool); !ok || !isFromMe {
		t.Fatalf("expected is_from_me=true, got %v", value)
	}
}

func TestConfiguredWebhookInstanceIdentityRequiresMatchingAllowedPair(t *testing.T) {
	tests := []struct {
		name    string
		primary string
		compat  string
		want    string
	}{
		{name: "matching normalized allowed pair", primary: " GOWA-BLUE ", compat: "gowa-blue", want: "gowa-blue"},
		{name: "primary only", primary: "gowa-blue", compat: "", want: ""},
		{name: "compatibility only", primary: "", compat: "gowa-blue", want: ""},
		{name: "conflicting pair", primary: "gowa-blue", compat: "gowa-main", want: ""},
		{name: "invalid pair", primary: "gowa-other", compat: "gowa-other", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOWA_INSTANCE_ID", tt.primary)
			t.Setenv("RETENA_GOWA_INSTANCE_ID", tt.compat)
			if got := configuredWebhookInstanceIdentity(); got != tt.want {
				t.Fatalf("configured webhook instance identity = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateWebhookEventIncludesConfiguredInstanceIdentity(t *testing.T) {
	t.Setenv("GOWA_INSTANCE_ID", "gowa-blue")
	t.Setenv("RETENA_GOWA_INSTANCE_ID", "gowa-blue")
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   types.NewJID("123", types.DefaultUserServer),
				Sender: types.NewJID("123", types.DefaultUserServer),
			},
			ID:        "INSTANCE-ID-TEST",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{Conversation: protoString("hello")},
	}

	webhookEvent, err := createWebhookEvent(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("create webhook event: %v", err)
	}
	encoded, err := json.Marshal(webhookEvent)
	if err != nil {
		t.Fatalf("marshal webhook event: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatalf("unmarshal webhook event: %v", err)
	}
	if got := body["instance_id"]; got != "gowa-blue" {
		t.Fatalf("instance_id = %#v, want gowa-blue", got)
	}
}

func TestForwardMessageToWebhookSignsConfiguredInstanceIdentity(t *testing.T) {
	originalWebhookURLs := config.WhatsappWebhook
	originalWebhookEvents := config.WhatsappWebhookEvents
	originalWebhookSecret := config.WhatsappWebhookSecret
	originalChatwootEnabled := config.ChatwootEnabled
	originalSubmit := submitWebhookFn
	defer func() {
		config.WhatsappWebhook = originalWebhookURLs
		config.WhatsappWebhookEvents = originalWebhookEvents
		config.WhatsappWebhookSecret = originalWebhookSecret
		config.ChatwootEnabled = originalChatwootEnabled
		submitWebhookFn = originalSubmit
	}()

	t.Setenv("GOWA_INSTANCE_ID", "gowa-blue")
	t.Setenv("RETENA_GOWA_INSTANCE_ID", "gowa-blue")
	config.WhatsappWebhookEvents = nil
	config.ChatwootEnabled = false
	config.WhatsappWebhookSecret = "instance-identity-test-secret"
	submitWebhookFn = submitWebhook

	type receivedRequest struct {
		body      []byte
		signature string
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read forwarded request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		received <- receivedRequest{body: body, signature: r.Header.Get("X-Hub-Signature-256")}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	config.WhatsappWebhook = []string{server.URL}

	if err := forwardMessageToWebhook(context.Background(), nil, textEventForTest("signed-instance", types.NewJID("123", types.DefaultUserServer))); err != nil {
		t.Fatalf("forward message to webhook: %v", err)
	}

	select {
	case got := <-received:
		var body map[string]any
		if err := json.Unmarshal(got.body, &body); err != nil {
			t.Fatalf("decode signed webhook body: %v", err)
		}
		if body["instance_id"] != "gowa-blue" {
			t.Fatalf("forwarded instance_id = %#v, want gowa-blue", body["instance_id"])
		}
		mac := hmac.New(sha256.New, []byte(config.WhatsappWebhookSecret))
		_, _ = mac.Write(got.body)
		wantSignature := "sha256=" + fmt.Sprintf("%x", mac.Sum(nil))
		if !hmac.Equal([]byte(got.signature), []byte(wantSignature)) {
			t.Fatalf("signature = %q, want %q", got.signature, wantSignature)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signed webhook request")
	}
}

func TestBuildEventPayloadRevokedIncludesIsFromMe(t *testing.T) {
	key := &waCommon.MessageKey{
		RemoteJID: protoString("123@s.whatsapp.net"),
		FromMe:    protoBool(true),
		ID:        protoString("REV123"),
	}
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("123", types.DefaultUserServer),
				IsFromMe: true,
			},
			ID:        "MSG124",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type: protoProtocolMessageType(waE2E.ProtocolMessage_REVOKE),
				Key:  key,
			},
		},
	}

	eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eventType != EventTypeMessageRevoked {
		t.Fatalf("expected event type %s, got %s", EventTypeMessageRevoked, eventType)
	}
	if value, ok := payload["is_from_me"]; !ok {
		t.Fatalf("expected is_from_me in payload")
	} else if isFromMe, ok := value.(bool); !ok || !isFromMe {
		t.Fatalf("expected is_from_me=true, got %v", value)
	}
}

func TestBuildEventPayloadReactionIncludesTargetMessageID(t *testing.T) {
	evt := reactionEventForTest("reaction-event-1", "MSG100", "\U0001f44d")

	eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eventType != EventTypeMessageReaction {
		t.Fatalf("expected event type %s, got %s", EventTypeMessageReaction, eventType)
	}
	if got := payload["reaction"]; got != "\U0001f44d" {
		t.Fatalf("expected reaction payload, got %v", got)
	}
	if got := payload["reacted_message_id"]; got != "MSG100" {
		t.Fatalf("expected reacted message id MSG100, got %v", got)
	}
}

func TestBuildEventPayloadReactionWithoutKeyDoesNotPanic(t *testing.T) {
	evt := reactionEventForTest("reaction-event-2", "MSG101", "\U0001f44d")
	evt.Message.ReactionMessage.Key = nil

	eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eventType != EventTypeMessageReaction {
		t.Fatalf("expected event type %s, got %s", EventTypeMessageReaction, eventType)
	}
	if _, ok := payload["reacted_message_id"]; ok {
		t.Fatalf("expected no reacted_message_id when reaction key is missing")
	}
}

func protoString(value string) *string {
	return &value
}

func protoBool(value bool) *bool {
	return &value
}

func protoUint32(value uint32) *uint32 {
	return &value
}

func protoProtocolMessageType(value waE2E.ProtocolMessage_Type) *waE2E.ProtocolMessage_Type {
	return &value
}

func TestBuildEventPayloadIncludesForwardedContext(t *testing.T) {
	config.WhatsappAutoDownloadMedia = false
	forwardedContext := &waE2E.ContextInfo{
		IsForwarded:     protoBool(true),
		ForwardingScore: protoUint32(7),
	}

	tests := []struct {
		name    string
		message *waE2E.Message
	}{
		{
			name: "extended text",
			message: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text:        protoString("forwarded text"),
					ContextInfo: forwardedContext,
				},
			},
		},
		{
			name: "audio voice",
			message: &waE2E.Message{
				AudioMessage: &waE2E.AudioMessage{
					ContextInfo: forwardedContext,
					PTT:         protoBool(true),
				},
			},
		},
		{
			name: "edited extended text",
			message: &waE2E.Message{
				ProtocolMessage: &waE2E.ProtocolMessage{
					Type: protoProtocolMessageType(waE2E.ProtocolMessage_MESSAGE_EDIT),
					EditedMessage: &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text:        protoString("edited forwarded text"),
							ContextInfo: forwardedContext,
						},
					},
				},
			},
		},
		{
			name: "device sent extended text",
			message: &waE2E.Message{
				DeviceSentMessage: &waE2E.DeviceSentMessage{
					Message: &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text:        protoString("forwarded from companion"),
							ContextInfo: forwardedContext,
						},
					},
				},
			},
		},
		{
			name: "bot forwarded future proof text",
			message: &waE2E.Message{
				BotForwardedMessage: &waE2E.FutureProofMessage{
					Message: &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text:        protoString("forwarded future proof text"),
							ContextInfo: forwardedContext,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := &events.Message{
				Info: types.MessageInfo{
					MessageSource: types.MessageSource{
						Chat:     types.NewJID("123", types.DefaultUserServer),
						Sender:   types.NewJID("456", types.DefaultUserServer),
						IsFromMe: false,
					},
					ID:        "FWD123",
					Timestamp: time.Date(2026, time.June, 9, 10, 0, 0, 0, time.UTC),
				},
				Message: tt.message,
			}

			_, payload, err := buildEventPayload(context.Background(), nil, evt)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if forwarded, ok := payload["forwarded"].(bool); !ok || !forwarded {
				t.Fatalf("expected forwarded=true, got %v", payload["forwarded"])
			}
			if forwarded, ok := payload["is_forwarded"].(bool); !ok || !forwarded {
				t.Fatalf("expected is_forwarded=true, got %v", payload["is_forwarded"])
			}
			if score, ok := payload["forwarding_score"].(uint32); !ok || score != 7 {
				t.Fatalf("expected forwarding_score=7, got %#v", payload["forwarding_score"])
			}
		})
	}
}

func TestBuildEventPayloadIncludesQuotedReplyContext(t *testing.T) {
	config.WhatsappAutoDownloadMedia = false
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("456", types.DefaultUserServer),
				IsFromMe: false,
			},
			ID:        "REPLY123",
			Timestamp: time.Date(2026, time.June, 9, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: protoString("Ok me avisa"),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:    protoString("ORIGINAL123"),
					Participant: protoString("123@s.whatsapp.net"),
					QuotedMessage: &waE2E.Message{
						Conversation: protoString("Ahora con el depósito, hay para el seguro."),
					},
				},
			},
		},
	}

	eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eventType != EventTypeMessage {
		t.Fatalf("expected event type %s, got %s", EventTypeMessage, eventType)
	}
	if got := payload["replied_to_id"]; got != "ORIGINAL123" {
		t.Fatalf("expected replied_to_id ORIGINAL123, got %#v", got)
	}
	if got := payload["quoted_body"]; got != "Ahora con el depósito, hay para el seguro." {
		t.Fatalf("expected quoted_body to round trip, got %#v", got)
	}
	if got := payload["quoted_sender"]; got != "123@s.whatsapp.net" {
		t.Fatalf("expected quoted_sender to round trip, got %#v", got)
	}
}

func TestBuildEventPayloadImageWithCaption(t *testing.T) {
	config.WhatsappAutoDownloadMedia = false
	caption := "Check this out!"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("456", types.DefaultUserServer),
				IsFromMe: false,
			},
			ID:        "MSG200",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				Caption: &caption,
			},
		},
	}

	eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eventType != EventTypeMessage {
		t.Fatalf("expected event type %s, got %s", EventTypeMessage, eventType)
	}
	body, ok := payload["body"]
	if !ok {
		t.Fatal("expected body in payload for image with caption")
	}
	if body != "Check this out!" {
		t.Fatalf("expected body='Check this out!', got %v", body)
	}
}

func TestBuildEventPayloadVideoWithCaption(t *testing.T) {
	config.WhatsappAutoDownloadMedia = false
	caption := "Watch this video"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("456", types.DefaultUserServer),
				IsFromMe: false,
			},
			ID:        "MSG201",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				Caption: &caption,
			},
		},
	}

	eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eventType != EventTypeMessage {
		t.Fatalf("expected event type %s, got %s", EventTypeMessage, eventType)
	}
	body, ok := payload["body"]
	if !ok {
		t.Fatal("expected body in payload for video with caption")
	}
	if body != "Watch this video" {
		t.Fatalf("expected body='Watch this video', got %v", body)
	}
}

func TestBuildEventPayloadImageWithoutCaption(t *testing.T) {
	config.WhatsappAutoDownloadMedia = false
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("456", types.DefaultUserServer),
				IsFromMe: false,
			},
			ID:        "MSG202",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{},
		},
	}

	_, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := payload["body"]; ok {
		t.Fatal("expected no body in payload for image without caption")
	}
}

func TestBuildEventPayloadDocumentWithCaption(t *testing.T) {
	config.WhatsappAutoDownloadMedia = false
	caption := "Important document"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("456", types.DefaultUserServer),
				IsFromMe: false,
			},
			ID:        "MSG203",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{
				Caption: &caption,
			},
		},
	}

	eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if eventType != EventTypeMessage {
		t.Fatalf("expected event type %s, got %s", EventTypeMessage, eventType)
	}
	body, ok := payload["body"]
	if !ok {
		t.Fatal("expected body in payload for document with caption")
	}
	if body != "Important document" {
		t.Fatalf("expected body='Important document', got %v", body)
	}
}

func TestBuildEventPayloadQuotedBodyUsesQuotedCaption(t *testing.T) {
	oldAutoDownload := config.WhatsappAutoDownloadMedia
	config.WhatsappAutoDownloadMedia = false
	t.Cleanup(func() {
		config.WhatsappAutoDownloadMedia = oldAutoDownload
	})

	tests := []struct {
		name           string
		quotedMessage  func(*string) *waE2E.Message
		wantQuotedBody string
	}{
		{
			name: "uses quoted image caption",
			quotedMessage: func(caption *string) *waE2E.Message {
				return &waE2E.Message{
					ImageMessage: &waE2E.ImageMessage{
						Caption: caption,
					},
				}
			},
			wantQuotedBody: "Launch checklist",
		},
		{
			name: "uses quoted document caption",
			quotedMessage: func(caption *string) *waE2E.Message {
				return &waE2E.Message{
					DocumentMessage: &waE2E.DocumentMessage{
						Caption: caption,
					},
				}
			},
			wantQuotedBody: "Project brief",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replyText := "Thanks for the update"
			quotedCaption := tt.wantQuotedBody
			evt := &events.Message{
				Info: types.MessageInfo{
					MessageSource: types.MessageSource{
						Chat:     types.NewJID("123", types.DefaultUserServer),
						Sender:   types.NewJID("456", types.DefaultUserServer),
						IsFromMe: false,
					},
					ID:        "MSG206",
					Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
				},
				Message: &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &replyText,
						ContextInfo: &waE2E.ContextInfo{
							StanzaID:      protoString("QUOTE206"),
							QuotedMessage: tt.quotedMessage(&quotedCaption),
						},
					},
				},
			}

			eventType, payload, err := buildEventPayload(context.Background(), nil, evt)
			assert.NoError(t, err)
			assert.Equal(t, EventTypeMessage, eventType)
			assert.Contains(t, payload, "quoted_body")
			assert.Equal(t, tt.wantQuotedBody, payload["quoted_body"])
		})
	}
}

func TestBuildEventPayloadContactIncludesPhoneNumber(t *testing.T) {
	name := "Alice"
	vcard := "BEGIN:VCARD\nVERSION:3.0\nN:;Alice;;;\nFN:Alice\nTEL;type=Mobile:+62 812 3456 7890\nEND:VCARD"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("456", types.DefaultUserServer),
				IsFromMe: false,
			},
			ID:        "MSG204",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			ContactMessage: &waE2E.ContactMessage{
				DisplayName: &name,
				Vcard:       &vcard,
			},
		},
	}

	_, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	contact, ok := payload["contact"].(webhookContactPayload)
	if !ok {
		t.Fatalf("expected contact payload to be webhookContactPayload, got %T", payload["contact"])
	}
	if contact.DisplayName != "Alice" {
		t.Fatalf("expected display name Alice, got %q", contact.DisplayName)
	}
	if contact.PhoneNumber != "+62 812 3456 7890" {
		t.Fatalf("expected phone number from vCard, got %q", contact.PhoneNumber)
	}
}

func TestBuildEventPayloadContactsArrayIncludesPhoneNumbers(t *testing.T) {
	nameOne := "Alice"
	vcardOne := "BEGIN:VCARD\nVERSION:3.0\nN:;Alice;;;\nFN:Alice\nTEL;type=Mobile:+62 812 3456 7890\nEND:VCARD"
	nameTwo := "Bob"
	vcardTwo := "BEGIN:VCARD\nVERSION:3.0\nN:;Bob;;;\nFN:Bob\nTEL;type=Mobile:+62 813 9876 5432\nEND:VCARD"
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("123", types.DefaultUserServer),
				Sender:   types.NewJID("456", types.DefaultUserServer),
				IsFromMe: false,
			},
			ID:        "MSG205",
			Timestamp: time.Date(2026, time.February, 8, 10, 0, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{
			ContactsArrayMessage: &waE2E.ContactsArrayMessage{
				Contacts: []*waE2E.ContactMessage{
					{
						DisplayName: &nameOne,
						Vcard:       &vcardOne,
					},
					{
						DisplayName: &nameTwo,
						Vcard:       &vcardTwo,
					},
				},
			},
		},
	}

	_, payload, err := buildEventPayload(context.Background(), nil, evt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	contacts, ok := payload["contacts_array"].([]webhookContactPayload)
	if !ok {
		t.Fatalf("expected contacts_array to be []webhookContactPayload, got %T", payload["contacts_array"])
	}
	if len(contacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(contacts))
	}
	if contacts[0].PhoneNumber != "+62 812 3456 7890" {
		t.Fatalf("expected first phone number, got %q", contacts[0].PhoneNumber)
	}
	if contacts[1].PhoneNumber != "+62 813 9876 5432" {
		t.Fatalf("expected second phone number, got %q", contacts[1].PhoneNumber)
	}
}
