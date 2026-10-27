package whatsapp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/types"
)

// webhookDeviceRef derives the X-Gowa-Device-Ref value for a webhook forward:
// safeLogRef("device", ToNonAD(jid)) — sha256("device\0"+jid+"\0") first 6
// bytes, hex — exactly the device_ref the fork logs in telemetry, so
// processors can correlate forwards with telemetry by string equality.
// Empty (header omitted) when the payload carries no device_id.
func webhookDeviceRef(payload map[string]any) string {
	deviceID, _ := payload["device_id"].(string)
	if strings.TrimSpace(deviceID) == "" {
		return ""
	}
	identity := deviceID
	if parsed, err := types.ParseJID(deviceID); err == nil {
		identity = parsed.ToNonAD().String()
	}
	return safeLogRef("device", identity)
}

func submitWebhook(ctx context.Context, payload map[string]any, url string, webhookConfig *chatstorage.DeviceWebhookConfig) error {
	// Determine effective config - use device-specific if set, otherwise fall back to global
	insecureSkipVerify := config.WhatsappWebhookInsecureSkipVerify
	webhookSecret := config.WhatsappWebhookSecret

	if webhookConfig != nil {
		if webhookConfig.WebhookInsecureSkipVerify {
			insecureSkipVerify = true
		}
		if webhookConfig.WebhookSecret != "" {
			webhookSecret = webhookConfig.WebhookSecret
		}
	}

	// Configure HTTP client with optional TLS skip verification
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	postBody, err := json.Marshal(payload)
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("Failed to marshal body: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("error when create http object %v", err))
	}

	secretKey := []byte(webhookSecret)
	signature, err := utils.GetMessageDigestOrSignature(postBody, secretKey)
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("error when create signature %v", err))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%s", signature))
	// Session identity stamp (RET-427-FU7b): processors fence on the SESSION
	// (device_ref), not only the instance id, so a stale clone holding the
	// same webhook secret + instance id can no longer impersonate this
	// deployment's sessions. Same hash form as telemetry device_ref.
	if ref := webhookDeviceRef(payload); ref != "" {
		req.Header.Set("X-Gowa-Device-Ref", ref)
	}

	var attempt int
	var maxAttempts = 5
	var sleepDuration = 1 * time.Second

	for attempt = 0; attempt < maxAttempts; attempt++ {
		// Create new request body for each attempt
		req.Body = io.NopCloser(bytes.NewBuffer(postBody))
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				logrus.Infof("Successfully submitted webhook on attempt %d", attempt+1)
				return nil
			}
			err = fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}
		logrus.Warnf("Attempt %d to submit webhook failed: %v", attempt+1, err)
		if attempt < maxAttempts-1 {
			time.Sleep(sleepDuration)
			sleepDuration *= 2
		}
	}

	return pkgError.WebhookError(fmt.Sprintf("error when submit webhook after %d attempts: %v", attempt, err))
}
