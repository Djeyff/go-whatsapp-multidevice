package whatsapp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
)

func TestPairingSessionPublishesMonotonicQRGenerations(t *testing.T) {
	session := NewPairingSession(context.Background(), "pairing-session-1")
	base := time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC)

	for index, timeout := range []time.Duration{60 * time.Second, 20 * time.Second, 20 * time.Second} {
		emittedAt := base.Add(time.Duration(index) * time.Minute)
		snapshot := session.PublishQR("qr-image", emittedAt, timeout)
		wantGeneration := int64(index + 1)
		if snapshot.Generation != wantGeneration {
			t.Fatalf("generation = %d, want %d", snapshot.Generation, wantGeneration)
		}
		if snapshot.State != PairingSessionQRReady {
			t.Fatalf("state = %q, want %q", snapshot.State, PairingSessionQRReady)
		}
		if !snapshot.EmittedAt.Equal(emittedAt) {
			t.Fatalf("emitted_at = %s, want %s", snapshot.EmittedAt, emittedAt)
		}
		if !snapshot.ValidUntil.Equal(emittedAt.Add(timeout)) {
			t.Fatalf("valid_until = %s, want %s", snapshot.ValidUntil, emittedAt.Add(timeout))
		}
	}
}

func TestDeviceInstanceKeepsOneActivePairingSession(t *testing.T) {
	instance := NewDeviceInstance("device-1", nil, nil)
	var created atomic.Int32
	factory := func() *PairingSession {
		created.Add(1)
		return NewPairingSession(context.Background(), "pairing-session-1")
	}

	first, firstCreated := instance.GetOrCreatePairingSession(factory)
	second, secondCreated := instance.GetOrCreatePairingSession(factory)

	if !firstCreated {
		t.Fatal("first pairing session was not reported as created")
	}
	if secondCreated {
		t.Fatal("second lookup created an overlapping pairing session")
	}
	if first != second {
		t.Fatal("second lookup did not return the active pairing session")
	}
	if created.Load() != 1 {
		t.Fatalf("factory called %d times, want 1", created.Load())
	}
}

func TestPairingSessionWaitReadyBlocksUntilFirstQR(t *testing.T) {
	session := NewPairingSession(context.Background(), "pairing-session-1")
	ready := make(chan error, 1)

	go func() {
		ready <- session.WaitReady(context.Background())
	}()

	select {
	case <-ready:
		t.Fatal("WaitReady returned before a QR event")
	case <-time.After(20 * time.Millisecond):
	}

	session.PublishQR("qr-image", time.Now(), 60*time.Second)
	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("WaitReady returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitReady did not return after the first QR event")
	}
}

func TestTerminalPairingSessionCanBeReplaced(t *testing.T) {
	instance := NewDeviceInstance("device-1", nil, nil)
	first, _ := instance.GetOrCreatePairingSession(func() *PairingSession {
		return NewPairingSession(context.Background(), "pairing-session-1")
	})
	first.MarkTerminal(PairingSessionExpired, "qr_window_expired", time.Now())

	second, created := instance.GetOrCreatePairingSession(func() *PairingSession {
		return NewPairingSession(context.Background(), "pairing-session-2")
	})

	if !created {
		t.Fatal("terminal pairing session was not replaced")
	}
	if first == second || second.ID() != "pairing-session-2" {
		t.Fatal("replacement pairing session was not installed")
	}
}

func TestPairedSessionCanBeReplacedAfterClientDisconnects(t *testing.T) {
	instance := NewDeviceInstance("device-1", nil, nil)
	first, _ := instance.GetOrCreatePairingSession(func() *PairingSession {
		return NewPairingSession(context.Background(), "pairing-session-1")
	})
	first.PublishQR("qr-image", time.Now(), 60*time.Second)
	first.MarkTerminal(PairingSessionPaired, "", time.Now())

	second, created := instance.GetOrCreatePairingSession(func() *PairingSession {
		return NewPairingSession(context.Background(), "pairing-session-2")
	})

	if !created {
		t.Fatal("paired session was reused after a later disconnect")
	}
	if first == second || second.ID() != "pairing-session-2" {
		t.Fatal("fresh pairing session was not installed")
	}
}

func TestPairingSessionTerminalStateUnblocksReadinessWait(t *testing.T) {
	session := NewPairingSession(context.Background(), "pairing-session-1")
	session.MarkTerminal(PairingSessionFailed, "connect_failed", time.Now())

	if err := session.WaitReady(context.Background()); err == nil {
		t.Fatal("WaitReady returned nil after the session failed")
	}
}

func TestPairingSessionWaitReadyRejectsExpiredGeneration(t *testing.T) {
	session := NewPairingSession(context.Background(), "pairing-session-1")
	session.PublishQR("qr-image", time.Now().Add(-time.Minute), 20*time.Second)
	session.MarkTerminal(PairingSessionExpired, "qr_window_expired", time.Now())

	if err := session.WaitReady(context.Background()); err == nil {
		t.Fatal("WaitReady returned stale success after the QR session expired")
	}
}

func TestConsumePairingQRChannelPublishesEveryCodeUntilSuccess(t *testing.T) {
	session := NewPairingSession(context.Background(), "pairing-session-1")
	events := make(chan whatsmeow.QRChannelItem, 4)
	events <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "raw-1", Timeout: 60 * time.Second}
	events <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "raw-2", Timeout: 20 * time.Second}
	events <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "raw-3", Timeout: 20 * time.Second}
	events <- whatsmeow.QRChannelSuccess
	close(events)

	writes := 0
	removed := 0
	base := time.Date(2026, time.July, 10, 11, 0, 0, 0, time.UTC)
	ConsumePairingQRChannel(session, events, PairingQRConsumer{
		WriteImage: func(code string) (string, error) {
			writes++
			return "qr-image", nil
		},
		RemoveImageAfter: func(path string, after time.Duration) {
			removed++
		},
		Now: func() time.Time { return base.Add(time.Duration(writes) * time.Second) },
	})

	snapshot := session.Snapshot()
	if snapshot.Generation != 3 {
		t.Fatalf("generation = %d, want 3", snapshot.Generation)
	}
	if snapshot.State != PairingSessionPaired {
		t.Fatalf("state = %q, want %q", snapshot.State, PairingSessionPaired)
	}
	if writes != 3 || removed != 3 {
		t.Fatalf("writes/removals = %d/%d, want 3/3", writes, removed)
	}
}

func TestConsumePairingQRChannelClassifiesTimeout(t *testing.T) {
	session := NewPairingSession(context.Background(), "pairing-session-1")
	events := make(chan whatsmeow.QRChannelItem, 1)
	events <- whatsmeow.QRChannelTimeout
	close(events)

	ConsumePairingQRChannel(session, events, PairingQRConsumer{})
	snapshot := session.Snapshot()
	if snapshot.State != PairingSessionExpired || snapshot.ErrorCode != "qr_window_expired" {
		t.Fatalf("terminal snapshot = %#v", snapshot)
	}
}
