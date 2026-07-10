package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
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

func TestPassiveSafetyStatusRedactsAndClassifies(t *testing.T) {
	prevPassive := config.RetenaPassiveListenerMode
	prevReply := config.WhatsappAutoReplyMessage
	prevMarkRead := config.WhatsappAutoMarkRead
	prevDownload := config.WhatsappAutoDownloadMedia
	prevRejectCall := config.WhatsappAutoRejectCall
	prevPresence := config.WhatsappPresenceOnConnect
	prevWebhook := config.RetenaSessionEventWebhook
	prevWebhookSecret := config.RetenaSessionEventWebhookSecret
	t.Cleanup(func() {
		config.RetenaPassiveListenerMode = prevPassive
		config.WhatsappAutoReplyMessage = prevReply
		config.WhatsappAutoMarkRead = prevMarkRead
		config.WhatsappAutoDownloadMedia = prevDownload
		config.WhatsappAutoRejectCall = prevRejectCall
		config.WhatsappPresenceOnConnect = prevPresence
		config.RetenaSessionEventWebhook = prevWebhook
		config.RetenaSessionEventWebhookSecret = prevWebhookSecret
	})

	config.RetenaPassiveListenerMode = true
	config.WhatsappAutoReplyMessage = ""
	config.WhatsappAutoMarkRead = false
	config.WhatsappAutoDownloadMedia = false
	config.WhatsappAutoRejectCall = false
	config.WhatsappPresenceOnConnect = "none"
	config.RetenaSessionEventWebhook = "https://api.example/internal/gowa/session-event"
	config.RetenaSessionEventWebhookSecret = "super-secret"

	status := passiveSafetyStatus()
	if status["ok"] != true {
		t.Fatalf("passive safety ok = %v, want true", status["ok"])
	}
	if status["session_event_webhook_secret_value_exposed"] != false {
		t.Fatalf("passive safety must not expose webhook secret values")
	}

	config.WhatsappAutoMarkRead = true
	status = passiveSafetyStatus()
	if status["ok"] != false {
		t.Fatalf("passive safety ok = %v, want false when auto mark read is enabled", status["ok"])
	}
	findings, ok := status["unsafe_findings"].([]string)
	if !ok || len(findings) == 0 || findings[0] != "whatsapp_auto_mark_read_enabled" {
		t.Fatalf("unexpected unsafe findings: %#v", status["unsafe_findings"])
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
