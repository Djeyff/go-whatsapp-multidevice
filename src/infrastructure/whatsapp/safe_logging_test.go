package whatsapp

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestSafeLogRefIsStableAndDomainSeparated(t *testing.T) {
	first := safeLogRef("message", "ab", "c")
	if first != safeLogRef("message", "ab", "c") {
		t.Fatal("safe log refs must be stable")
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(first) {
		t.Fatalf("safe log ref shape = %q, want 12 lowercase hex characters", first)
	}
	if first == safeLogRef("device", "ab", "c") {
		t.Fatal("safe log refs must be namespace separated")
	}
	if first == safeLogRef("message", "a", "bc") {
		t.Fatal("safe log refs must delimit values")
	}
}

func TestSafeLogRefDoesNotContainSourceIdentifier(t *testing.T) {
	source := "18095550123@s.whatsapp.net"
	ref := safeLogRef("jid", source)
	if strings.Contains(ref, source) || strings.Contains(ref, "18095550123") {
		t.Fatalf("safe log ref exposed source identifier: %q", ref)
	}
}

func TestClientLoggerNameRedactsWhatsAppJID(t *testing.T) {
	deviceID := "18095550123@s.whatsapp.net"
	name := clientLoggerName(deviceID)
	if strings.Contains(name, deviceID) || strings.Contains(name, "18095550123") {
		t.Fatalf("client logger name exposed raw device identifier: %q", name)
	}
	if !regexp.MustCompile(`^Client-[0-9a-f]{12}$`).MatchString(name) {
		t.Fatalf("client logger name = %q, want Client plus safe ref", name)
	}
}

func TestSafeMessageLogFieldsExcludeSensitivePayloadAndIdentifiers(t *testing.T) {
	evt := sensitiveMessageEventForLoggingTest()
	fields := safeMessageLogFields(nil, evt)

	allowed := map[string]bool{
		"event": true, "message_ref": true, "chat_ref": true, "sender_ref": true,
		"device_ref": true, "from_me": true, "is_group": true, "view_once": true,
		"has_message": true, "timestamp": true,
	}
	for key := range fields {
		if !allowed[key] {
			t.Fatalf("unexpected message log field %q", key)
		}
	}

	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal safe fields: %v", err)
	}
	assertNoLoggingSentinel(t, string(encoded))
}

func TestLogReceivedMessageEmitsOnlyAllowlistedFields(t *testing.T) {
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
	logger.SetLevel(logrus.InfoLevel)

	logReceivedMessage(nil, sensitiveMessageEventForLoggingTest())

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); err != nil {
		t.Fatalf("decode structured log: %v", err)
	}
	allowed := map[string]bool{
		"level": true, "msg": true, "event": true, "message_ref": true,
		"chat_ref": true, "sender_ref": true, "device_ref": true,
		"from_me": true, "is_group": true, "view_once": true,
		"has_message": true, "timestamp": true,
	}
	for key := range entry {
		if !allowed[key] {
			t.Fatalf("unexpected serialized log field %q", key)
		}
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal log entry: %v", err)
	}
	assertNoLoggingSentinel(t, string(encoded))
}

func sensitiveMessageEventForLoggingTest() *events.Message {
	sensitiveBody := strings.Join([]string{
		"PRIVATE-BODY-SENTINEL", "CIPHERTEXT-SENTINEL", "SIGNAL-KEY-SENTINEL",
		"WEBHOOK-SECRET-SENTINEL", "CHATWOOT-CREDENTIAL-SENTINEL",
		"postgres://DATABASE-URI-SENTINEL", "Bearer AUTHORIZATION-SENTINEL",
	}, " ")
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("18095550123", types.DefaultUserServer),
				Sender:   types.NewJID("18095550999", types.DefaultUserServer),
				IsFromMe: false,
				IsGroup:  false,
			},
			ID:        "MESSAGE-ID-SENTINEL",
			PushName:  "PUSH-NAME-SENTINEL",
			Type:      "TEXT-TYPE-SENTINEL",
			Category:  "CATEGORY-SENTINEL",
			Timestamp: time.Date(2026, time.July, 12, 1, 15, 0, 0, time.UTC),
		},
		Message: &waE2E.Message{Conversation: protoString(sensitiveBody)},
	}
}

func assertNoLoggingSentinel(t *testing.T, serialized string) {
	t.Helper()
	for _, sentinel := range []string{
		"18095550123", "18095550999", "MESSAGE-ID-SENTINEL", "PUSH-NAME-SENTINEL",
		"TEXT-TYPE-SENTINEL", "CATEGORY-SENTINEL", "PRIVATE-BODY-SENTINEL",
		"CIPHERTEXT-SENTINEL", "SIGNAL-KEY-SENTINEL", "WEBHOOK-SECRET-SENTINEL",
		"CHATWOOT-CREDENTIAL-SENTINEL", "postgres://DATABASE-URI-SENTINEL",
		"Bearer AUTHORIZATION-SENTINEL",
	} {
		if strings.Contains(serialized, sentinel) {
			t.Fatalf("serialized log exposed a sensitive sentinel")
		}
	}
}
