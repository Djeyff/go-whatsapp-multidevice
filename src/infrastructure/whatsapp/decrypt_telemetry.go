package whatsapp

import (
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func decryptTelemetryFields(instance *DeviceInstance, info types.MessageInfo) logrus.Fields {
	deviceID := "missing"
	if instance != nil {
		if instance.JID() != "" {
			deviceID = instance.JID()
		} else if instance.ID() != "" {
			deviceID = instance.ID()
		}
	}
	messageID := string(info.ID)
	if messageID == "" {
		messageID = "missing"
	}
	return logrus.Fields{
		"event":       "signal_decrypt",
		"message_ref": safeLogRef("signal-message", messageID),
		"device_ref":  safeLogRef("signal-device", deviceID),
		"from_me":     info.IsFromMe,
		"is_group":    info.IsGroup,
	}
}

func logUndecryptableMessage(instance *DeviceInstance, evt *events.UndecryptableMessage) {
	if evt == nil {
		return
	}
	fields := decryptTelemetryFields(instance, evt.Info)
	fields["outcome"] = "failed"
	fields["is_unavailable"] = evt.IsUnavailable
	fields["unavailable_type"] = safeUnavailableType(evt.UnavailableType)
	fields["decrypt_fail_mode"] = safeDecryptFailMode(evt.DecryptFailMode)
	logrus.WithFields(fields).Warn("Signal decrypt failed")
}

func logDecryptRecovery(instance *DeviceInstance, evt *events.Message) {
	if evt == nil || evt.RetryCount <= 0 {
		return
	}
	fields := decryptTelemetryFields(instance, evt.Info)
	fields["outcome"] = "recovered"
	fields["retry_count"] = evt.RetryCount
	logrus.WithFields(fields).Info("Signal decrypt recovered")
}

func safeUnavailableType(value events.UnavailableType) string {
	if value == events.UnavailableTypeViewOnce {
		return "view_once"
	}
	return "unknown"
}

func safeDecryptFailMode(value events.DecryptFailMode) string {
	switch value {
	case events.DecryptFailHide:
		return "hide"
	case events.DecryptFailShow:
		return "show"
	default:
		return "unknown"
	}
}
