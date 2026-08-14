package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	pkgUtils "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/websocket"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/validations"
	fiberUtils "github.com/gofiber/fiber/v2/utils"
	"github.com/sirupsen/logrus"
	"go.mau.fi/libsignal/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type serviceApp struct {
	chatStorageRepo domainChatStorage.IChatStorageRepository
	deviceManager   *whatsapp.DeviceManager
}

const retenaPairingSessionMaxLifetime = 3 * time.Minute

func pairPhoneAfterReadiness(ctx context.Context, session *whatsapp.PairingSession, pair func(context.Context) (string, error)) (string, error) {
	if session == nil {
		return "", fmt.Errorf("pairing session is unavailable")
	}
	if pair == nil {
		return "", fmt.Errorf("phone pairing provider is unavailable")
	}
	if err := session.WaitReady(ctx); err != nil {
		return "", err
	}
	if !session.BeginPhonePairing() {
		return "", fmt.Errorf("phone pairing is already in progress or unavailable")
	}
	defer session.EndPhonePairing()
	return pair(ctx)
}

func NewAppService(chatStorageRepo domainChatStorage.IChatStorageRepository, deviceManager *whatsapp.DeviceManager) domainApp.IAppUsecase {
	return &serviceApp{
		chatStorageRepo: chatStorageRepo,
		deviceManager:   deviceManager,
	}
}

func (service *serviceApp) getOrStartPairingSession(instance *whatsapp.DeviceInstance, client *whatsmeow.Client, maxLifetime time.Duration) (*whatsapp.PairingSession, error) {
	session, created := instance.GetOrCreatePairingSession(func() *whatsapp.PairingSession {
		return whatsapp.NewPairingSession(context.Background(), fiberUtils.UUIDv4())
	})
	if session == nil {
		return nil, fmt.Errorf("pairing session is unavailable")
	}
	if !created {
		return session, nil
	}

	if client.IsConnected() {
		client.Disconnect()
	}
	instance.ClearPasskeyState()

	events, err := client.GetQRChannel(session.Context())
	if err != nil {
		reasonClass := classifyQRChannelStartError(err)
		session.MarkTerminal(whatsapp.PairingSessionFailed, reasonClass, time.Now())
		whatsapp.NotifySessionEvent(context.Background(), instance, "reconnect_blocked", "attention", reasonClass, "WhatsApp pairing session could not start")
		if errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
			_ = client.Connect()
			instance.UpdateStateFromClient()
			if client.IsLoggedIn() {
				return session, pkgError.ErrAlreadyLoggedIn
			}
			return session, pkgError.ErrSessionSaved
		}
		return session, pkgError.ErrQrChannel
	}

	go func() {
		whatsapp.ConsumePairingQRChannel(session, events, whatsapp.PairingQRConsumer{
			MaxLifetime: maxLifetime,
			OnLifetimeExpired: func() {
				client.Disconnect()
				instance.UpdateStateFromClient()
			},
			WriteImage: func(code string) (string, error) {
				path := fmt.Sprintf("%s/scan-qr-%s.png", config.PathQrCode, fiberUtils.UUIDv4())
				if err := pkgUtils.WriteQRWithLogo(code, 512, path); err != nil {
					return "", err
				}
				return path, nil
			},
			RemoveImageAfter: func(path string, after time.Duration) {
				time.AfterFunc(after+30*time.Second, func() {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						logrus.Warnf("[LOGIN] failed to remove expired QR image")
					}
				})
			},
		})
		if eventType, severity, reasonClass, summary, ok := classifyPairingSessionEvent(session.Snapshot()); ok {
			whatsapp.NotifySessionEvent(context.Background(), instance, eventType, severity, reasonClass, summary)
		}
	}()

	if err := client.Connect(); err != nil {
		session.MarkTerminal(whatsapp.PairingSessionFailed, "connect_failed", time.Now())
		whatsapp.NotifySessionEvent(context.Background(), instance, "reconnect_blocked", "attention", "connect_failed", "WhatsApp pairing session could not connect")
		logger.Error("Error when connect to whatsapp", err)
		return session, pkgError.ErrReconnect
	}
	instance.UpdateStateFromClient()
	return session, nil
}

