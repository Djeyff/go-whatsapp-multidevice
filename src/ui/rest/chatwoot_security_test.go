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

func TestRedactChatwootIdentifier(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"123":                           "***",
		"+18092044903":                  "***4903",
		"584160334076@c.us":             "***4076",
		"120363123456789012@g.us":       "***9012",
		"  8295551212@s.whatsapp.net  ": "***1212",
	}
	for input, expected := range cases {
		if got := redactChatwootIdentifier(input); got != expected {
			t.Fatalf("redactChatwootIdentifier(%q) = %q, want %q", input, got, expected)
		}
	}
}
