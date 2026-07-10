package whatsapp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
)

type PairingQRConsumer struct {
	WriteImage       func(code string) (string, error)
	RemoveImageAfter func(path string, after time.Duration)
	Now              func() time.Time
}

func ConsumePairingQRChannel(session *PairingSession, events <-chan whatsmeow.QRChannelItem, consumer PairingQRConsumer) {
	if session == nil {
		return
	}
	now := consumer.Now
	if now == nil {
		now = time.Now
	}

	for event := range events {
		switch event.Event {
		case whatsmeow.QRChannelEventCode:
			if consumer.WriteImage == nil {
				session.MarkTerminal(PairingSessionFailed, "qr_image_writer_missing", now())
				return
			}
			imagePath, err := consumer.WriteImage(event.Code)
			if err != nil {
				session.MarkTerminal(PairingSessionFailed, "qr_image_write_failed", now())
				return
			}
			session.PublishQR(imagePath, now(), event.Timeout)
			if consumer.RemoveImageAfter != nil {
				consumer.RemoveImageAfter(imagePath, event.Timeout)
			}
		case whatsmeow.QRChannelSuccess.Event:
			session.MarkTerminal(PairingSessionPaired, "", now())
			return
		case whatsmeow.QRChannelTimeout.Event:
			session.MarkTerminal(PairingSessionExpired, "qr_window_expired", now())
			return
		case whatsmeow.QRChannelClientOutdated.Event:
			session.MarkTerminal(PairingSessionFailed, "client_outdated", now())
			return
		case whatsmeow.QRChannelScannedWithoutMultidevice.Event:
			session.MarkTerminal(PairingSessionFailed, "multidevice_required", now())
			return
		case whatsmeow.QRChannelErrUnexpectedEvent.Event:
			session.MarkTerminal(PairingSessionFailed, "unexpected_pairing_state", now())
			return
		case whatsmeow.QRChannelEventError:
			session.MarkTerminal(PairingSessionFailed, "pairing_error", now())
			return
		case whatsmeow.QRChannelEventPasskeyRequest, whatsmeow.QRChannelEventPasskeyResponse:
			// The device event handler owns passkey state; the QR session stays active.
		default:
			session.MarkTerminal(PairingSessionFailed, "unknown_pairing_event", now())
			return
		}
	}

	if !session.IsTerminal() {
		session.MarkTerminal(PairingSessionFailed, "qr_channel_closed", now())
	}
}

type PairingSessionState string

const (
	PairingSessionStarting PairingSessionState = "starting"
	PairingSessionQRReady  PairingSessionState = "qr_ready"
	PairingSessionPaired   PairingSessionState = "paired"
	PairingSessionExpired  PairingSessionState = "expired"
	PairingSessionFailed   PairingSessionState = "failed"
	PairingSessionCanceled PairingSessionState = "canceled"
)

type PairingSessionSnapshot struct {
	SessionID  string
	Generation int64
	ImagePath  string
	EmittedAt  time.Time
	ValidUntil time.Time
	State      PairingSessionState
	ErrorCode  string
	TerminalAt time.Time
}

type PairingSession struct {
	mu                   sync.RWMutex
	id                   string
	ctx                  context.Context
	cancel               context.CancelFunc
	ready                chan struct{}
	readyOnce            sync.Once
	generation           int64
	imagePath            string
	emittedAt            time.Time
	validUntil           time.Time
	state                PairingSessionState
	errorCode            string
	terminalAt           time.Time
	phonePairingInFlight bool
}

func (s *PairingSession) BeginPhonePairing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phonePairingInFlight || isTerminalPairingSessionState(s.state) {
		return false
	}
	s.phonePairingInFlight = true
	return true
}

func (s *PairingSession) EndPhonePairing() {
	s.mu.Lock()
	s.phonePairingInFlight = false
	s.mu.Unlock()
}

func NewPairingSession(parent context.Context, id string) *PairingSession {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &PairingSession{
		id:     id,
		ctx:    ctx,
		cancel: cancel,
		ready:  make(chan struct{}),
		state:  PairingSessionStarting,
	}
}

func (s *PairingSession) ID() string {
	return s.id
}

func (s *PairingSession) Context() context.Context {
	return s.ctx
}

func (s *PairingSession) PublishQR(imagePath string, emittedAt time.Time, timeout time.Duration) PairingSessionSnapshot {
	s.mu.Lock()
	if !isTerminalPairingSessionState(s.state) {
		s.generation++
		s.imagePath = imagePath
		s.emittedAt = emittedAt
		s.validUntil = emittedAt.Add(timeout)
		s.state = PairingSessionQRReady
		s.errorCode = ""
	}
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	if snapshot.Generation > 0 {
		s.readyOnce.Do(func() { close(s.ready) })
	}
	return snapshot
}

func (s *PairingSession) WaitReady(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-s.ready:
		return pairingSessionReadiness(s.Snapshot())
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return pairingSessionReadiness(s.Snapshot())
	}
}

func pairingSessionReadiness(snapshot PairingSessionSnapshot) error {
	if isTerminalPairingSessionState(snapshot.State) {
		if snapshot.ErrorCode != "" {
			return fmt.Errorf("pairing session %s: %s", snapshot.State, snapshot.ErrorCode)
		}
		return fmt.Errorf("pairing session %s before QR readiness", snapshot.State)
	}
	if snapshot.Generation > 0 {
		return nil
	}
	return fmt.Errorf("pairing session %s before QR readiness", snapshot.State)
}

func (s *PairingSession) MarkTerminal(state PairingSessionState, errorCode string, terminalAt time.Time) PairingSessionSnapshot {
	if !isTerminalPairingSessionState(state) {
		state = PairingSessionFailed
		if errorCode == "" {
			errorCode = "invalid_terminal_state"
		}
	}

	s.mu.Lock()
	if !isTerminalPairingSessionState(s.state) {
		s.state = state
		s.errorCode = errorCode
		s.terminalAt = terminalAt
	}
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	s.readyOnce.Do(func() { close(s.ready) })
	s.cancel()
	return snapshot
}

func (s *PairingSession) Snapshot() PairingSessionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *PairingSession) IsTerminal() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return isTerminalPairingSessionState(s.state)
}

func (s *PairingSession) CanReplace() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch s.state {
	case PairingSessionPaired, PairingSessionExpired, PairingSessionFailed, PairingSessionCanceled:
		return true
	default:
		return false
	}
}

func (s *PairingSession) snapshotLocked() PairingSessionSnapshot {
	return PairingSessionSnapshot{
		SessionID:  s.id,
		Generation: s.generation,
		ImagePath:  s.imagePath,
		EmittedAt:  s.emittedAt,
		ValidUntil: s.validUntil,
		State:      s.state,
		ErrorCode:  s.errorCode,
		TerminalAt: s.terminalAt,
	}
}

func isTerminalPairingSessionState(state PairingSessionState) bool {
	switch state {
	case PairingSessionPaired, PairingSessionExpired, PairingSessionFailed, PairingSessionCanceled:
		return true
	default:
		return false
	}
}
