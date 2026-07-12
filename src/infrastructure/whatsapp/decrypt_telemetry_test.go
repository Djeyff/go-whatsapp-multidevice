package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestDecryptTelemetryFieldsAreSafeAndCorrelatable(t *testing.T) {
	instance := NewDeviceInstance("18095550111@s.whatsapp.net", nil, nil)
	info := sensitiveDecryptMessageInfo()

	failureFields := decryptTelemetryFields(instance, info)
	recoveryFields := decryptTelemetryFields(instance, info)

	if failureFields["event"] != "signal_decrypt" {
		t.Fatalf("event = %v, want signal_decrypt", failureFields["event"])
	}
	if failureFields["message_ref"] != recoveryFields["message_ref"] {
		t.Fatal("failure and recovery must use the same message reference")
	}
	if failureFields["device_ref"] != recoveryFields["device_ref"] {
		t.Fatal("failure and recovery must use the same device reference")
	}
	if failureFields["message_ref"] == failureFields["device_ref"] {
		t.Fatal("message and device references must be namespace separated")
	}

	allowed := map[string]bool{
		"event": true, "message_ref": true, "device_ref": true,
		"from_me": true, "is_group": true,
	}
	for key := range failureFields {
		if !allowed[key] {
			t.Fatalf("unexpected decrypt telemetry field %q", key)
		}
	}
	assertNoDecryptTelemetrySentinel(t, mustMarshalDecryptTelemetry(t, failureFields))
}

func TestLogUndecryptableMessageEmitsPrivacySafeWarning(t *testing.T) {
	output := captureDecryptTelemetryLogs(t, logrus.WarnLevel)
	instance := NewDeviceInstance("18095550111@s.whatsapp.net", nil, nil)
	evt := &events.UndecryptableMessage{
		Info:            sensitiveDecryptMessageInfo(),
		IsUnavailable:   true,
		UnavailableType: events.UnavailableTypeViewOnce,
		DecryptFailMode: events.DecryptFailHide,
	}

	logUndecryptableMessage(instance, evt)

	entry := decodeSingleDecryptTelemetryEntry(t, output)
	if entry["level"] != "warning" || entry["event"] != "signal_decrypt" || entry["outcome"] != "failed" {
		t.Fatalf("unexpected failure telemetry: %#v", entry)
	}
	if entry["is_unavailable"] != true || entry["unavailable_type"] != "view_once" || entry["decrypt_fail_mode"] != "hide" {
		t.Fatalf("unexpected safe failure fields: %#v", entry)
	}
	if _, ok := entry["retry_count"]; ok {
		t.Fatal("failure telemetry must not claim a retry count")
	}
	assertNoDecryptTelemetrySentinel(t, mustMarshalDecryptTelemetry(t, entry))
}

func TestLogUndecryptableMessageSanitizesUnknownEnums(t *testing.T) {
	output := captureDecryptTelemetryLogs(t, logrus.WarnLevel)
	logUndecryptableMessage(NewDeviceInstance("18095550111@s.whatsapp.net", nil, nil), &events.UndecryptableMessage{
		Info:            sensitiveDecryptMessageInfo(),
		UnavailableType: events.UnavailableType("UNAVAILABLE-TYPE-SENTINEL"),
		DecryptFailMode: events.DecryptFailMode("DECRYPT-FAIL-MODE-SENTINEL"),
	})

	entry := decodeSingleDecryptTelemetryEntry(t, output)
	if entry["unavailable_type"] != "unknown" || entry["decrypt_fail_mode"] != "unknown" {
		t.Fatalf("unexpected enum values must fail closed: %#v", entry)
	}
	serialized := mustMarshalDecryptTelemetry(t, entry)
	for _, sentinel := range []string{"UNAVAILABLE-TYPE-SENTINEL", "DECRYPT-FAIL-MODE-SENTINEL"} {
		if strings.Contains(serialized, sentinel) {
			t.Fatalf("serialized decrypt telemetry exposed enum sentinel")
		}
	}
}

func TestLogDecryptRecoverySuppressesZeroRetryCount(t *testing.T) {
	output := captureDecryptTelemetryLogs(t, logrus.InfoLevel)
	logDecryptRecovery(NewDeviceInstance("18095550111@s.whatsapp.net", nil, nil), &events.Message{
		Info:       sensitiveDecryptMessageInfo(),
		RetryCount: 0,
	})
	if strings.TrimSpace(output.String()) != "" {
		t.Fatalf("zero retry count emitted recovery telemetry: %s", output.String())
	}
}

