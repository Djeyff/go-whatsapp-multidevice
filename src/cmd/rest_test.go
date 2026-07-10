package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/gofiber/fiber/v2"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

func TestPassiveSafetyStatusRedactsAndClassifies(t *testing.T) {
	previousOS := config.AppOs
	previousPlatform := config.AppPlatform
	previousPassive := config.RetenaPassiveListenerMode
	t.Cleanup(func() {
		config.AppOs = previousOS
		config.AppPlatform = previousPlatform
		config.RetenaPassiveListenerMode = previousPassive
	})
	config.AppOs = "Retena"
	config.AppPlatform = waCompanionReg.DeviceProps_DESKTOP
	config.RetenaPassiveListenerMode = true

	t.Setenv("COMMIT_SHA", "candidate-sha")
	payload := passiveSafetyStatus()
	for key, want := range map[string]any{
		"app_os":       "Retena",
		"app_platform": "DESKTOP",
		"commit":       "candidate-sha",
		"passive_mode": true,
	} {
		if got := payload[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
	if payload["session_event_webhook_secret_value_exposed"] != false {
		t.Fatal("passive safety status must never expose an internal webhook secret value")
	}
	for _, key := range []string{
		"presence_heartbeat",
		"presence_available_heartbeat",
		"auto_reply_enabled",
		"auto_mark_read",
		"auto_download_media",
		"auto_reject_call",
		"presence_on_connect",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("passive safety payload missing %s", key)
		}
	}
}

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

func TestBasicAuthConfigFailsClosedInProduction(t *testing.T) {
	prevAppEnv, hadAppEnv := os.LookupEnv("APP_ENV")
	prevNodeEnv, hadNodeEnv := os.LookupEnv("NODE_ENV")
	t.Cleanup(func() {
		if hadAppEnv {
			os.Setenv("APP_ENV", prevAppEnv)
		} else {
			os.Unsetenv("APP_ENV")
		}
		if hadNodeEnv {
			os.Setenv("NODE_ENV", prevNodeEnv)
		} else {
			os.Unsetenv("NODE_ENV")
		}
	})

	os.Setenv("APP_ENV", "production")
	os.Unsetenv("NODE_ENV")
	if _, err := basicAuthAccounts(nil); err == nil {
		t.Fatal("production without APP_BASIC_AUTH should fail closed")
	}

	os.Setenv("APP_ENV", "development")
	if accounts, err := basicAuthAccounts(nil); err != nil || len(accounts) != 0 {
		t.Fatalf("development without APP_BASIC_AUTH should stay local-only, accounts=%v err=%v", accounts, err)
	}

	os.Setenv("APP_ENV", "production")
	if _, err := basicAuthAccounts([]string{"broken"}); err == nil {
		t.Fatal("invalid basic auth credential should fail")
	}

	accounts, err := basicAuthAccounts([]string{"admin:secret"})
	if err != nil {
		t.Fatalf("valid production basic auth returned error: %v", err)
	}
	if accounts["admin"] != "secret" {
		t.Fatalf("valid production basic auth account missing: %#v", accounts)
	}
}

func TestChatwootWebhookGateRequiresDedicatedSecret(t *testing.T) {
	prevSecret := config.ChatwootWebhookSecret
	t.Cleanup(func() {
		config.ChatwootWebhookSecret = prevSecret
	})

	app := fiber.New()
	app.Post("/chatwoot/webhook", chatwootWebhookGate(func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}))

	config.ChatwootWebhookSecret = ""
	req := httptest.NewRequest(http.MethodPost, "/chatwoot/webhook", strings.NewReader(`{}`))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("missing secret request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("missing secret status = %d, want %d", resp.StatusCode, fiber.StatusServiceUnavailable)
	}

	config.ChatwootWebhookSecret = "top-secret"
	req = httptest.NewRequest(http.MethodPost, "/chatwoot/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-Chatwoot-Webhook-Secret", "wrong")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("wrong secret request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("wrong secret status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodPost, "/chatwoot/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-Chatwoot-Webhook-Secret", "top-secret")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("header secret request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("header secret status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodPost, "/chatwoot/webhook", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer top-secret")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("bearer secret request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("bearer secret status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodPost, "/chatwoot/webhook?token=top-secret", strings.NewReader(`{}`))
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("query secret request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("query secret status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}
