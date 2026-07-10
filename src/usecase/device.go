package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainDevice "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/device"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/websocket"
	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"google.golang.org/protobuf/proto"
)

type serviceDevice struct {
	manager                    *whatsapp.DeviceManager
	historySyncMu              sync.Mutex
	lastFullHistorySyncRequest map[string]time.Time
}

const (
	defaultFullHistorySyncDays = 3
	maxFullHistorySyncDays     = 30
	fullHistorySyncCooldown    = 2 * time.Minute
)

var errFullHistorySyncCooldown = errors.New("full history sync cooldown active")

func NewDeviceService(manager *whatsapp.DeviceManager) domainDevice.IDeviceUsecase {
	return &serviceDevice{
		manager:                    manager,
		lastFullHistorySyncRequest: make(map[string]time.Time),
	}
}

func (s *serviceDevice) ListDevices(_ context.Context) ([]domainDevice.Device, error) {
	if s.manager == nil {
		return []domainDevice.Device{}, nil
	}

	var result []domainDevice.Device
	for _, inst := range s.manager.ListDevices() {
		inst.UpdateStateFromClient()
		result = append(result, convertInstance(inst))
	}
	return result, nil
}

func (s *serviceDevice) GetDevice(_ context.Context, deviceID string) (*domainDevice.Device, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("device manager not initialized")
	}
	if inst, ok := s.manager.GetDevice(deviceID); ok {
		device := convertInstance(inst)
		return &device, nil
	}
	return nil, fmt.Errorf("device %s not found", deviceID)
}

func (s *serviceDevice) AddDevice(ctx context.Context, deviceID string, webhook *domainChatStorage.DeviceWebhookConfig) (*domainDevice.Device, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("device manager not initialized")
	}

	inst, err := s.manager.CreateDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if webhook != nil {
		storage := s.manager.GetStorage()
		if storage == nil {
			return nil, fmt.Errorf("device %s created but storage is unavailable to save webhook config", deviceID)
		}
		if err := storage.SetDeviceWebhookConfig(inst.ID(), webhook); err != nil {
			return nil, fmt.Errorf("device %s created but webhook config could not be saved: %w", deviceID, err)
		}
	}
	device := convertInstance(inst)
	return &device, nil
}

func (s *serviceDevice) RemoveDevice(_ context.Context, deviceID string) error {
	if s.manager == nil {
		return fmt.Errorf("device manager not initialized")
	}
	s.manager.RemoveDevice(deviceID)
	return nil
}

func (s *serviceDevice) LoginDevice(_ context.Context, _ string) error {
	return fmt.Errorf("device login per ID is not implemented yet")
}

func (s *serviceDevice) LoginDeviceWithCode(_ context.Context, _ string, _ string) (string, error) {
	return "", fmt.Errorf("device login with code is not implemented yet")
}

func (s *serviceDevice) LogoutDevice(ctx context.Context, deviceID string) error {
	if s.manager == nil {
		return fmt.Errorf("device manager not initialized")
	}

	if err := s.manager.PurgeDevice(ctx, deviceID); err != nil {
		return err
	}

	// Broadcast device removal so UI clients can refresh.
	var devices []domainDevice.Device
	if s.manager != nil {
		for _, inst := range s.manager.ListDevices() {
			inst.UpdateStateFromClient()
			devices = append(devices, convertInstance(inst))
		}
	}

	websocket.Broadcast <- websocket.BroadcastMessage{
		Code:    "DEVICE_REMOVED",
		Message: fmt.Sprintf("Device %s logged out and removed", deviceID),
		Result: map[string]any{
			"device_id": deviceID,
			"devices":   devices,
		},
	}

	return nil
}

func (s *serviceDevice) ReconnectDevice(_ context.Context, deviceID string) error {
	if s.manager == nil {
		return fmt.Errorf("device manager not initialized")
	}
	if inst, ok := s.manager.GetDevice(deviceID); ok {
		client := inst.GetClient()
		if client == nil {
			whatsapp.NotifySessionEvent(context.Background(), inst, "reconnect_blocked", "attention", "device_client_missing", "WhatsApp reconnect could not start because the device client is missing")
			return fmt.Errorf("device %s client not initialized", deviceID)
		}

		if client.Store == nil || client.Store.ID == nil {
			whatsapp.NotifySessionEvent(context.Background(), inst, "reconnect_blocked", "critical", "session_deleted", "WhatsApp stored session is missing and cannot reconnect automatically")
			return fmt.Errorf("device %s is not logged in (session deleted)", deviceID)
		}

		client.Disconnect()
		if err := client.Connect(); err != nil {
			whatsapp.NotifySessionEvent(context.Background(), inst, "reconnect_blocked", "attention", "device_reconnect_failed", "WhatsApp device reconnect attempt failed")
			return err
		}
		return nil
	}
	return fmt.Errorf("device %s not found", deviceID)
}

