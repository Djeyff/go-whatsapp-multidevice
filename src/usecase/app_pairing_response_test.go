package usecase

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
)

func TestLoginResponseDurationIsJSONSeconds(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	response := loginResponseFromPairingSnapshot(whatsapp.PairingSessionSnapshot{
		SessionID:  "pairing-1",
		Generation: 2,
		EmittedAt:  now.Add(-10 * time.Second),
		ValidUntil: now.Add(50 * time.Second),
		State:      whatsapp.PairingSessionQRReady,
	}, now)

	if response.Duration != 50 {
		t.Fatalf("Duration = %d, want 50 seconds", response.Duration)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got := payload["duration"]; got != float64(50) {
		t.Fatalf("JSON duration = %#v, want 50", got)
	}
}
