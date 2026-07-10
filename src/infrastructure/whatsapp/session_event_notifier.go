package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/sirupsen/logrus"
)

func buildSessionEventPayload(instance *DeviceInstance, eventType string, severity string, reasonClass string, summary string) (string, string, []byte, bool) {
	webhookURL := strings.TrimSpace(config.RetenaSessionEventWebhook)
	if webhookURL == "" {
		return "", "", nil, false
	}

	deviceID := ""
	if instance != nil {
		deviceID = instance.ID()
	}
	payload := map[string]any{
		"event_type":                   eventType,
		"severity":                     severity,
		"source":                       "gowa_runtime",
		"gowa_device_id":               deviceID,
		"safe_summary":                 summary,
		"raw_reason_class":             reasonClass,
		"provider_mutations_performed": false,
		"auto_relink_allowed":          false,
		"metadata": map[string]any{
			"app_version": config.AppVersion,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		logrus.Warnf("[SESSION_EVENT] marshal failed event=%s device=%s error=%v", eventType, deviceID, err)
		return "", "", nil, false
	}

	return webhookURL, deviceID, body, true
}

func deliverSessionEvent(ctx context.Context, webhookURL string, deviceID string, eventType string, body []byte) {
	reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			var childCancel context.CancelFunc
			reqCtx, childCancel = context.WithDeadline(context.Background(), deadline)
			defer childCancel()
		}
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		logrus.Warnf("[SESSION_EVENT] request build failed event=%s device=%s error=%v", eventType, deviceID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := strings.TrimSpace(config.RetenaSessionEventWebhookSecret); secret != "" {
		req.Header.Set("X-Api-Secret", secret)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logrus.Warnf("[SESSION_EVENT] delivery failed event=%s device=%s error=%v", eventType, deviceID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logrus.Warnf("[SESSION_EVENT] delivery rejected event=%s device=%s status=%d", eventType, deviceID, resp.StatusCode)
	}
}

func notifySessionEvent(ctx context.Context, instance *DeviceInstance, eventType string, severity string, reasonClass string, summary string) {
	webhookURL, deviceID, body, ok := buildSessionEventPayload(instance, eventType, severity, reasonClass, summary)
	if !ok {
		return
	}
	go deliverSessionEvent(ctx, webhookURL, deviceID, eventType, body)
}

func NotifySessionEvent(ctx context.Context, instance *DeviceInstance, eventType string, severity string, reasonClass string, summary string) {
	notifySessionEvent(ctx, instance, eventType, severity, reasonClass, summary)
}

func NotifySessionEventSync(ctx context.Context, instance *DeviceInstance, eventType string, severity string, reasonClass string, summary string) {
	webhookURL, deviceID, body, ok := buildSessionEventPayload(instance, eventType, severity, reasonClass, summary)
	if !ok {
		return
	}
	deliverSessionEvent(ctx, webhookURL, deviceID, eventType, body)
}
