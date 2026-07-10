package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
)

func TestPairPhoneAfterReadinessDoesNotCallProviderEarly(t *testing.T) {
	session := whatsapp.NewPairingSession(context.Background(), "pairing-session-1")
	called := make(chan struct{}, 1)
	result := make(chan error, 1)

	go func() {
		_, err := pairPhoneAfterReadiness(context.Background(), session, func(context.Context) (string, error) {
			called <- struct{}{}
			return "redacted-pair-code", nil
		})
		result <- err
	}()

	select {
	case <-called:
		t.Fatal("phone pairing provider called before QR readiness")
	case <-time.After(20 * time.Millisecond):
	}

	session.PublishQR("qr-image", time.Now(), 60*time.Second)
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("phone pairing provider was not called after QR readiness")
	}
	if err := <-result; err != nil {
		t.Fatalf("pairPhoneAfterReadiness returned error: %v", err)
	}
}

func TestPairPhoneAfterReadinessStopsOnTerminalFailure(t *testing.T) {
	session := whatsapp.NewPairingSession(context.Background(), "pairing-session-1")
	session.MarkTerminal(whatsapp.PairingSessionFailed, "connect_failed", time.Now())
	called := false

	_, err := pairPhoneAfterReadiness(context.Background(), session, func(context.Context) (string, error) {
		called = true
		return "", nil
	})
	if err == nil {
		t.Fatal("expected terminal readiness error")
	}
	if called {
		t.Fatal("phone pairing provider called after terminal failure")
	}
}

func TestPairPhoneAfterReadinessIsSingleFlight(t *testing.T) {
	session := whatsapp.NewPairingSession(context.Background(), "pairing-session-1")
	session.PublishQR("qr-image", time.Now(), 60*time.Second)
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	firstDone := make(chan error, 1)

	go func() {
		_, err := pairPhoneAfterReadiness(context.Background(), session, func(context.Context) (string, error) {
			close(providerStarted)
			<-releaseProvider
			return "redacted-pair-code", nil
		})
		firstDone <- err
	}()
	<-providerStarted

	secondCalled := false
	_, secondErr := pairPhoneAfterReadiness(context.Background(), session, func(context.Context) (string, error) {
		secondCalled = true
		return "", nil
	})
	if secondErr == nil {
		t.Fatal("second concurrent phone pairing did not fail closed")
	}
	if secondCalled {
		t.Fatal("second concurrent phone pairing reached the provider")
	}

	close(releaseProvider)
	if err := <-firstDone; err != nil {
		t.Fatalf("first phone pairing failed: %v", err)
	}
}