func classifyQRChannelStartError(err error) string {
	switch {
	case errors.Is(err, whatsmeow.ErrQRAlreadyConnected):
		return "qr_already_connected"
	case errors.Is(err, whatsmeow.ErrQRStoreContainsID):
		return "session_already_saved"
	default:
		return "qr_channel_start_failed"
	}
}

func loginResponseFromPairingSnapshot(snapshot whatsapp.PairingSessionSnapshot, now time.Time) domainApp.LoginResponse {
	remaining := snapshot.ValidUntil.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	return domainApp.LoginResponse{
		ImagePath:        snapshot.ImagePath,
		Duration:         int64(remaining / time.Second),
		PairingSessionID: snapshot.SessionID,
		QRGeneration:     snapshot.Generation,
		EmittedAt:        snapshot.EmittedAt,
		ValidUntil:       snapshot.ValidUntil,
		State:            string(snapshot.State),
		ErrorCode:        snapshot.ErrorCode,
	}
}

func (service *serviceApp) Login(ctx context.Context, deviceID string) (response domainApp.LoginResponse, err error) {
	instance, client, err := service.ensureClient(ctx, deviceID)
	if err != nil {
		return response, err
	}

	if client.IsLoggedIn() {
		instance.UpdateStateFromClient()
		return response, pkgError.ErrAlreadyLoggedIn
	}

	session, err := service.getOrStartPairingSession(instance, client, retenaPairingSessionMaxLifetime)
	if err != nil {
		return response, err
	}
	if err := session.WaitReady(ctx); err != nil {
		return loginResponseFromPairingSnapshot(session.Snapshot(), time.Now()), err
	}
	return loginResponseFromPairingSnapshot(session.Snapshot(), time.Now()), nil
}

func (service *serviceApp) LoginWithCode(ctx context.Context, deviceID string, phoneNumber string) (loginCode string, err error) {
	if err = validations.ValidateLoginWithCode(ctx, phoneNumber); err != nil {
		logrus.Errorf("Error when validate login with code: %s", err.Error())
		return loginCode, err
	}

	instance, client, err := service.ensureClient(ctx, deviceID)
	if err != nil {
		return loginCode, err
	}

	if client.IsLoggedIn() {
		instance.UpdateStateFromClient()
		return loginCode, pkgError.ErrAlreadyLoggedIn
	}

	session, err := service.getOrStartPairingSession(instance, client, 0)
	if err != nil {
		whatsapp.NotifySessionEvent(context.Background(), instance, "reconnect_blocked", "attention", "pairing_code_connect_failed", "WhatsApp pairing-code login could not establish a connection")
		return loginCode, err
	}

	logrus.Infof("[LOGIN_CODE][%s] Starting phone pairing after QR readiness", deviceID)
	loginCode, err = pairPhoneAfterReadiness(ctx, session, func(pairCtx context.Context) (string, error) {
		return client.PairPhone(pairCtx, phoneNumber, true, whatsmeow.PairClientOtherWebClient, "Chrome (Linux)")
	})
	if err != nil {
		logrus.Warnf("[LOGIN_CODE][%s] phone pairing failed", deviceID)
		whatsapp.NotifySessionEvent(context.Background(), instance, "reconnect_blocked", "attention", "pairing_code_request_failed", "WhatsApp pairing-code request failed")
		return loginCode, err
	}

	instance.UpdateStateFromClient()
	logrus.Infof("[LOGIN_CODE][%s] phone pairing code generated", deviceID)
	return loginCode, nil
}

