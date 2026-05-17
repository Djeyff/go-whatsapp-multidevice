package helpers

import (
	"context"
	"mime/multipart"
	"strings"
	"time"

	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
)

func SetAutoConnectAfterBooting(service domainApp.IAppUsecase) {
	time.Sleep(2 * time.Second)
	devices, err := service.FetchDevices(context.Background())
	if err != nil || len(devices) == 0 {
		logrus.Warn("auto-connect skipped: no devices available")
		return
	}
	attempted := 0
	connected := 0
	staleSessionDeleted := 0
	failed := 0
	for _, device := range devices {
		attempted++
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
		"attempted":             attempted,
		"connected":             connected,
		"stale_session_deleted": staleSessionDeleted,
		"failed":                failed,
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
