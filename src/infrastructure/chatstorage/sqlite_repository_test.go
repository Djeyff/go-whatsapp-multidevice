package chatstorage

import (
	"database/sql"
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	_ "github.com/mattn/go-sqlite3"
)

func newForwardedMetadataTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := &SQLiteRepository{db: db}
	if err := repo.InitializeSchema(); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo
}

func TestStoreMessagePersistsForwardedMetadata(t *testing.T) {
	repo := newForwardedMetadataTestRepo(t)

	err := repo.StoreMessage(&domainChatStorage.Message{
		ID:              "forwarded-message",
		ChatJID:         "120363407735925224@g.us",
		DeviceID:        "18090000000@s.whatsapp.net",
		Sender:          "18095550000@s.whatsapp.net",
		Content:         "voice",
		Timestamp:       time.Unix(1780000000, 0),
		IsForwarded:     true,
		ForwardingScore: 7,
		MediaType:       "audio",
	})
	if err != nil {
		t.Fatalf("store message: %v", err)
	}

	messages, err := repo.GetMessages(&domainChatStorage.MessageFilter{
		DeviceID: "18090000000@s.whatsapp.net",
		ChatJID:  "120363407735925224@g.us",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if !messages[0].IsForwarded {
		t.Fatal("expected is_forwarded to round trip")
	}
	if messages[0].ForwardingScore != 7 {
		t.Fatalf("expected forwarding score 7, got %d", messages[0].ForwardingScore)
	}
}

func TestStoreMessagesBatchUpdatesForwardedMetadata(t *testing.T) {
	repo := newForwardedMetadataTestRepo(t)

	base := &domainChatStorage.Message{
		ID:        "same-message",
		ChatJID:   "120363407735925224@g.us",
		DeviceID:  "18090000000@s.whatsapp.net",
		Sender:    "18095550000@s.whatsapp.net",
		Content:   "voice",
		Timestamp: time.Unix(1780000000, 0),
		MediaType: "audio",
	}
	if err := repo.StoreMessage(base); err != nil {
		t.Fatalf("store base message: %v", err)
	}

	base.IsForwarded = true
	base.ForwardingScore = 12
	if err := repo.StoreMessagesBatch([]*domainChatStorage.Message{base}); err != nil {
		t.Fatalf("batch update message: %v", err)
	}

	message, err := repo.GetMessageByID("same-message")
	if err != nil {
		t.Fatalf("get message by id: %v", err)
	}
	if message == nil {
		t.Fatal("expected message")
	}
	if !message.IsForwarded {
		t.Fatal("expected batch update to persist is_forwarded")
	}
	if message.ForwardingScore != 12 {
		t.Fatalf("expected forwarding score 12, got %d", message.ForwardingScore)
	}
}