func (s *serviceDevice) RequestFullHistorySync(ctx context.Context, deviceID string, days int) (*domainDevice.FullHistorySyncRequest, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("device manager not initialized")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, fmt.Errorf("device id is required")
	}
	requestedDays := days
	if days <= 0 {
		days = defaultFullHistorySyncDays
	}
	if days > maxFullHistorySyncDays {
		days = maxFullHistorySyncDays
	}

	inst, ok := s.manager.GetDevice(deviceID)
	if !ok || inst == nil {
		return nil, fmt.Errorf("device %s not found", deviceID)
	}

	client := inst.GetClient()
	if client == nil {
		return nil, fmt.Errorf("device %s client not initialized", deviceID)
	}
	if client.Store == nil || client.Store.ID == nil || !client.IsLoggedIn() {
		return nil, fmt.Errorf("device %s is not logged in", deviceID)
	}
	if !client.IsConnected() {
		return nil, fmt.Errorf("device %s is not connected", deviceID)
	}

	now := time.Now().UTC()
	if err := s.reserveFullHistorySyncRequest(deviceID, now); err != nil {
		return nil, err
	}
	sent := false
	defer func() {
		if !sent {
			s.releaseFullHistorySyncRequest(deviceID)
		}
	}()

	from := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
	requestID := uuid.NewString()
	message := &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_FULL_HISTORY_SYNC_ON_DEMAND.Enum(),
				FullHistorySyncOnDemandRequest: &waE2E.PeerDataOperationRequestMessage_FullHistorySyncOnDemandRequest{
					RequestMetadata: &waE2E.FullHistorySyncOnDemandRequestMetadata{
						RequestID:       proto.String(requestID),
						BusinessProduct: proto.String("retena"),
					},
					HistorySyncConfig: store.DeviceProps.HistorySyncConfig,
					FullHistorySyncOnDemandConfig: &waE2E.FullHistorySyncOnDemandConfig{
						HistoryFromTimestamp: proto.Uint64(uint64(from)),
						HistoryDurationDays:  proto.Uint32(uint32(days)),
					},
				},
			},
		},
	}

	if _, err := client.SendPeerMessage(ctx, message); err != nil {
		return nil, fmt.Errorf("request full history sync for device %s: %w", deviceID, err)
	}
	sent = true

	return &domainDevice.FullHistorySyncRequest{
		DeviceID:      deviceID,
		RequestID:     requestID,
		RequestedDays: requestedDays,
		Days:          days,
		MaxDays:       maxFullHistorySyncDays,
		FromTimestamp: from,
		RequestedAt:   now,
	}, nil
}

func (s *serviceDevice) reserveFullHistorySyncRequest(deviceID string, now time.Time) error {
	s.historySyncMu.Lock()
	defer s.historySyncMu.Unlock()

	if s.lastFullHistorySyncRequest == nil {
		s.lastFullHistorySyncRequest = make(map[string]time.Time)
	}
	if last, ok := s.lastFullHistorySyncRequest[deviceID]; ok {
		retryAt := last.Add(fullHistorySyncCooldown)
		if now.Before(retryAt) {
			return fmt.Errorf("%w for device %s; retry_after=%s", errFullHistorySyncCooldown, deviceID, retryAt.Format(time.RFC3339))
		}
	}
	s.lastFullHistorySyncRequest[deviceID] = now
	return nil
}

func (s *serviceDevice) releaseFullHistorySyncRequest(deviceID string) {
	s.historySyncMu.Lock()
	defer s.historySyncMu.Unlock()
	delete(s.lastFullHistorySyncRequest, deviceID)
}

func (s *serviceDevice) GetStatus(_ context.Context, deviceID string) (bool, bool, error) {
	if s.manager == nil {
		return false, false, fmt.Errorf("device manager not initialized")
	}
	if inst, ok := s.manager.GetDevice(deviceID); ok {
		client := inst.GetClient()
		if client == nil {
			return false, inst.State() == domainDevice.DeviceStateLoggedOut, nil
		}

		if client.Store == nil || client.Store.ID == nil {
			return false, inst.State() == domainDevice.DeviceStateLoggedOut, nil
		}

		// Update state snapshot based on live client flags
		state := deriveState(inst)
		return client.IsConnected(), state == domainDevice.DeviceStateLoggedIn, nil
	}
	return false, false, fmt.Errorf("device %s not found", deviceID)
}

