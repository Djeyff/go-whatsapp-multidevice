package whatsapp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

func safeLogRef(namespace string, values ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(namespace))
	_, _ = hash.Write([]byte{0})
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)[:6])
}

func clientLoggerName(deviceID string) string {
	return fmt.Sprintf("Client-%s", safeLogRef("whatsapp-client", deviceID))
}

func safeErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	errorType := reflect.TypeOf(err)
	if errorType.Kind() == reflect.Pointer {
		errorType = errorType.Elem()
	}
	return safeLogRef("error-type", errorType.PkgPath(), errorType.Name())
}

func safeMessageLogFields(client *whatsmeow.Client, evt *events.Message) logrus.Fields {
	fields := logrus.Fields{
		"event":       "whatsapp_message_received",
		"message_ref": safeLogRef("message", "missing"),
		"chat_ref":    safeLogRef("chat", "missing"),
		"sender_ref":  safeLogRef("sender", "missing"),
		"device_ref":  safeLogRef("device", "missing"),
		"from_me":     false,
		"is_group":    false,
		"view_once":   false,
		"has_message": false,
		"timestamp":   time.Time{}.UTC().Format(time.RFC3339),
	}
	if client != nil && client.Store != nil && client.Store.ID != nil {
		fields["device_ref"] = safeLogRef("device", client.Store.ID.ToNonAD().String())
	}
	if evt == nil {
		return fields
	}
	fields["message_ref"] = safeLogRef("message", string(evt.Info.ID))
	fields["chat_ref"] = safeLogRef("chat", evt.Info.Chat.ToNonAD().String())
	fields["sender_ref"] = safeLogRef("sender", evt.Info.Sender.ToNonAD().String())
	fields["from_me"] = evt.Info.IsFromMe
	fields["is_group"] = evt.Info.IsGroup
	fields["view_once"] = evt.IsViewOnce
	fields["has_message"] = evt.Message != nil
	fields["timestamp"] = evt.Info.Timestamp.UTC().Format(time.RFC3339)
	return fields
}

func logReceivedMessage(client *whatsmeow.Client, evt *events.Message) {
	logrus.WithFields(safeMessageLogFields(client, evt)).Info("WhatsApp message received")
}
