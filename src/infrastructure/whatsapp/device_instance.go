package whatsapp

import (
	"sync"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainDevice "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/device"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// DeviceInstance bundles a WhatsApp client with device metadata and scoped storage.
type DeviceInstance struct {
	mu              sync.RWMutex
	id              string
	client          *whatsmeow.Client
	chatStorageRepo domainChatStorage.IChatStorageRepository
	state           domainDevice.DeviceState
	displayName     string
	phoneNumber     string
	jid             string
	createdAt       time.Time
	lastSeenAt      time.Time
	onLoggedOut     func(deviceID string) // Callback for remote logout cleanup

	// Pending passkey pairing state, populated by PairPasskey* events during login.
	passkeyChallenge     *types.WebAuthnPublicKey
	passkeyCode          string
	passkeySkipHandoffUX bool
	pairingSession       *PairingSession
}

func NewDeviceInstance(deviceID string, client *whatsmeow.Client, chatStorageRepo domainChatStorage.IChatStorageRepository) *DeviceInstance {
	jid := ""
	display := ""
	if client != nil && client.Store != nil && client.Store.ID != nil {
		jid = client.Store.ID.ToNonAD().String()
		display = client.Store.PushName
	}

	return &DeviceInstance{
		id:              deviceID,
		client:          client,
		chatStorageRepo: chatStorageRepo,
		state:           domainDevice.DeviceStateDisconnected,
		displayName:     display,
		jid:             jid,
		createdAt:       time.Now(),
		lastSeenAt:      time.Now(),
	}
}

func (d *DeviceInstance) ID() string {
	return d.id
}

func (d *DeviceInstance) GetClient() *whatsmeow.Client {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.client
}

func (d *DeviceInstance) GetChatStorage() domainChatStorage.IChatStorageRepository {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.chatStorageRepo
}

func (d *DeviceInstance) SetState(state domainDevice.DeviceState) {
	d.mu.Lock()
	if shouldRefreshDeviceLastSeen(d.state, state) {
		d.lastSeenAt = time.Now()
	}
	d.state = state
	d.mu.Unlock()
}

func (d *DeviceInstance) State() domainDevice.DeviceState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

func (d *DeviceInstance) DisplayName() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.displayName
}

func (d *DeviceInstance) PhoneNumber() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.phoneNumber
}

func (d *DeviceInstance) JID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.jid
}

func (d *DeviceInstance) CreatedAt() time.Time {
	return d.createdAt
}

func (d *DeviceInstance) LastSeenAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastSeenAt
}

func (d *DeviceInstance) Touch() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastSeenAt = time.Now()
}

// SetClient attaches a WhatsApp client to this instance and updates metadata.
func (d *DeviceInstance) SetClient(client *whatsmeow.Client) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.client = client
	d.refreshIdentityLocked()
	d.state = domainDevice.DeviceStateDisconnected
	d.lastSeenAt = time.Now()
}

// ResetClient detaches the WhatsApp client and clears the session-derived identity
// (jid, phone number) so the slot can be re-paired with a fresh client on the next
// login. The device id, display name and creation time are preserved, keeping the
// slot in place after a logout.
func (d *DeviceInstance) ResetClient() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pairingSession != nil && !d.pairingSession.IsTerminal() {
		d.pairingSession.MarkTerminal(PairingSessionCanceled, "client_reset", time.Now())
	}
	d.pairingSession = nil
	d.client = nil
	d.jid = ""
	d.phoneNumber = ""
	d.state = domainDevice.DeviceStateDisconnected
}

func (d *DeviceInstance) GetOrCreatePairingSession(factory func() *PairingSession) (*PairingSession, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.pairingSession != nil && !d.pairingSession.CanReplace() {
		return d.pairingSession, false
	}
	if factory == nil {
		return nil, false
	}
	d.pairingSession = factory()
	return d.pairingSession, d.pairingSession != nil
}

func (d *DeviceInstance) PairingSession() *PairingSession {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.pairingSession
}