// SetDeviceWebhook sets the webhook URL for a specific device.
func (s *serviceDevice) SetDeviceWebhook(ctx context.Context, deviceID string, webhookURL string) error {
	if s.manager == nil {
		return fmt.Errorf("device manager not initialized")
	}
	if _, ok := s.manager.GetDevice(deviceID); !ok {
		return pkgError.ErrDeviceNotFound
	}
	storage := s.manager.GetStorage()
	if storage == nil {
		return fmt.Errorf("storage not available")
	}

	var urlPtr *string
	if webhookURL != "" {
		urlPtr = &webhookURL
	}
	if err := storage.SetDeviceWebhookURL(deviceID, urlPtr); err != nil {
		return fmt.Errorf("failed to set device webhook: %w", err)
	}

	websocket.Broadcast <- websocket.BroadcastMessage{
		Code:    "DEVICE_WEBHOOK_UPDATED",
		Message: fmt.Sprintf("Device %s webhook updated", deviceID),
		Result: map[string]any{
			"device_id":   deviceID,
			"webhook_url": webhookURL,
		},
	}
	return nil
}

// GetDeviceWebhook retrieves the webhook URL for a specific device.
func (s *serviceDevice) GetDeviceWebhook(ctx context.Context, deviceID string) (string, error) {
	if s.manager == nil {
		return "", fmt.Errorf("device manager not initialized")
	}
	if _, ok := s.manager.GetDevice(deviceID); !ok {
		return "", pkgError.ErrDeviceNotFound
	}
	storage := s.manager.GetStorage()
	if storage == nil {
		return "", fmt.Errorf("storage not available")
	}

	webhookURL, err := storage.GetDeviceWebhookURL(deviceID)
	if err != nil {
		return "", fmt.Errorf("failed to get device webhook: %w", err)
	}
	if webhookURL == nil {
		return "", nil
	}
	return *webhookURL, nil
}

// SetDeviceWebhookConfig sets the complete webhook configuration for a specific device.
func (s *serviceDevice) SetDeviceWebhookConfig(ctx context.Context, deviceID string, config *domainChatStorage.DeviceWebhookConfig) error {
	if s.manager == nil {
		return fmt.Errorf("device manager not initialized")
	}
	if _, ok := s.manager.GetDevice(deviceID); !ok {
		return pkgError.ErrDeviceNotFound
	}
	storage := s.manager.GetStorage()
	if storage == nil {
		return fmt.Errorf("storage not available")
	}

	if err := storage.SetDeviceWebhookConfig(deviceID, config); err != nil {
		return fmt.Errorf("failed to set device webhook config: %w", err)
	}

	websocket.Broadcast <- websocket.BroadcastMessage{
		Code:    "DEVICE_WEBHOOK_CONFIG_UPDATED",
		Message: fmt.Sprintf("Device %s webhook config updated", deviceID),
		Result: map[string]any{
			"device_id": deviceID,
		},
	}
	return nil
}

// GetDeviceWebhookConfig retrieves the complete webhook configuration for a specific device.
func (s *serviceDevice) GetDeviceWebhookConfig(ctx context.Context, deviceID string) (*domainChatStorage.DeviceWebhookConfig, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("device manager not initialized")
	}
	if _, ok := s.manager.GetDevice(deviceID); !ok {
		return nil, pkgError.ErrDeviceNotFound
	}
	storage := s.manager.GetStorage()
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}

	config, err := storage.GetDeviceWebhookConfig(deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device webhook config: %w", err)
	}
	return config, nil
}

func convertInstance(inst *whatsapp.DeviceInstance) domainDevice.Device {
	if inst == nil {
		return domainDevice.Device{}
	}

	state := deriveState(inst)

	return domainDevice.Device{
		ID:          inst.ID(),
		PhoneNumber: inst.PhoneNumber(),
		DisplayName: inst.DisplayName(),
		State:       state,
		JID:         inst.JID(),
		CreatedAt:   inst.CreatedAt(),
	}
}

func deriveState(inst *whatsapp.DeviceInstance) domainDevice.DeviceState {
	if inst == nil {
		return domainDevice.DeviceStateDisconnected
	}

	client := inst.GetClient()
	state := inst.State()
	if client != nil {
		switch {
		case client.IsLoggedIn():
			state = domainDevice.DeviceStateLoggedIn
		case client.IsConnected():
			state = domainDevice.DeviceStateConnected
		default:
			if state != domainDevice.DeviceStateLoggedOut {
				state = domainDevice.DeviceStateDisconnected
			}
		}
		inst.SetState(state)
	}

	return state
}
