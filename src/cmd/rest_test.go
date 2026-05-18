package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/gofiber/fiber/v2"
)

func TestPassiveListenerGuardPresenceHeartbeatException(t *testing.T) {
	prevPassive := config.RetenaPassiveListenerMode
	prevHeartbeat := config.RetenaPassivePresenceHeartbeat
	prevAvailableHeartbeat := config.RetenaPassivePresenceAvailableHeartbeat
	prevBasePath := config.AppBasePath
	t.Cleanup(func() {
		config.RetenaPassiveListenerMode = prevPassive
		config.RetenaPassivePresenceHeartbeat = prevHeartbeat
		config.RetenaPassivePresenceAvailableHeartbeat = prevAvailableHeartbeat
		config.AppBasePath = prevBasePath
	})

	config.RetenaPassiveListenerMode = true
	config.RetenaPassivePresenceHeartbeat = true
	config.RetenaPassivePresenceAvailableHeartbeat = false
	config.AppBasePath = ""

	app := fiber.New()
	app.Post("/send/presence", passiveListenerGuard, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Post("/send/message", passiveListenerGuard, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/send/presence", strings.NewReader(`{"type":"unavailable"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unavailable presence request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("unavailable presence status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodPost, "/send/presence", strings.NewReader(`{"type":"available"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("available presence request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("available presence status = %d, want %d", resp.StatusCode, fiber.StatusForbidden)
	}

	config.RetenaPassivePresenceAvailableHeartbeat = true
	req = httptest.NewRequest(http.MethodPost, "/send/presence", strings.NewReader(`{"type":"available"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("available fallback presence request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("available fallback presence status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodPost, "/send/message", strings.NewReader(`{"message":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("message request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("message status = %d, want %d", resp.StatusCode, fiber.StatusForbidden)
	}
}