func (d *DeviceInstance) ReleasePairingSession(sessionID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pairingSession == nil || d.pairingSession.ID() != sessionID {
		return false
	}
	if !d.pairingSession.IsTerminal() {
		d.pairingSession.MarkTerminal(PairingSessionCanceled, "released", time.Now())
	}
	d.pairingSession = nil
	return true
}

// SetChatStorage swaps the chat storage repository for this device.
func (d *DeviceInstance) SetChatStorage(repo domainChatStorage.IChatStorageRepository) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.chatStorageRepo = repo
}

// IsConnected returns the live connection flag if a client exists.
func (d *DeviceInstance) IsConnected() bool {
	d.mu.RLock()
	client := d.client
	d.mu.RUnlock()
	if client == nil {
		return false
	}
	return client.IsConnected()
}

// IsLoggedIn returns the login status if a client exists.
func (d *DeviceInstance) IsLoggedIn() bool {
	d.mu.RLock()
	client := d.client
	d.mu.RUnlock()
	if client == nil {
		return false
	}
	return client.IsLoggedIn()
}

// UpdateStateFromClient refreshes the snapshot state based on the client flags.
func (d *DeviceInstance) UpdateStateFromClient() domainDevice.DeviceState {
	d.mu.Lock()
	defer d.mu.Unlock()

	previousState := d.state
	nextState := d.state
	switch {
	case d.client != nil && d.client.IsLoggedIn():
		nextState = domainDevice.DeviceStateLoggedIn
	case d.client != nil && d.client.IsConnected():
		nextState = domainDevice.DeviceStateConnected
	default:
		if d.state != domainDevice.DeviceStateLoggedOut {
			nextState = domainDevice.DeviceStateDisconnected
		}
	}

	if shouldRefreshDeviceLastSeen(previousState, nextState) {
		d.lastSeenAt = time.Now()
	}
	d.state = nextState
	d.refreshIdentityLocked()
	return d.state
}

func shouldRefreshDeviceLastSeen(previousState, nextState domainDevice.DeviceState) bool {
	switch nextState {
	case domainDevice.DeviceStateLoggedIn, domainDevice.DeviceStateConnected, domainDevice.DeviceStateConnecting:
		return true
	}
	return previousState != nextState
}

func (d *DeviceInstance) refreshIdentityLocked() {
	if d.client != nil && d.client.Store != nil && d.client.Store.ID != nil {
		d.jid = d.client.Store.ID.ToNonAD().String()
		d.displayName = d.client.Store.PushName
	}
}

// SetPasskeyChallenge stores a pending WebAuthn challenge and clears any previous confirmation code.
func (d *DeviceInstance) SetPasskeyChallenge(pk *types.WebAuthnPublicKey) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.passkeyChallenge = pk
	d.passkeyCode = ""
	d.passkeySkipHandoffUX = false
}

// SetPasskeyConfirmation stores the pairing confirmation code and clears the pending challenge.
func (d *DeviceInstance) SetPasskeyConfirmation(code string, skipHandoffUX bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.passkeyChallenge = nil
	d.passkeyCode = code
	d.passkeySkipHandoffUX = skipHandoffUX
}

// PasskeyState returns the pending challenge, confirmation code and skip-handoff flag.
func (d *DeviceInstance) PasskeyState() (*types.WebAuthnPublicKey, string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.passkeyChallenge, d.passkeyCode, d.passkeySkipHandoffUX
}

// ClearPasskeyState resets all pending passkey pairing state.
func (d *DeviceInstance) ClearPasskeyState() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.passkeyChallenge = nil
	d.passkeyCode = ""
	d.passkeySkipHandoffUX = false
}

func (d *DeviceInstance) SetOnLoggedOut(callback func(deviceID string)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onLoggedOut = callback
}

func (d *DeviceInstance) TriggerLoggedOut() {
	d.mu.RLock()
	callback := d.onLoggedOut
	deviceID := d.id
	d.mu.RUnlock()

	if callback != nil {
		callback(deviceID)
	}
}