func (service *serviceApp) PasskeyChallenge(_ context.Context, deviceID string) (response domainApp.PasskeyChallengeResponse, err error) {
	if service.deviceManager == nil {
		return response, fmt.Errorf("device manager not initialized")
	}

	instance, ok := service.deviceManager.GetDevice(deviceID)
	if !ok || instance == nil {
		return response, fmt.Errorf("device %s not found", deviceID)
	}

	challenge, code, skipHandoffUX := instance.PasskeyState()
	response.Status = "none"
	if challenge != nil {
		response.Status = "awaiting_response"
	} else if code != "" {
		response.Status = "awaiting_confirmation"
	}
	response.Challenge = challenge
	response.Code = code
	response.SkipHandoffUX = skipHandoffUX
	return response, nil
}

func (service *serviceApp) PasskeyResponse(ctx context.Context, deviceID string, assertion *types.WebAuthnResponse) error {
	if err := validations.ValidatePasskeyResponse(ctx, assertion); err != nil {
		return err
	}

	if service.deviceManager == nil {
		return fmt.Errorf("device manager not initialized")
	}

	instance, ok := service.deviceManager.GetDevice(deviceID)
	if !ok || instance == nil {
		return fmt.Errorf("device %s not found", deviceID)
	}

	client := instance.GetClient()
	if client == nil {
		return pkgError.ErrWaCLI
	}

	challenge, _, _ := instance.PasskeyState()
	if challenge == nil {
		return fmt.Errorf("no pending passkey pairing request for device %s", deviceID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("device %s is not connected, restart login and retry the passkey flow", deviceID)
	}

	if err := client.SendPasskeyResponse(ctx, assertion); err != nil {
		logrus.Errorf("[PASSKEY][%s] failed to send passkey response: %v", deviceID, err)
		return err
	}

	// The PairPasskeyConfirmation event repopulates the confirmation code shortly after.
	instance.ClearPasskeyState()
	return nil
}

func (service *serviceApp) PasskeyConfirm(ctx context.Context, deviceID string) error {
	if service.deviceManager == nil {
		return fmt.Errorf("device manager not initialized")
	}

	instance, ok := service.deviceManager.GetDevice(deviceID)
	if !ok || instance == nil {
		return fmt.Errorf("device %s not found", deviceID)
	}

	client := instance.GetClient()
	if client == nil {
		return pkgError.ErrWaCLI
	}

	_, code, _ := instance.PasskeyState()
	if code == "" {
		return fmt.Errorf("no pending passkey confirmation for device %s", deviceID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("device %s is not connected, restart login and retry the passkey flow", deviceID)
	}

	if err := client.SendPasskeyConfirmation(ctx); err != nil {
		logrus.Errorf("[PASSKEY][%s] failed to send passkey confirmation: %v", deviceID, err)
		return err
	}

	instance.ClearPasskeyState()
	return nil
}

func (service *serviceApp) Logout(ctx context.Context, deviceID string) error {
	if err := validations.ValidateDeviceID(ctx, deviceID); err != nil {
		return err
	}
	if service.deviceManager == nil {
		return fmt.Errorf("device manager not initialized")
	}

	if err := service.deviceManager.LogoutDeviceKeepSlot(ctx, deviceID); err != nil {
		logrus.WithError(err).Warnf("[LOGOUT][%s] logout completed with warnings", deviceID)
		return err
	}

	// Broadcast the logout so the UI can refresh without manual polling. The slot is
	// kept, so the device stays listed (disconnected) and can be re-paired by id.
	var devices []domainApp.DevicesResponse
	if list, err := service.FetchDevices(ctx); err == nil {
		devices = list
	} else {
		logrus.WithError(err).Warn("[LOGOUT] failed to fetch devices after logout")
	}

	websocket.Broadcast <- websocket.BroadcastMessage{
		Code:    "DEVICE_LOGGED_OUT",
		Message: fmt.Sprintf("Device %s logged out (slot kept)", deviceID),
		Result: map[string]any{
			"device_id": deviceID,
			"devices":   devices,
		},
	}

	return nil
}

func (service *serviceApp) Reconnect(_ context.Context, deviceID string) (err error) {
	instance, client, err := service.ensureClient(context.Background(), deviceID)
	if err != nil {
		return err
	}

	if client.Store == nil || client.Store.ID == nil {
		whatsapp.NotifySessionEvent(context.Background(), instance, "reconnect_blocked", "critical", "session_deleted", "WhatsApp stored session is missing and cannot reconnect automatically")
		return fmt.Errorf("device %s is not logged in (session deleted)", deviceID)
	}

	client.Disconnect()
	err = client.Connect()
	instance.UpdateStateFromClient()
	if err != nil {
		logrus.Errorf("[RECONNECT][%s] Reconnect failed: %v", deviceID, err)
		whatsapp.NotifySessionEvent(context.Background(), instance, "reconnect_blocked", "attention", "reconnect_failed", "WhatsApp reconnect attempt failed")
	}
	return err
}

func classifyPairingSessionEvent(snapshot whatsapp.PairingSessionSnapshot) (string, string, string, string, bool) {
	switch snapshot.State {
	case whatsapp.PairingSessionExpired:
		return "qr_expired", "attention", snapshot.ErrorCode, "WhatsApp QR login expired before pairing completed", true
	case whatsapp.PairingSessionFailed:
		if snapshot.ErrorCode == "client_outdated" {
			return "reconnect_blocked", "attention", snapshot.ErrorCode, "WhatsApp QR login was blocked because the client is out of date", true
		}
		return "qr_invalid", "attention", snapshot.ErrorCode, "WhatsApp QR login returned an invalid or unexpected QR event", true
	default:
		return "", "", "", "", false
	}
}

func (service *serviceApp) Status(_ context.Context, deviceID string) (bool, bool, error) {
	if service.deviceManager == nil {
		return false, false, fmt.Errorf("device manager not initialized")
	}

	instance, ok := service.deviceManager.GetDevice(deviceID)
	if !ok || instance == nil {
		return false, false, fmt.Errorf("device %s not found", deviceID)
	}

	instance.UpdateStateFromClient()
	client := instance.GetClient()
	if client == nil {
		return false, false, nil
	}

	if client.Store == nil || client.Store.ID == nil {
		return false, false, nil
	}

	return client.IsConnected(), client.IsLoggedIn(), nil
}

func (service *serviceApp) FirstDevice(ctx context.Context) (response domainApp.DevicesResponse, err error) {
	devices, err := service.FetchDevices(ctx)
	if err != nil {
		return response, err
	}
	if len(devices) == 0 {
		return response, fmt.Errorf("no devices available")
	}
	return devices[0], nil
}

func (service *serviceApp) FetchDevices(_ context.Context) (response []domainApp.DevicesResponse, err error) {
	if service.deviceManager == nil {
		return response, fmt.Errorf("device manager not initialized")
	}

	for _, inst := range service.deviceManager.ListDevices() {
		inst.UpdateStateFromClient()
		name := inst.DisplayName()
		if name == "" {
			name = inst.PhoneNumber()
		}

		response = append(response, domainApp.DevicesResponse{
			Name:   name,
			Device: inst.ID(),
			JID:    inst.JID(),
		})
	}

	return response, nil
}

func (service *serviceApp) ensureClient(ctx context.Context, deviceID string) (*whatsapp.DeviceInstance, *whatsmeow.Client, error) {
	if deviceID == "" {
		return nil, nil, fmt.Errorf("device id is required")
	}

	if service.deviceManager == nil {
		return nil, nil, fmt.Errorf("device manager not initialized")
	}

	instance, err := service.deviceManager.EnsureClient(ctx, deviceID)
	if err != nil {
		return nil, nil, err
	}

	client := instance.GetClient()
	if client == nil {
		return instance, nil, pkgError.ErrWaCLI
	}

	return instance, client, nil
}
