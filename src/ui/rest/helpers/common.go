package helpers

import (
	"context"
	"mime/multipart"
	"strings"
	"time"

	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
)

// autoConnectService is the narrow slice of IAppUsecase that boot-time
// auto-connect needs; satisfied structurally by the app usecase so tests can
// drive the loop with a stub.
type autoConnectService interface {
	FetchDevices(ctx context.Context) ([]domainApp.DevicesResponse, error)
	Reconnect(ctx context.Context, deviceID string) error
}

// shouldAutoConnectFn is the auto-connect identity gate (RET-427-FU7b):
// sessions with no registered device record, or marked disconnected by the
// optional identity service, are skipped+warned instead of connected. Seam
// for tests, same defer/restore idiom as the fork's other test seams.
var shouldAutoConnectFn = whatsapp.ShouldAutoConnect

func SetAutoConnectAfterBooting(service autoConnectService) {
	time.Sleep(2 * time.Second)
	devices, err := service.FetchDevices(context.Background())
	if err != nil || len(devices) == 0 {
		logrus.Warn("auto-connect skipped: no devices available")
		return
	}
	attempted := 0
	connected := 0
	staleSessionDeleted := 0
	skippedNoRecord := 0
	skippedIdentityDisconnected := 0
	failed := 0
	for _, device := range devices {
		attempted++
		verdict := shouldAutoConnectFn(context.Background(), device.Device, device.JID)
		if !verdict.Allow {
			switch verdict.Reason {
			case whatsapp.AutoConnectSkipNoRegisteredRecord:
				skippedNoRecord++
			case whatsapp.AutoConnectSkipIdentityDisconnected:
				skippedIdentityDisconnected++
			}
			continue
		}
		if err := service.Reconnect(context.Background(), device.Device); err != nil {
			if IsAutoConnectSessionDeleted(err) {
				staleSessionDeleted++
				logrus.Debugf("auto-connect skipped stale deleted session device=%s", device.Device)
				continue
			}
			failed++
			logrus.Warnf("auto-connect failed for device %s: %v", device.Device, err)
		} else {
			connected++
			logrus.Debugf("auto-connected device %s", device.Device)
		}
	}
	fields := logrus.Fields{
		"attempted":                     attempted,
		"connected":                     connected,
		"stale_session_deleted":         staleSessionDeleted,
		"skipped_no_registered_record":  skippedNoRecord,
		"skipped_identity_disconnected": skippedIdentityDisconnected,
		"failed":                        failed,
	}
	if failed > 0 {
		logrus.WithFields(fields).Warn("auto-connect completed with failures")
	} else {
		logrus.WithFields(fields).Info("auto-connect completed")
	}
}

func IsAutoConnectSessionDeleted(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "not logged in") && strings.Contains(text, "session deleted")
}

func SetAutoReconnectChecking(cli *whatsmeow.Client) {
	if cli == nil {
		logrus.Warn("SetAutoReconnectChecking was called with a nil WhatsApp client; skipping auto-reconnect loop")
		return
	}
	// Run every 5 minutes to check if the connection is still alive, if not, reconnect
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			if !cli.IsConnected() {
				_ = cli.Connect()
			}
		}
	}()
}

func MultipartFormFileHeaderToBytes(fileHeader *multipart.FileHeader) []byte {
	file, _ := fileHeader.Open()
	defer file.Close()

	fileBytes := make([]byte, fileHeader.Size)
	_, _ = file.Read(fileBytes)

	return fileBytes
}
