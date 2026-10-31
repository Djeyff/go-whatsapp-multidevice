package whatsapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// RET-427-FU7b: every webhook forward must carry the session identity
// (X-Gowa-Device-Ref, same hash form as telemetry device_ref) so processors
// can fence on the SESSION, not just the instance id a stale clone also holds.

func TestWebhookDeviceRef(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{
			name:    "agent jid is canonicalized to nonAD",
			payload: map[string]any{"device_id": "18498837876:5@s.whatsapp.net"},
			want:    safeLogRef("device", "18498837876@s.whatsapp.net"),
		},
		{
			name:    "bare jid passes through",
			payload: map[string]any{"device_id": "18498837876@s.whatsapp.net"},
			want:    safeLogRef("device", "18498837876@s.whatsapp.net"),
		},
		{
			name:    "group chat jid keeps its server",
			payload: map[string]any{"device_id": "120363024128364064@g.us"},
			want:    safeLogRef("device", "120363024128364064@g.us"),
		},
		{
			name:    "missing device_id omits the ref",
			payload: map[string]any{"event_name": "message"},
			want:    "",
		},
		{
			name:    "non-string device_id omits the ref",
			payload: map[string]any{"device_id": 42},
			want:    "",
		},
		{
			name:    "whitespace device_id omits the ref",
			payload: map[string]any{"device_id": "   "},
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := webhookDeviceRef(tc.payload); got != tc.want {
				t.Fatalf("webhookDeviceRef(%v) = %q, want %q", tc.payload, got, tc.want)
			}
		})
	}
}

func TestSubmitWebhookStampsDeviceRefHeader(t *testing.T) {
	var gotDeviceRef, gotSignature string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDeviceRef = r.Header.Get("X-Gowa-Device-Ref")
		gotSignature = r.Header.Get("X-Hub-Signature-256")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	payload := map[string]any{
		"event_name": "message",
		"device_id":  "18498837876:5@s.whatsapp.net",
	}
	if err := submitWebhook(context.Background(), payload, ts.URL, nil); err != nil {
		t.Fatalf("submitWebhook failed: %v", err)
	}

	wantRef := safeLogRef("device", "18498837876@s.whatsapp.net")
	if gotDeviceRef != wantRef {
		t.Fatalf("X-Gowa-Device-Ref = %q, want %q", gotDeviceRef, wantRef)
	}
	if !strings.HasPrefix(gotSignature, "sha256=") || len(gotSignature) != len("sha256=")+64 {
		t.Fatalf("X-Hub-Signature-256 malformed: %q", gotSignature)
	}
}

func TestSubmitWebhookOmitsDeviceRefWithoutDeviceID(t *testing.T) {
	var sawDeviceRef bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawDeviceRef = r.Header.Get("X-Gowa-Device-Ref") != ""
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	payload := map[string]any{"event_name": "message"}
	if err := submitWebhook(context.Background(), payload, ts.URL, nil); err != nil {
		t.Fatalf("submitWebhook failed: %v", err)
	}
	if sawDeviceRef {
		t.Fatal("X-Gowa-Device-Ref must be omitted when the payload has no device_id")
	}
}
