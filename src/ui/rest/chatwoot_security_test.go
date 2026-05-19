package rest

import (
	"testing"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
)

func TestChatwootAttachmentURLAllowed(t *testing.T) {
	prevURL := config.ChatwootURL
	t.Cleanup(func() {
		config.ChatwootURL = prevURL
	})

	config.ChatwootURL = "https://chatwoot.example.com"

	allowed := []string{
		"https://chatwoot.example.com/rails/active_storage/blobs/file.ogg",
		"http://chatwoot.example.com/storage/file.jpg",
		"https://CHATWOOT.example.com/path/file.pdf",
	}
	for _, raw := range allowed {
		if !chatwootAttachmentURLAllowed(raw) {
			t.Fatalf("expected allowed Chatwoot attachment URL: %s", raw)
		}
	}

	blocked := []string{
		"",
		"file:///tmp/file.jpg",
		"https://evil.example.com/storage/file.jpg",
		"http://127.0.0.1/internal",
		"http://10.0.0.2/internal",
		"http://169.254.169.254/latest/meta-data",
		"http://localhost:3000/internal",
		"http://service.local/internal",
	}
	for _, raw := range blocked {
		if chatwootAttachmentURLAllowed(raw) {
			t.Fatalf("expected blocked Chatwoot attachment URL: %s", raw)
		}
	}
}
