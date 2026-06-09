package whatsapp

import (
	"testing"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestApplyMessageContextMetadataIncludesQuotedReply(t *testing.T) {
	message := &domainChatStorage.Message{}
	protoMessage := &waE2E.Message{
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
	}

	applyMessageContextMetadata(message, protoMessage)

	if message.RepliedToID != "ORIGINAL123" {
		t.Fatalf("expected replied_to_id ORIGINAL123, got %q", message.RepliedToID)
	}
	if message.QuotedBody != "Ahora con el depósito, hay para el seguro." {
		t.Fatalf("expected quoted body to be persisted, got %q", message.QuotedBody)
	}
	if message.QuotedSender != "123@s.whatsapp.net" {
		t.Fatalf("expected quoted sender to be persisted, got %q", message.QuotedSender)
	}
}
