package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/types"
)

// Auto-connect identity gate (RET-427-FU7b).
//
// Auto-connect used to connect EVERY session found in the keys store, even
// sessions the tenant had marked disconnected, and then forwarded their
// unrelated traffic to the tenant webhook. The gate below never deletes keys:
// it only decides whether a stored session should be auto-connected at boot.
// Explicit user reconnects bypass it entirely.

// Skip reasons reported by ShouldAutoConnect.
const (
	// AutoConnectSkipNoRegisteredRecord: the session identity has no row in
	// this instance's device registry (chatstorage devices table).
	AutoConnectSkipNoRegisteredRecord = "no_registered_device_record"
	// AutoConnectSkipIdentityDisconnected: the optional identity service
	// (tenant-side session registry) reports the session as disconnected.
	AutoConnectSkipIdentityDisconnected = "identity_service_disconnected"
)

// AutoConnectVerdict is the outcome of the auto-connect gate.
type AutoConnectVerdict struct {
	Allow  bool
	Reason string // "" when the session may be auto-connected
}

// Seams so tests can drive the gate without a live registry or HTTP service
// (same defer/restore idiom as submitWebhookFn in webhook_forward.go).
var (
	autoConnectRecordLookupFn   = defaultAutoConnectRecordLookup
	autoConnectIdentityCheckFn  = defaultAutoConnectIdentityCheck
	autoConnectIdentityClientFn = func() *http.Client {
		return &http.Client{Timeout: 3 * time.Second}
	}
)

// SessionIdentityForDevice resolves the canonical session identity for a
// device manager entry: the whatsmeow Store.ID in NonAD form (bare JID). The
// JID field wins when present; the device id is used otherwise (for
// store-discovered devices the id IS the JID). Unparseable values fall back
// to the raw string so the ref stays stable and matchable.
func SessionIdentityForDevice(deviceID, jid string) string {
	raw := strings.TrimSpace(jid)
	if raw == "" {
		raw = strings.TrimSpace(deviceID)
	}
	if raw == "" {
		return ""
	}
	if parsed, err := types.ParseJID(raw); err == nil {
		return parsed.ToNonAD().String()
	}
	return raw
}

// ShouldAutoConnect evaluates the auto-connect identity gate for one stored
// session. Failure modes fail OPEN (the session is allowed): the gate may
// skip sessions, never block the fleet's healthy boot traffic.
func ShouldAutoConnect(ctx context.Context, deviceID, jid string) AutoConnectVerdict {
	sessionJID := SessionIdentityForDevice(deviceID, jid)
	if sessionJID == "" {
		return AutoConnectVerdict{Allow: true}
	}
	deviceRef := safeLogRef("device", sessionJID)

	record, err := autoConnectRecordLookupFn(sessionJID)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"event": "auto_connect_gate", "outcome": "registry_lookup_failed",
			"device_ref": deviceRef, "error_class": safeErrorClass(err),
		}).Warn("[AUTO_CONNECT] registry lookup failed; allowing session")
		return AutoConnectVerdict{Allow: true}
	}
	if record == nil {
		logrus.WithFields(logrus.Fields{
			"event": "auto_connect_gate", "outcome": "skipped",
			"reason":     AutoConnectSkipNoRegisteredRecord,
			"device_ref": deviceRef,
		}).Warn("[AUTO_CONNECT] skipping session with no registered device record")
		return AutoConnectVerdict{Allow: false, Reason: AutoConnectSkipNoRegisteredRecord}
	}

	connected, err := autoConnectIdentityCheckFn(ctx, sessionJID)
	if err != nil {
		// Off-by-default / unreachable identity service must never hold
		// boot traffic hostage: fail open with a warn.
		logrus.WithFields(logrus.Fields{
			"event": "auto_connect_gate", "outcome": "identity_check_failed",
			"device_ref": deviceRef, "error_class": safeErrorClass(err),
		}).Warn("[AUTO_CONNECT] identity check failed; allowing session")
		return AutoConnectVerdict{Allow: true}
	}
	if !connected {
		logrus.WithFields(logrus.Fields{
			"event": "auto_connect_gate", "outcome": "skipped",
			"reason":     AutoConnectSkipIdentityDisconnected,
			"device_ref": deviceRef,
		}).Warn("[AUTO_CONNECT] identity service reports session disconnected; skipping")
		return AutoConnectVerdict{Allow: false, Reason: AutoConnectSkipIdentityDisconnected}
	}
	return AutoConnectVerdict{Allow: true}
}

// defaultAutoConnectRecordLookup finds the device registry row for a session
// identity. A nil manager / nil storage means no registry is configured at
// all (single-session upstream boots): reported as "unavailable", which the
// caller treats as fail-open, NOT as an unregistered session.
func defaultAutoConnectRecordLookup(sessionJID string) (*domainChatStorage.DeviceRecord, error) {
	if strings.TrimSpace(sessionJID) == "" {
		return nil, nil
	}
	dm := GetDeviceManager()
	if dm == nil || dm.storage == nil {
		return nil, nil
	}
	return dm.storage.GetDeviceRecordByJID(sessionJID)
}

// defaultAutoConnectIdentityCheck asks the (optional, tenant-operated)
// identity service whether the session is currently connected. Configure it
// with RETENA_AUTO_CONNECT_IDENTITY_URL, a URL template that must contain
// {jid}; it is substituted with the URL-escaped session identity. Default
// empty = check disabled (compat: everything allowed).
//
// Accepted 200 responses (either shape):
//
//	{"connected": true|false}
//	{"state": "connected"|"disconnected"}
//
// Anything else (non-200, malformed body) is an error and fails open.
func defaultAutoConnectIdentityCheck(ctx context.Context, sessionJID string) (bool, error) {
	tmpl := strings.TrimSpace(config.RetenaAutoConnectIdentityURL)
	if tmpl == "" {
		return true, nil
	}
	if !strings.Contains(tmpl, "{jid}") {
		return true, fmt.Errorf("identity url template has no {jid} placeholder")
	}
	target := strings.ReplaceAll(tmpl, "{jid}", url.PathEscape(sessionJID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return true, err
	}
	resp, err := autoConnectIdentityClientFn().Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return true, fmt.Errorf("identity service returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return true, err
	}
	var payload struct {
		Connected *bool  `json:"connected"`
		State     string `json:"state"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return true, fmt.Errorf("identity service response is not JSON: %w", err)
	}
	switch {
	case payload.Connected != nil:
		return *payload.Connected, nil
	case strings.EqualFold(payload.State, "disconnected"):
		return false, nil
	case payload.State != "":
		return true, nil
	default:
		return true, fmt.Errorf("identity service response carries neither connected nor state")
	}
}
