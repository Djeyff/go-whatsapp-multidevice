package whatsapp

import (
	"context"
	"testing"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
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
