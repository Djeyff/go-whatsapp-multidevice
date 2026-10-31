package helpers

import (
	"context"
	"errors"
	"testing"

	domainApp "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
)

// autoConnectStubService drives SetAutoConnectAfterBooting without a live
// device manager; it implements the narrow autoConnectService slice.
type autoConnectStubService struct {
	devices    []domainApp.DevicesResponse
	reconnects []string
	failWith   map[string]error
}

func (s *autoConnectStubService) FetchDevices(_ context.Context) ([]domainApp.DevicesResponse, error) {
	return s.devices, nil
}

func (s *autoConnectStubService) Reconnect(_ context.Context, deviceID string) error {
	s.reconnects = append(s.reconnects, deviceID)
	if s.failWith != nil {
		if err, ok := s.failWith[deviceID]; ok {
			return err
		}
	}
	return nil
}

// withAutoConnectGate swaps the gate seam for one test (defer/restore
// idiom shared with the fork's other seams).
func withAutoConnectGate(t *testing.T, fn func(ctx context.Context, deviceID, jid string) whatsapp.AutoConnectVerdict) {
	t.Helper()
	original := shouldAutoConnectFn
	shouldAutoConnectFn = fn
	t.Cleanup(func() { shouldAutoConnectFn = original })
}

// TestSetAutoConnectAfterBooting_SkipsIdentityDisconnectedSession pins the
// RET-427-FU7b defect: a session the gate denies (tenant marked it
// disconnected) must NOT be reconnected, while the registered session still
// connects.
func TestSetAutoConnectAfterBooting_SkipsIdentityDisconnectedSession(t *testing.T) {
	withAutoConnectGate(t, func(_ context.Context, deviceID, _ string) whatsapp.AutoConnectVerdict {
		if deviceID == "584146485729@s.whatsapp.net" {
			return whatsapp.AutoConnectVerdict{Allow: false, Reason: whatsapp.AutoConnectSkipIdentityDisconnected}
		}
		return whatsapp.AutoConnectVerdict{Allow: true}
	})
	stub := &autoConnectStubService{devices: []domainApp.DevicesResponse{
		{Name: "tenant", Device: "18498837876@s.whatsapp.net", JID: "18498837876:9@s.whatsapp.net"},
		{Name: "stale", Device: "584146485729@s.whatsapp.net", JID: "584146485729:71@s.whatsapp.net"},
	}}

	SetAutoConnectAfterBooting(stub)

	if len(stub.reconnects) != 1 || stub.reconnects[0] != "18498837876@s.whatsapp.net" {
		t.Fatalf("only the registered session must reconnect, got %v", stub.reconnects)
	}
}

// TestSetAutoConnectAfterBooting_SkipsUnregisteredSession covers the other
// skip reason: no device record for the session identity.
func TestSetAutoConnectAfterBooting_SkipsUnregisteredSession(t *testing.T) {
	withAutoConnectGate(t, func(_ context.Context, deviceID, _ string) whatsapp.AutoConnectVerdict {
		if deviceID == "999@s.whatsapp.net" {
			return whatsapp.AutoConnectVerdict{Allow: false, Reason: whatsapp.AutoConnectSkipNoRegisteredRecord}
		}
		return whatsapp.AutoConnectVerdict{Allow: true}
	})
	stub := &autoConnectStubService{devices: []domainApp.DevicesResponse{
		{Name: "tenant", Device: "18498837876@s.whatsapp.net"},
		{Name: "unknown", Device: "999@s.whatsapp.net"},
	}}

	SetAutoConnectAfterBooting(stub)

	if len(stub.reconnects) != 1 || stub.reconnects[0] != "18498837876@s.whatsapp.net" {
		t.Fatalf("only the registered session must reconnect, got %v", stub.reconnects)
	}
}

// TestSetAutoConnectAfterBooting_ConnectsRegisteredSessions: gate-allowed
// sessions flow into Reconnect exactly as before the gate existed.
func TestSetAutoConnectAfterBooting_ConnectsRegisteredSessions(t *testing.T) {
	withAutoConnectGate(t, func(_ context.Context, _, _ string) whatsapp.AutoConnectVerdict {
		return whatsapp.AutoConnectVerdict{Allow: true}
	})
	stub := &autoConnectStubService{devices: []domainApp.DevicesResponse{
		{Name: "tenant", Device: "18498837876@s.whatsapp.net"},
		{Name: "second", Device: "18092044903@s.whatsapp.net"},
	}}

	SetAutoConnectAfterBooting(stub)

	if len(stub.reconnects) != 2 {
		t.Fatalf("both registered sessions must reconnect, got %v", stub.reconnects)
	}
}

// TestSetAutoConnectAfterBooting_StillHonoursDeletedSessionSkip: the
// pre-existing stale-session skip (session deleted) must keep working behind
// the gate.
func TestSetAutoConnectAfterBooting_StillHonoursDeletedSessionSkip(t *testing.T) {
	withAutoConnectGate(t, func(_ context.Context, _, _ string) whatsapp.AutoConnectVerdict {
		return whatsapp.AutoConnectVerdict{Allow: true}
	})
	stub := &autoConnectStubService{devices: []domainApp.DevicesResponse{
		{Name: "gone", Device: "deleted@s.whatsapp.net"},
	}}
	stub.failWith = map[string]error{
		"deleted@s.whatsapp.net": errors.New("device deleted@s.whatsapp.net is not logged in (session deleted)"),
	}

	SetAutoConnectAfterBooting(stub)

	if len(stub.reconnects) != 1 {
		t.Fatalf("reconnect attempt expected, got %v", stub.reconnects)
	}
}

func TestIsAutoConnectSessionDeleted(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "session deleted reconnect",
			err:  errors.New("device abc is not logged in (session deleted)"),
			want: true,
		},
		{
			name: "real reconnect failure",
			err:  errors.New("connect failed: websocket timeout"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAutoConnectSessionDeleted(tc.err); got != tc.want {
				t.Fatalf("IsAutoConnectSessionDeleted() = %v, want %v", got, tc.want)
			}
		})
	}
}
