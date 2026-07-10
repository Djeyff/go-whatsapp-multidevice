package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/gofiber/fiber/v2"
)

type pairingAppStub struct {
	domainApp.IAppUsecase
	response domainApp.LoginResponse
}

func (s *pairingAppStub) Login(context.Context, string) (domainApp.LoginResponse, error) {
	return s.response, nil
}

func TestLoginReturnsGenerationAwarePairingSnapshot(t *testing.T) {
	emittedAt := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	validUntil := emittedAt.Add(60 * time.Second)
	stub := &pairingAppStub{response: domainApp.LoginResponse{
		ImagePath:        "statics/qrcode/example.png",
		Duration:         60,
		PairingSessionID: "pairing-session-1",
		QRGeneration:     2,
		EmittedAt:        emittedAt,
		ValidUntil:       validUntil,
		State:            "qr_ready",
	}}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("device", whatsapp.NewDeviceInstance("device-1", nil, nil))
		return c.Next()
	})
	controller := App{Service: stub}
	app.Get("/app/login", controller.Login)

	request := httptest.NewRequest(http.MethodGet, "/app/login", nil)
	request.Host = "example.com"
	request.Header.Set("Host", "example.com")
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	var payload struct {
		Results map[string]any `json:"results"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for key, want := range map[string]any{
		"pairing_session_id": "pairing-session-1",
		"qr_generation":      float64(2),
		"state":              "qr_ready",
	} {
		if got := payload.Results[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
	if payload.Results["emitted_at"] != emittedAt.Format(time.RFC3339) {
		t.Fatalf("emitted_at = %#v", payload.Results["emitted_at"])
	}
	if payload.Results["valid_until"] != validUntil.Format(time.RFC3339) {
		t.Fatalf("valid_until = %#v", payload.Results["valid_until"])
	}
}