func TestLogDecryptRecoveryEmitsPositiveRetryAndSameReference(t *testing.T) {
	instance := NewDeviceInstance("18095550111@s.whatsapp.net", nil, nil)
	info := sensitiveDecryptMessageInfo()

	failureOutput := captureDecryptTelemetryLogs(t, logrus.WarnLevel)
	logUndecryptableMessage(instance, &events.UndecryptableMessage{Info: info})
	failureEntry := decodeSingleDecryptTelemetryEntry(t, failureOutput)

	recoveryOutput := captureDecryptTelemetryLogs(t, logrus.InfoLevel)
	logDecryptRecovery(instance, &events.Message{Info: info, RetryCount: 2})
	recoveryEntry := decodeSingleDecryptTelemetryEntry(t, recoveryOutput)

	if recoveryEntry["event"] != "signal_decrypt" || recoveryEntry["outcome"] != "recovered" {
		t.Fatalf("unexpected recovery telemetry: %#v", recoveryEntry)
	}
	if recoveryEntry["retry_count"] != float64(2) {
		t.Fatalf("retry_count = %v, want 2", recoveryEntry["retry_count"])
	}
	if recoveryEntry["message_ref"] != failureEntry["message_ref"] || recoveryEntry["device_ref"] != failureEntry["device_ref"] {
		t.Fatal("recovery telemetry did not correlate with failure telemetry")
	}
	assertNoDecryptTelemetrySentinel(t, mustMarshalDecryptTelemetry(t, recoveryEntry))
}

func TestHandlerDispatchesUndecryptableMessage(t *testing.T) {
	output := captureDecryptTelemetryLogs(t, logrus.WarnLevel)
	instance := NewDeviceInstance("18095550111@s.whatsapp.net", nil, nil)

	handler(context.Background(), instance, &events.UndecryptableMessage{
		Info:            sensitiveDecryptMessageInfo(),
		IsUnavailable:   true,
		UnavailableType: events.UnavailableTypeViewOnce,
	})

	entry := decodeSingleDecryptTelemetryEntry(t, output)
	if entry["event"] != "signal_decrypt" || entry["outcome"] != "failed" {
		t.Fatalf("handler did not dispatch undecryptable telemetry: %#v", entry)
	}
}

func sensitiveDecryptMessageInfo() types.MessageInfo {
	return types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     types.NewJID("18095550123", types.DefaultUserServer),
			Sender:   types.NewJID("18095550999", types.DefaultUserServer),
			IsFromMe: false,
			IsGroup:  false,
		},
		ID:       "MESSAGE-ID-SENTINEL",
		PushName: "PUSH-NAME-SENTINEL",
		Type: strings.Join([]string{
			"PRIVATE-BODY-SENTINEL", "CIPHERTEXT-SENTINEL", "SIGNAL-KEY-SENTINEL",
			"WEBHOOK-SECRET-SENTINEL", "CHATWOOT-CREDENTIAL-SENTINEL",
			"postgres://DATABASE-URI-SENTINEL", "Bearer AUTHORIZATION-SENTINEL",
			"DECRYPT-ERROR-SENTINEL",
		}, " "),
		Category: "CATEGORY-SENTINEL",
	}
}

func captureDecryptTelemetryLogs(t *testing.T, level logrus.Level) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	logger := logrus.StandardLogger()
	previousOutput := logger.Out
	previousFormatter := logger.Formatter
	previousLevel := logger.Level
	t.Cleanup(func() {
		logger.SetOutput(previousOutput)
		logger.SetFormatter(previousFormatter)
		logger.SetLevel(previousLevel)
	})
	logger.SetOutput(&output)
	logger.SetFormatter(&logrus.JSONFormatter{DisableTimestamp: true})
	logger.SetLevel(level)
	return &output
}

func decodeSingleDecryptTelemetryEntry(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); err != nil {
		t.Fatalf("decode structured decrypt telemetry: %v; output=%q", err, output.String())
	}
	return entry
}

func mustMarshalDecryptTelemetry(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal decrypt telemetry: %v", err)
	}
	return string(encoded)
}

func assertNoDecryptTelemetrySentinel(t *testing.T, serialized string) {
	t.Helper()
	for _, sentinel := range []string{
		"18095550111", "18095550123", "18095550999", "MESSAGE-ID-SENTINEL",
		"PUSH-NAME-SENTINEL", "CATEGORY-SENTINEL", "PRIVATE-BODY-SENTINEL",
		"CIPHERTEXT-SENTINEL", "SIGNAL-KEY-SENTINEL", "WEBHOOK-SECRET-SENTINEL",
		"CHATWOOT-CREDENTIAL-SENTINEL", "postgres://DATABASE-URI-SENTINEL",
		"Bearer AUTHORIZATION-SENTINEL", "DECRYPT-ERROR-SENTINEL",
	} {
		if strings.Contains(serialized, sentinel) {
			t.Fatalf("serialized decrypt telemetry exposed a sensitive sentinel")
		}
	}
	for _, unsupported := range []string{"mac_mismatch", "retry_sent", "retry_exhausted"} {
		if strings.Contains(serialized, unsupported) {
			t.Fatalf("serialized decrypt telemetry claimed unsupported cause %q", unsupported)
		}
	}
}
